// internal/domain/events/events.go
package events

import "time"

// whenever a new file is detected by file watcher
type FileDetectedEvent struct {
	FilePath  string    `json:"filePath"`
	FileType  string    `json:"fileType"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
}

//TODO: FileChangedEvent struct {}

type AnalysisRequestedEvent struct {
	FilePath  string    `json:"filePath"`
	FileType  string    `json:"fileType"`
	Timestamp time.Time `json:"timestamp"`
}

type AnalysisCompletedEvent struct {
	FilePath       string        `json:"filePath"`
	ResultKey      string        `json:"resultKey"`      // S3 key where the result is stored
	AnalysisType   string        `json:"analysisType"`
	ProcessingTime time.Duration `json:"processingTime"` // How long the analysis took
	Timestamp      time.Time     `json:"timestamp"`
}