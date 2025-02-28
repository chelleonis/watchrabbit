// cmd/file-watcher/main.go
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"
	"watchrabbit/internal/config"
	"watchrabbit/internal/domain/events"
	"watchrabbit/pkg/messaging"

	"github.com/fsnotify/fsnotify"
)

//todo: load config - read settings from config.go
// init rabbitmq client - setup exchanges/queues
// setup rabbitmq infrastructure
//init file watcher - use fsnotify to watch for file changes
// process file events - filter files (.csv or .sas7bdat)
// publish events if occur
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	rabbitClient, err := messaging.NewRabbitMQClient(cfg.RabbitMQ.URI)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer rabbitClient.Close()

	if err := rabbitClient.SetupInfrastructure(); err != nil {
		log.Fatalf("Failed to set up RabbitMQ infrastructure: %v", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	//adding directories to watch:
	for _, dir := range cfg.FileWatcher.Directories {
		if err := watcher.Add(dir); err != nil {
			log.Fatalf("Error in watching directory %s: $v", dir, err)
		}
		//for development, prod will have a lot of directories
		log.Printf("Watching Directory: %s", dir)
	}

	// infinite loop w/ no exit condition to constantly watch files
	for { 
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			//only process create/write events
			if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
				ext := filepath.Ext(event.Name)
				if !isFileTypeSupported(ext, cfg.FileWatcher.SupportedExtensions) {
					continue
				}
				fileInfo, err := os.Stat(event.Name)
				if err != nil {
					log.Printf("Error getting file info: %v", err)
					continue
				}
				//skip directories
				if fileInfo.IsDir() {
					continue
				}

				//publish event:
				fileEvent := events.FileDetectedEvent{
					FilePath: event.Name,
					FileType: ext,
					Size: fileInfo.Size(),
					Timestamp: time.Now(),
				}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				routingKey := "file.detected" + ext
				err = rabbitClient.PublishEvent(ctx, "biomarker.file.events", routingKey, fileEvent)
				cancel()

				if err != nil {
					log.Printf("Failed to publish file detected event: %v", err)
				} else {
					log.Printf("Published file detected event for %s", event.Name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func isFileTypeSupported(ext string, supportedExts []string) bool {
	for _, supported := range supportedExts {
		if ext == supported {
			return true
		}
	}
	return false
}