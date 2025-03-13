// internal/services/database/postgres.go
package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
)

type PostgresConfig struct {
	Host string
	Port int
	User string
	Password string
	DBName string
	SSLMode string
}
// 3 main file storage types: Files, Analyses, Results
// FileRecords - files in the db
// Analysis Records - Analysis metadata in the DB
// Results records - Results in the DB
// AnalysisDetails combines the above 3 records that are part of the whole analysis workflow
type FileRecord struct {
	FileID       int64             `db:"file_id" json:"file_id"`
	FilePath     string            `db:"file_path" json:"file_path"`
	FileName     string            `db:"file_name" json:"file_name"`
	FileType     string            `db:"file_type" json:"file_type"`
	FileSize     int64             `db:"file_size" json:"file_size"`
	CreatedAt    time.Time         `db:"created_at" json:"created_at"`
	LastModified time.Time         `db:"last_modified" json:"last_modified"`
	Checksum     string            `db:"checksum" json:"checksum,omitempty"`
	Metadata     json.RawMessage   `db:"metadata" json:"-"`
	MetadataMap  map[string]string `db:"-" json:"metadata,omitempty"`
}

type AnalysisRecord struct {
	AnalysisID    int64             `db:"analysis_id" json:"analysis_id"`
	AnalysisUUID  string            `db:"analysis_uuid" json:"analysis_uuid"`
	FileID        int64             `db:"file_id" json:"file_id"`
	AnalysisType  string            `db:"analysis_type" json:"analysis_type"`
	Status        string            `db:"status" json:"status"`
	StartedAt     *time.Time        `db:"started_at" json:"started_at,omitempty"`
	CompletedAt   *time.Time        `db:"completed_at" json:"completed_at,omitempty"`
	DurationMs    *int64            `db:"duration_ms" json:"duration_ms,omitempty"`
	ErrorMessage  string            `db:"error_message" json:"error_message,omitempty"`
	CreatedBy     string            `db:"created_by" json:"created_by,omitempty"`
	Metadata      json.RawMessage   `db:"metadata" json:"-"`
	MetadataMap   map[string]string `db:"-" json:"metadata,omitempty"`
}

type ResultRecord struct {
	ResultID    int64             `db:"result_id" json:"result_id"`
	AnalysisID  int64             `db:"analysis_id" json:"analysis_id"`
	ResultType  string            `db:"result_type" json:"result_type"`
	StorageType string            `db:"storage_type" json:"storage_type"`
	StorageKey  string            `db:"storage_key" json:"storage_key"`
	ContentType string            `db:"content_type" json:"content_type"`
	SizeBytes   int64             `db:"size_bytes" json:"size_bytes,omitempty"`
	CreatedAt   time.Time         `db:"created_at" json:"created_at"`
	Metadata    json.RawMessage   `db:"metadata" json:"-"`
	MetadataMap map[string]string `db:"-" json:"metadata,omitempty"`
}

type AnalysisDetails struct {
	AnalysisRecord
	FileRecord
	Results []ResultRecord `json:"results,omitempty"`
}

type PostgresService struct {
	db *sqlx.DB
}

func NewPostgresSerivce(config PostgresConfig) (*PostgresService, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode,
	)

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %v", err)
	}

	//DB connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5* time.Minute)

	log.Printf("Connected to PostgreSQL database: %s", config.DBName)

	return &PostgresService{db: db}, nil
}

func (p *PostgresService) Close() error {
	return p.db.Close()
}
// File section
// return the ID of the file record
func (p *PostgresService) CreateFileRecord(ctx context.Context, filePath string, fileSize int64, metadata map[string]string) (int64, error) {
	fileName := filepath.Base(filePath)
	fileType := filepath.Ext(filePath)

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return -1, fmt.Errorf("failed to marshal metadata: %v", err)
	}

	query := `
	INSERT INTO biomarker.files (file_path, file_name, file_type, file_size, metadata)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING file_id
	`

	var fileID int64
	err = p.db.GetContext(ctx, &fileID, query, filePath, fileName, fileType, fileSize, metadataJSON)
	if err != nil {
		return 0, fmt.Errorf("failed to create file record: %v", err)
	}

	log.Printf("Created file record with ID: %d", fileID)
	return fileID, nil
}

func (p *PostgresService) GetFileRecordByPath(ctx context.Context, filePath string) (*FileRecord, error) {
	query := `
	SELECT file_id, file_path, file_name, file_type, file_size
	created_at, last_modified, checksum, metadata
	FROM biomarker.files
	WHERE file_path = $1
	`

	var file FileRecord 
	err := p.db.GetContext(ctx, &file, query, filePath) 
	if err != nil {
		if errors.Is(err, sqlx.ErrNoRows) {
			//file not found in db, no results
			return nil, nil 
		}
		return nil, fmt.Errorf("failed to get file: %v", err)
	}

	if file.Metadata != nil {
		file.MetadataMap = make(map[string]string)
		if err := json.Unmarshal(file.Metadata, &file.MetadataMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
		}
	}

	return &file, nil
}

// Analysis Section
func (p *PostgresService) CreateAnalysisRecord(ctx context.Context, fileID int64, analysisType, status string, metadata map[string]string) (string, error) {
	analysisUUID := uuid.New().String()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %v", err)
	}

	query := `
	INSERT INTO biomarker.analyses
	(analysis_uuid, file_id, analysis_type, status, metadata)
	VALUES ($1, $2, $3, $4, $5)
	`

	_, err = p.db.ExecContext(ctx, query, analysisUUID, fileID, analysisType, status, metadataJSON)
	if err != nil {
		return "", fmt.Errorf("failed to create analysis record: %v", err)
	}

	log.Printf("Created analysis with UUID: %s", analysisUUID)
	return analysisUUID, nil
}

func (p *PostgresService) UpdateAnalysisStatus(ctx context.Context, analysisUUID string, status string, error string) error {
	query := `SELECT biomarker.update_analysis_status($1, $2, $3)`
	_, err = p.db.ExecContext(ctx, query, analysisUUID, status, errorMessage)

	if err != nil {
		return fmt.Errorf("failed to update analysis status: %v", err)
	}

	log.Printf("Updated analysis %s status to: %s", analysisUUID, status)
	return nil
}

func (p *PostgresService) GetAnalysisRecordByUUID(ctx context.Context, analysisUUID string) (*AnalysisRecord, error) {
	query := `
	SELECT analysis_id, analysis_uuid, file_id, analysis_type, status, started_at, completed_at,
	duration_ms, error_message, created_by, metadata
	FROM biomarker.analyses
	WHERE analysis_uuid = $1
	`
	var analysis AnalysisRecord
	err := p.db.ExecContext(ctx, query, analysisUUID)

	if err != nil {
		if errors.Is(err, sqlx.ErrNoRows) {
			// no analysis found with UUID in db
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve analysis record: %v", err)
	}

	if analysis.Metadata != nil {
		analysis.MetadataMap = make(map[string]string)
		if err := json.Unmarshal(analysis.Metadata, &analysis.MetadataMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
		}
	}

	return &analysis, nil
}

//Results section

