// cmd/worker/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/yourorg/biomarker-system/internal/config"
	"github.com/yourorg/biomarker-system/internal/domain/events"
	"github.com/yourorg/biomarker-system/internal/services/analyzer"
	"github.com/yourorg/biomarker-system/internal/services/storage"
	"github.com/yourorg/biomarker-system/pkg/messaging"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize RabbitMQ client
	rabbitMQ, err := messaging.NewRabbitMQClient(cfg.RabbitMQ.URI)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer rabbitMQ.Close()

	// Set up RabbitMQ infrastructure
	if err := rabbitMQ.SetupInfrastructure(); err != nil {
		log.Fatalf("Failed to set up RabbitMQ infrastructure: %v", err)
	}

	// Initialize analyzer service
	analyzerService := analyzer.NewService()

	// Initialize storage service
	storageService, err := storage.NewS3Service(cfg.S3)
	if err != nil {
		log.Fatalf("Failed to initialize S3 storage: %v", err)
	}

	// Subscribe to file detected events
	if err := rabbitMQ.Subscribe("file.detected", func(data []byte) error {
		var fileEvent events.FileDetectedEvent
		if err := json.Unmarshal(data, &fileEvent); err != nil {
			log.Printf("Failed to unmarshal file detected event: %v", err)
			return err
		}

		log.Printf("Processing file: %s", fileEvent.FilePath)

		// Create analysis request event
		requestEvent := events.AnalysisRequestedEvent{
			FilePath:  fileEvent.FilePath,
			FileType:  fileEvent.FileType,
			Timestamp: time.Now(),
		}

		// Publish analysis request event
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		routingKey := "analysis.requested" + fileEvent.FileType
		return rabbitMQ.PublishEvent(ctx, "biomarker.analysis.events", routingKey, requestEvent)
	}); err != nil {
		log.Fatalf("Failed to subscribe to file detected events: %v", err)
	}

	// Subscribe to analysis requested events
	if err := rabbitMQ.Subscribe("analysis.requested", func(data []byte) error {
		var requestEvent events.AnalysisRequestedEvent
		if err := json.Unmarshal(data, &requestEvent); err != nil {
			log.Printf("Failed to unmarshal analysis requested event: %v", err)
			return err
		}

		log.Printf("Analyzing file: %s", requestEvent.FilePath)

		// Perform analysis
		result, err := analyzerService.Analyze(requestEvent.FilePath)
		if err != nil {
			log.Printf("Failed to analyze file: %v", err)
			return err
		}

		// Store result in S3
		s3Key, err := storageService.StoreResult(result)
		if err != nil {
			log.Printf("Failed to store result: %v", err)
			return err
		}

		// Create analysis completed event
		completedEvent := events.AnalysisCompletedEvent{
			FilePath:       requestEvent.FilePath,
			ResultKey:      s3Key,
			AnalysisType:   requestEvent.FileType,
			ProcessingTime: time.Since(requestEvent.Timestamp),
			Timestamp:      time.Now(),
		}

		// Publish analysis completed event
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		routingKey := "analysis.completed" + requestEvent.FileType
		return rabbitMQ.PublishEvent(ctx, "biomarker.result.events", routingKey, completedEvent)
	}); err != nil {
		log.Fatalf("Failed to subscribe to analysis requested events: %v", err)
	}

	log.Println("Worker service started")

	// Keep the application running
	select {}
}