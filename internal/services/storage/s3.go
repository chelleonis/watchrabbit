// internal/services/storage/s3_service.go
package storage

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// S3Config holds S3 configuration settings
type S3Config struct {
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	Endpoint  string // Optional for local testing with MinIO/LocalStack
}

// ResultData represents data to be stored in S3
type ResultData struct {
	FilePath    string                 `json:"filePath"`
	AnalysisID  string                 `json:"analysisId"`
	ContentType string                 `json:"contentType"`
	OutputPath  string                 `json:"outputPath"`   // Local path to the output file
	Metadata    map[string]string      `json:"metadata"`     // Metadata for the result
}

// S3Service handles storage operations using S3
type S3Service struct {
	client   *s3.S3
	uploader *s3manager.Uploader
	bucket   string
}

// NewS3Service creates a new S3 storage service
func NewS3Service(config S3Config) (*S3Service, error) {
	// Create AWS session configuration
	awsConfig := &aws.Config{
		Region: aws.String(config.Region),
	}

	// Add credentials if provided
	if config.AccessKey != "" && config.SecretKey != "" {
		awsConfig.Credentials = credentials.NewStaticCredentials(
			config.AccessKey,
			config.SecretKey,
			"", // Token can be empty for local testing
		)
	}

	// Set custom endpoint for local testing if provided
	if config.Endpoint != "" {
		awsConfig.Endpoint = aws.String(config.Endpoint)
		awsConfig.S3ForcePathStyle = aws.Bool(true) // Required for MinIO/LocalStack
	}

	// Create session
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Create S3 client and uploader
	s3Client := s3.New(sess)
	uploader := s3manager.NewUploader(sess)

	log.Printf("Initialized S3 service for bucket: %s in region: %s", config.Bucket, config.Region)
	
	// Create a new S3Service instance
	return &S3Service{
		client:   s3Client,
		uploader: uploader,
		bucket:   config.Bucket,
	}, nil
}

// StoreResult stores analysis results in S3
func (s *S3Service) StoreResult(result *ResultData) (string, error) {
	if result == nil {
		return "", fmt.Errorf("cannot store nil result")
	}

	// Generate S3 key for the result
	// Format: results/{year}/{month}/{day}/{analysisId}/{filename}
	now := time.Now()
	baseFileName := filepath.Base(result.OutputPath)
	
	s3Key := fmt.Sprintf("results/%d/%02d/%02d/%s/%s",
		now.Year(), now.Month(), now.Day(),
		result.AnalysisID,
		baseFileName,
	)

	// Read the file from disk
	file, err := os.Open(result.OutputPath)
	if err != nil {
		return "", fmt.Errorf("failed to open result file: %v", err)
	}
	defer file.Close()

	// Prepare metadata (convert map[string]string to map[string]*string for AWS SDK)
	awsMetadata := make(map[string]*string)
	for key, value := range result.Metadata {
		awsMetadata[key] = aws.String(value)
	}
	
	// Add some standard metadata
	awsMetadata["AnalysisID"] = aws.String(result.AnalysisID)
	awsMetadata["OriginalFile"] = aws.String(filepath.Base(result.FilePath))
	awsMetadata["Timestamp"] = aws.String(now.Format(time.RFC3339))

	// Upload file to S3
	log.Printf("Uploading result to S3: %s", s3Key)
	
	// Read file into buffer to get content length
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file content: %v", err)
	}
	
	// Upload using uploader
	_, err = s.uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(fileContent),
		ContentType: aws.String(result.ContentType),
		Metadata:    awsMetadata,
	})
	
	if err != nil {
		return "", fmt.Errorf("failed to upload file to S3: %v", err)
	}

	log.Printf("Successfully uploaded result to S3 at key: %s", s3Key)
	return s3Key, nil
}

// GetResult retrieves a result from S3
func (s *S3Service) GetResult(s3Key string) ([]byte, string, error) {
	// Create a buffer to store the result
	buf := aws.NewWriteAtBuffer([]byte{})
	
	// Create a downloader
	downloader := s3manager.NewDownloader(session.Must(session.NewSession(&aws.Config{
		Region: s.client.Config.Region,
	})))
	
	// Download the file
	_, err := downloader.Download(buf,
		&s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s3Key),
		})
	
	if err != nil {
		return nil, "", fmt.Errorf("failed to download file from S3: %v", err)
	}
	
	// Get object attributes to retrieve ContentType
	attrs, err := s.client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	
	if err != nil {
		return buf.Bytes(), "application/octet-stream", nil // Default content type if we can't retrieve it
	}
	
	contentType := "application/octet-stream"
	if attrs.ContentType != nil {
		contentType = *attrs.ContentType
	}
	
	return buf.Bytes(), contentType, nil
}

// DeleteResult deletes a result from S3
func (s *S3Service) DeleteResult(s3Key string) error {
	_, err := s.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	
	if err != nil {
		return fmt.Errorf("failed to delete object from S3: %v", err)
	}
	
	// Wait for the deletion to complete
	err = s.client.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key),
	})
	
	if err != nil {
		return fmt.Errorf("error waiting for object deletion: %v", err)
	}
	
	log.Printf("Successfully deleted S3 object at key: %s", s3Key)
	return nil
}

// ListResults lists all results in a given prefix
func (s *S3Service) ListResults(prefix string) ([]string, error) {
	// List objects in the bucket with the given prefix
	resp, err := s.client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in S3: %v", err)
	}
	
	// Extract the keys from the response
	var keys []string
	for _, item := range resp.Contents {
		keys = append(keys, *item.Key)
	}
	
	return keys, nil
}