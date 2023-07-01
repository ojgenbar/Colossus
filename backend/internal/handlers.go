package handlers

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ojgenbar/Colossus/backend/utils"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
)

func InitializeS3Client(s3cfg utils.S3) (*minio.Client, error) {
	endpoint := s3cfg.Auth.Endpoint
	accessKeyID := s3cfg.Auth.AccessKeyID
	secretAccessKey := s3cfg.Auth.SecretAccessKey

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		//Secure: useSSL,
	})
	if err != nil {
		log.Fatalln(err)
		return nil, err
	}
	return minioClient, nil
}

func PrepareS3Buckets(s3cfg utils.S3) {
	PrepareS3Bucket(
		s3cfg,
		s3cfg.Buckets.Raw.Name,
		s3cfg.Buckets.Raw.Location,
	)
	PrepareS3Bucket(
		s3cfg,
		s3cfg.Buckets.Processed.Name,
		s3cfg.Buckets.Processed.Location,
	)
}

func PrepareS3Bucket(s3cfg utils.S3, name string, location string) {
	ctx := context.Background()

	minioClient, err := InitializeS3Client(s3cfg)
	if err != nil {
		panic(err)
	}

	err = minioClient.MakeBucket(ctx, name, minio.MakeBucketOptions{Region: location})
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, errBucketExists := minioClient.BucketExists(ctx, name)
		if errBucketExists == nil && exists {
			log.Printf("We already own %s\n", name)
		} else {
			log.Fatalln(err)
		}
	} else {
		log.Printf("Successfully created %s\n", name)
	}
}

func UploadToS3(s3cfg utils.S3, objectName string, uploadedFile *multipart.FileHeader) (minio.UploadInfo, error) {
	ctx := context.Background()

	// Initialize minio client object.
	minioClient, err := InitializeS3Client(s3cfg)
	if err != nil {
		log.Fatalln(err)
	}

	contentType := uploadedFile.Header.Get("Content-Type")

	// Upload the zip file with FPutObject
	f, err := uploadedFile.Open()
	if err != nil {
		return minio.UploadInfo{}, err
	}

	bucketName := s3cfg.Buckets.Raw.Name

	info, err := minioClient.PutObject(
		ctx, bucketName, objectName, f,
		uploadedFile.Size, minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		log.Fatalln(err)
		return minio.UploadInfo{}, err
	}

	log.Printf("Successfully uploaded %s of size %d\n", objectName, info.Size)
	return info, err
}

func GenerateNamePair(fileName string) (fileNameRaw string, fileNameProcessed string) {
	id := uuid.New()
	ext := filepath.Ext(fileName)
	fileNameRaw = fmt.Sprintf("%s-raw%s", id, ext)
	fileNameProcessed = fmt.Sprintf("%s-processed%s", id, ext)
	return fileNameRaw, fileNameProcessed
}

func RetrieveS3File(c *gin.Context, s3cfg utils.S3, bucketName, objectName string) {
	minioClient, err := InitializeS3Client(s3cfg)
	if err != nil {
		JsonErrorResponse(c, err, http.StatusInternalServerError)
	}

	objectInfo, err := minioClient.StatObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		fmt.Println(err)
		JsonErrorResponse(c, err, http.StatusNotFound)
		return
	}

	object, err := minioClient.GetObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		fmt.Println(err)
		JsonErrorResponse(c, err, http.StatusInternalServerError)
		return
	}
	defer object.Close()

	c.DataFromReader(http.StatusOK, objectInfo.Size, objectInfo.ContentType, object, nil)
}

func JsonErrorResponse(c *gin.Context, err error, status int) {
	c.JSON(status, gin.H{
		"message": err.Error(),
		"error":   true,
	})
}

func HandleHealthz(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "ok",
	})
}

func HandleFileUploadToBucket(c *gin.Context) {
	cfg := c.MustGet("cfg").(*utils.Config)

	f, uploadedFile, err := c.Request.FormFile("file")
	if err != nil {
		JsonErrorResponse(c, err, http.StatusInternalServerError)
		return
	}

	defer f.Close()

	fileNameRaw, fileNameProcessed := GenerateNamePair(uploadedFile.Filename)

	info, err := UploadToS3(cfg.S3, fileNameRaw, uploadedFile)
	if err != nil {
		JsonErrorResponse(c, err, http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "file uploaded successfully",
		"filename_raw":       fileNameRaw,
		"filename_processed": fileNameProcessed,
		"info":               info,
	})
}

func HandleFileRetrieveUploadToBucket(c *gin.Context) {
	cfg := c.MustGet("cfg").(*utils.Config)
	type_ := c.Param("type")
	file := c.Param("file")

	var bucketName string
	switch type_ {
	case "raw":
		bucketName = cfg.S3.Buckets.Raw.Name
	case "processed":
		bucketName = cfg.S3.Buckets.Processed.Name
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"message": fmt.Sprintf("'%s' is not a valid type", type_),
			"error":   true,
		})
		return
	}
	RetrieveS3File(c, cfg.S3, bucketName, file)
}
