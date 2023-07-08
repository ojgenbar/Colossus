package internal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/ojgenbar/Colossus/converter/utils"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

var processedImagesSuccess = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "colossus_converter_processed_images_success",
		Help: "Count successfully processed raw images",
	},
	[]string{"partition"},
)

var processedImagesFailure = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "colossus_converter_processed_images_failure",
		Help: "Count fails due processing raw images",
	},
	[]string{"partition"},
)

var processedImagesSuccessBytes = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "colossus_converter_processed_images_success_bytes",
		Help: "Total bytes successfully processed of raw images",
	},
	[]string{"mime_type"},
)

func RegisterMetrics() {
	prometheus.MustRegister(processedImagesSuccess)
	prometheus.MustRegister(processedImagesFailure)
	prometheus.MustRegister(processedImagesSuccessBytes)
}

func createConsumer(cfg utils.Kafka) (*kafka.Consumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":        cfg.BootstrapServers,
		"group.id":                 cfg.GroupId,
		"session.timeout.ms":       cfg.SessionTimeoutMs,
		"auto.offset.reset":        cfg.AutoOffsetReset,
		"enable.auto.commit":       true,
		"allow.auto.create.topics": false,
	})

	if err != nil {
		log.Fatalf("Failed to create consumer: %s\n", err)
		return nil, err
	}

	log.Printf("Created Consumer %v\n", c)
	return c, nil
}

func initializeS3Client(s3cfg utils.S3) (*minio.Client, error) {
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

func getDecodeEncodeFunctionsByMimeType(mimeType string) (
	func(r io.Reader) (image.Image, error),
	func(w io.Writer, m image.Image) error,
	error,
) {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		encodeFunc := func(w io.Writer, m image.Image) error { return jpeg.Encode(w, m, nil) }
		return jpeg.Decode, encodeFunc, nil
	case "image/png":
		return png.Decode, png.Encode, nil
	case "image/gif":
		encodeFunc := func(w io.Writer, m image.Image) error { return gif.Encode(w, m, nil) }
		return gif.Decode, encodeFunc, nil
	case "image/bmp", "image/x-ms-bmp":
		return bmp.Decode, bmp.Encode, nil
	case "image/tiff", "image/tif":
		encodeFunc := func(w io.Writer, m image.Image) error { return tiff.Encode(w, m, nil) }
		return tiff.Decode, encodeFunc, nil
	// Add more cases for other image types if needed
	default:
		return nil, nil, errors.New("unsupported MIME type")
	}
}

func convert(input io.Reader, output *io.PipeWriter, contentType string, k int) error {
	decodeFunc, encodeFunc, err := getDecodeEncodeFunctionsByMimeType(contentType)
	if err != nil {
		return err
	}

	// Decode the image:
	src, _ := decodeFunc(input)

	// Set the expected size that you want:
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Max.X/k, src.Bounds().Max.Y/k))

	// Resize:
	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	// Encode to `output`:
	err = encodeFunc(output, dst)
	if err != nil {
		return err
	}
	defer output.Close()
	return nil
}

func RetrieveS3File(clientS3 *minio.Client, bucketName string, objectName string) (*minio.Object, string, error) {
	var minioClient = clientS3

	objectInfo, err := minioClient.StatObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Println(err)
		return nil, "", err
	}

	object, err := minioClient.GetObject(context.Background(), bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Println(err)
		return nil, "", err
	}
	//retrievedRawImages.WithLabelValues(bucketName).Inc()
	return object, objectInfo.ContentType, nil
}

func ProcessOne(clientS3 *minio.Client, cfg *utils.S3, task *ConvertTask) error {
	var k = 2
	if task.K > 1 {
		k = task.K
	}

	ctx := context.Background()

	//get image stream
	object, contentType, err := RetrieveS3File(clientS3, cfg.Buckets.Raw.Name, task.FilenameRaw)
	if err != nil {
		return err
	}
	defer object.Close()

	pr, pw := io.Pipe()
	defer pr.Close()

	errs := make(chan error, 1)
	go func() {
		err = convert(object, pw, contentType, k)
		errs <- err
		close(errs)
	}()

	info, err := clientS3.PutObject(
		ctx, cfg.Buckets.Processed.Name, task.FilenameProcessed, pr,
		-1, minio.PutObjectOptions{ContentType: contentType},
	)

	err = <-errs
	if err != nil {
		log.Fatalln(err)
		return err
	}

	if err != nil {
		log.Fatalln(err)
		return err
	}
	log.Printf("Successfully uploaded processed image, info: %s", info)
	processedImagesSuccessBytes.WithLabelValues(contentType).Add(float64(info.Size))
	//safe image at processed bucket
	return nil
}

func StartProcessing(sigchan chan os.Signal, wg *sync.WaitGroup, cfg *utils.Config) {
	var topics = []string{cfg.Kafka.Topic}
	consumer, err := createConsumer(cfg.Kafka)
	if err != nil {
		panic(err)
	}

	err = consumer.SubscribeTopics(topics, nil)
	if err != nil {
		panic(err)
	}

	clientS3, err := initializeS3Client(cfg.S3)
	if err != nil {
		panic(err)
	}

	run := true
	for run {
		select {
		case sig := <-sigchan:
			log.Printf("Caught signal %v: terminating\n", sig)
			run = false
		default:
			ev := consumer.Poll(100)
			if ev == nil {
				continue
			}

			switch e := ev.(type) {
			case *kafka.Message:
				value := ConvertTask{}
				err = json.Unmarshal(e.Value, &value)
				if err != nil {
					log.Printf("Failed to deserialize payload: %s\n", err)
				} else {
					log.Printf("%% Message on %s:\n%+v\n", e.TopicPartition, value)
					err = ProcessOne(clientS3, &cfg.S3, &value)
					if err != nil {
						log.Printf("Failed to process: %s\n", err)
						processedImagesFailure.WithLabelValues(strconv.Itoa(int(e.TopicPartition.Partition))).Inc()
					} else {
						processedImagesSuccess.WithLabelValues(strconv.Itoa(int(e.TopicPartition.Partition))).Inc()
					}
				}
				if e.Headers != nil {
					log.Printf("%% Headers: %v\n", e.Headers)
				}
			case kafka.Error:
				// Errors should generally be considered
				// informational, the client will try to
				// automatically recover.
				log.Printf("Error: %v: %v\n", e.Code(), e)
			default:
				log.Printf("Ignored event %v\n", e)
			}
		}
	}

	log.Printf("Closing consumer\n")
	consumer.Close()
	log.Println("Graceful consumer shutdown complete.")
	wg.Done()
}

type ConvertTask struct {
	FilenameProcessed string    `json:"filename_processed"`
	FilenameRaw       string    `json:"filename_raw"`
	Message           string    `json:"message"`
	QueuedAt          time.Time `json:"queued_at"`
	K                 int       `json:"k"`
}
