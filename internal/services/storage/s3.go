// internal/services/storage/s3.go
package storage

import (
	"bytes"
	"context"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/yourorg/biomarker-system/internal/config" as appconfig
	"github.com/yourorg/biomarker-system/internal/services/analyzer"
)

// S3Service handles storage of analysis results in S3
type S3Service struct {
	client *s3.Client
	bucket string
}

// NewS3Service creates a new S3 storage service
func NewS3Service(cfg appconfig.S3Config) (*S3Service, error) {
	// Create AWS credentials from config
	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	
	// Load the AWS SDK configuration
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(creds),
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, err
	}
	
	// Create S3 client
	client := s3.NewFromConfig(awsCfg)
	
	return &S3Service{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// StoreResult stores an analysis result in S3
func (s *S3Service) StoreResult(result *analyzer.ResultData) (string, error) {
	// Generate a key for the result
	key := generateS3Key(result)
	
	// Create the S3 put object input
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(result.Data),
		ContentType: aws.String(result.ContentType),
		Metadata:    mapToAWSStrings(result.Metadata),
	}
	
	// Upload to S3
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		return "", err
	}
	
	return key, nil
}

// GetResult retrieves an analysis result from S3
func (s *S3Service) GetResult(key string) (*analyzer.ResultData, error) {
	// Create the S3 get object input
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	
	// Get object from S3
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	resp, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	// Read the response body
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Create the result data
	result := &analyzer.ResultData{
		AnalysisID:  filepath.Base(key),
		ContentType: aws.ToString(resp.ContentType),
		Data:        buf.Bytes(),
		Metadata:    awsStringsToMap(resp.Metadata),
	}
	
	return result, nil
}

// generateS3Key creates a unique S3 key for a result
func generateS3Key(result *analyzer.ResultData) string {
	// TODO: Implement a better key generation strategy
	// This is a simple placeholder
	filename := filepath.Base(result.FilePath)
	timestamp := time.Now().Format("20060102-150405")
	return "results/" + timestamp + "/" + filename + ".html"
}

// mapToAWSStrings converts a Go map to AWS string map
func mapToAWSStrings(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

// awsStringsToMap converts AWS string map to a Go map
func awsStringsToMap(m map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}