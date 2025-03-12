// cmd/worker/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"time"
	"watchrabbit/internal/config"
	"watchrabbit/internal/domain/events"
	"watchrabbit/internal/services/analyzer"
	"watchrabbit/internal/services/storage"
	"watchrabbit/pkg/messaging"
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

	// Initialize analyzer service - to replace with actual biomarker scripts or adapt template to use different R files
	// currently using a test script that generates an Rmd .html from a .csv file
	analyzerService, err := analyzer.NewDescriptiveService(
		cfg.Analysis.RExecutable,
		cfg.Analysis.ScriptsDir,
		cfg.Analysis.Timeout,
	)

	if err != nil {
		log.Fatalf("Failed to initialize descriptive report genreator: %v", err)
	}
	// Initialize storage service
	storageService, err := storage.NewS3Service(cfg.S3)
	if err != nil {
		log.Fatalf("Failed to initialize S3 storage: %v", err)
	}

	// Subscribe to RabbitMQ queues: 
	// file detected, analysis requested
	if err := subscribeToQueue(rabbitMQ, "file.detected", handleFileDetectedEvent(rabbitMQ)); err != nil {
		log.Fatalf("Failed to subscribe to file detected events: %v", err)
	}
	
	if err := subscribeToQueue(rabbitMQ, "analysis.requested", handleAnalysisRequestedEvent(rabbitMQ, analyzerService, storageService)); err != nil {
		log.Fatalf("Failed to subscribe to analysis requested events: %v", err)
	}

	// Keep the application running
	select {}
}

// RabbitMQ queue subscription helper functions:
type EventHandler func([]byte) error

func subscribeToQueue(rabbitMQ *messaging.RabbitMQClient, queueName string, handler EventHandler) error {
    log.Printf("Subscribing to queue: %s", queueName)
    return rabbitMQ.Subscribe(queueName, handler)
}

// sends any file change events to the RabbitMQ queue
// will also request an analysis (and send that to the queue) to generate a Rmarkdown report
func handleFileDetectedEvent(rabbitMQ *messaging.RabbitMQClient) EventHandler {
	return func(data []byte) error {
		var fileEvent events.FileDetectedEvent
		if err := json.Unmarshal(data, &fileEvent); err != nil {
			log.Printf("Failed to unmarshal file detected event: %v", err)
			return err
		}
		// file detected handler logic
		// may need to adjust types
		log.Printf("Received file detected event for: %s", fileEvent.FilePath)

		requestEvent := fileEvent.AnalysisRequestedEvent{
			FilePath: fileEvent.FilePath,
			FileType: fileEvent.FileType,
			Timestamp: time.Now(),
	}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		routingKey := "analysis.requested" + fileEvent.FileType

		if err := rabbitMQ.PublishEvent(ctx, "biomarker.analysis.events", routingKey, requestEvent); err != nil {
			log.Printf("Failed to publish analysis requested event: %v", err)
			return err
		}

		log.Printf("Published analysis requested event for file: %s", fileEvent.FilePath)
		return nil
	}
}

// subscribes to the analysis requested events + executes them via cmd line (in analyzer/descriptive_analyzer.go)
func handleAnalysisRequestedEvent(rabbitMQ *messaging.RabbitMQClient) EventHandler {
	return func(data []byte) error {
		var requestEvent events.AnalysisRequestedEvent
		if err := json.Unmarshal(data, &requestEvent); err != nil {
			log.Printf("Failed to unmarshal analysis requested event: %v", err)
			return err
		}
		// Analysis handler logic
		log.Printf("Processing analysis request for file: %s", requestEvent.FilePath)

		result, err := analyzerService.ExecuteAnalysis(requestEvent.FilePath)
		if err != nil {
			log.Printf("Analysis Failed: %v", err)
			// update analysis status if failed and close the queue ticket
			completedEvent := events.AnalysisCompletedEvent{
				FilePath: requestEvent.FilePath,
				ResultKey: "",
				AnalysisType: requestEvent.FilePath,
				ProcessingTime: time.Since(requestEvent.Timestamp),
				Timestamp: time.Now(),
				Status: "failed",
				ErrorMessage: err.Error(),
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			routingKey := "analysis.completed" + requestEvent.FileType
			return rabbitMQ.PublishEvent(ctx, "biomarker.result.events", routingKey, completedEvent)
		}
		// if successful, store result to postgres DB
		// TODO: implement postgres with GO

		// create & publish completed analysis to rabbitMQ
		completedEvent := events.AnalysisCompletedEvent{
			FilePath:       requestEvent.FilePath,
			ResultKey:      s3Key,
			AnalysisType:   requestEvent.FileType,
			ProcessingTime: result.Duration,
			Timestamp:      time.Now(),
			Status:         "success",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		routingKey := "analysis.completed" + requestEvent.FileType
		return rabbitMQ.PublishEvent(ctx, "biomarker.result.events", routingKey, completedEvent)
	}
}