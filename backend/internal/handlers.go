package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ojgenbar/Colossus/backend/utils"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func InitializeS3Client(s3cfg *utils.S3) (*minio.Client, error) {
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

func PrepareS3Buckets(s3cfg *utils.S3) {
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

func PrepareS3Bucket(s3cfg *utils.S3, name string, location string) {
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

func UploadToS3(s3cfg *utils.S3, objectName string, uploadedFile *multipart.FileHeader) (minio.UploadInfo, error) {
	ctx := context.Background()

	// Initialize minio client object.
	minioClient, err := InitializeS3Client(s3cfg)
	if err != nil {
		log.Fatalln(err)
		return minio.UploadInfo{}, err
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
	uploadedRawImages.Inc()

	return info, nil
}

func GenerateNamePair(fileName string) (fileNameRaw string, fileNameProcessed string) {
	id := uuid.New()
	ext := filepath.Ext(fileName)
	fileNameRaw = fmt.Sprintf("%s-raw%s", id, ext)
	fileNameProcessed = fmt.Sprintf("%s-processed%s", id, ext)
	return fileNameRaw, fileNameProcessed
}

func RetrieveS3File(c *gin.Context, s3cfg *utils.S3, bucketName, objectName string) {
	minioClient, err := InitializeS3Client(s3cfg)
	if err != nil {
		JsonErrorResponse(c, err, http.StatusInternalServerError)
	}

	objectInfo, err := minioClient.StatObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Println(err)
		JsonErrorResponse(c, err, http.StatusNotFound)
		return
	}

	object, err := minioClient.GetObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Println(err)
		JsonErrorResponse(c, err, http.StatusInternalServerError)
		return
	}
	defer object.Close()

	retrievedRawImages.WithLabelValues(bucketName).Inc()
	c.DataFromReader(http.StatusOK, objectInfo.Size, objectInfo.ContentType, object, nil)
}

func PrepareKafkaTopic(cfg *utils.Kafka) {
	// Create a new AdminClient.
	a, err := createKafkaAdminClient(cfg)
	if err != nil {
		fmt.Printf("Failed to create Admin client: %s\n", err)
		os.Exit(1)
	}

	// Contexts are used to abort or limit the amount of time
	// the Admin call blocks waiting for a result.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create topics on cluster.
	// Set Admin options to wait for the operation to finish (or at most 60s)
	maxDur, err := time.ParseDuration("60s")
	if err != nil {
		panic("ParseDuration(60s)")
	}
	results, err := a.CreateTopics(
		ctx,
		// Multiple topics can be created simultaneously
		// by providing more TopicSpecification structs here.
		[]kafka.TopicSpecification{{
			Topic:             cfg.Topic.Name,
			NumPartitions:     cfg.Topic.NumPartitions,
			ReplicationFactor: 1}},
		// Admin options
		kafka.SetAdminOperationTimeout(maxDur))
	if err != nil {
		fmt.Printf("Failed to create topic: %v\n", err)
		os.Exit(1)
	}

	// Print results
	for _, result := range results {
		fmt.Printf("%s\n", result)
	}

	a.Close()
}

func JsonErrorResponse(c *gin.Context, err error, status int) {
	c.JSON(status, gin.H{
		"message": err.Error(),
		"error":   true,
	})
}

func RegisterMetrics() {
	prometheus.MustRegister(uploadedRawImages)
	prometheus.MustRegister(uploadedRawImagesToKafka)
	prometheus.MustRegister(retrievedRawImages)
}

var uploadedRawImages = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "colossus_backend_uploaded_raw_images",
		Help: "Count successfully uploaded raw images",
	},
)

var uploadedRawImagesToKafka = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "colossus_backend_uploaded_raw_images_to_kafka",
		Help: "Count successfully uploaded raw images links to Kafka",
	},
	[]string{"partition", "client_id"},
)

var retrievedRawImages = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "colossus_backend_retrieved_images",
		Help: "Count successfully retrieved images from S3",
	},
	[]string{"s3_bucket_name"},
)

func HandleHealthz(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "ok",
	})
}

func PutInKafka(cfg *utils.Kafka, data *gin.H) (*kafka.Message, error) {
	topic := cfg.Topic.Name
	clientId := cfg.ClientId

	p, err, message, err2 := createKafkaProducer(cfg, clientId)
	if err2 != nil {
		return message, err2
	}

	b, err := json.Marshal(data)
	if err != nil {
		log.Fatalln(fmt.Sprintf("Failed to Marshall: %s\n", err), err)
		return nil, err
	}
	log.Println(string(b))

	payload := b

	deliveryChan := make(chan kafka.Event, 10000)
	err = p.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Value:          payload},
		deliveryChan,
	)

	e := <-deliveryChan
	m := e.(*kafka.Message)

	if m.TopicPartition.Error != nil {
		log.Fatalln(fmt.Sprintf("Delivery failed: %v\n", m.TopicPartition.Error), err)
		return nil, err
	} else {
		log.Printf("Delivered message to topic %s [%d] at offset %v\n",
			*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
	}
	close(deliveryChan)

	uploadedRawImagesToKafka.WithLabelValues(
		strconv.Itoa(int(m.TopicPartition.Partition)),
		clientId,
	).Inc()

	return m, nil
}

func createKafkaProducer(cfg *utils.Kafka, clientId string) (*kafka.Producer, error, *kafka.Message, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": cfg.BootstrapServers,
		"client.id":         clientId,
		"acks":              cfg.Acks,
	})

	if err != nil {
		log.Fatalln(fmt.Sprintf("Failed to create producer: %s\n", err), err)
		return nil, nil, nil, err
	}
	return p, err, nil, nil
}

func createKafkaAdminClient(cfg *utils.Kafka) (*kafka.AdminClient, error) {
	p, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers": cfg.BootstrapServers,
	})

	if err != nil {
		log.Fatalln(fmt.Sprintf("Failed to create admin client: %s\n", err), err)
		return nil, err
	}
	return p, nil
}

func HandleFileUploadRaw(c *gin.Context) {
	cfg := c.MustGet("cfg").(*utils.Config)

	f, uploadedFile, err := c.Request.FormFile("file")
	if err != nil {
		JsonErrorResponse(c, err, http.StatusBadRequest)
		return
	}

	defer f.Close()

	fileNameRaw, fileNameProcessed := GenerateNamePair(uploadedFile.Filename)

	info, err := UploadToS3(&cfg.S3, fileNameRaw, uploadedFile)
	if err != nil {
		JsonErrorResponse(c, err, http.StatusInternalServerError)
		return
	}

	data := gin.H{
		"message":            "file uploaded successfully",
		"filename_raw":       fileNameRaw,
		"filename_processed": fileNameProcessed,
		"queued_at":          time.Now().UTC().Format(time.RFC3339),
		"info":               info,
	}

	_, err = PutInKafka(&cfg.Kafka, &data)
	if err != nil {
		JsonErrorResponse(c, err, http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, data)
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
	RetrieveS3File(c, &cfg.S3, bucketName, file)
}
