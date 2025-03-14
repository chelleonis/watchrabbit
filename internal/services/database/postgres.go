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

// below is mostly copied from AI generation, too much SQL boilerplate - may need to correct later
//Results section
func (p *PostgresService) CreateResultRecord(ctx context.Context, analysisID int64, resultType, storageType, storageKey, contentType string, sizeBytes int64, metadata map[string]string) (int64, error) {
	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal metadata: %v", err)
	}

	// Insert result record
	query := `
		INSERT INTO biomarker.results 
		(analysis_id, result_type, storage_type, storage_key, content_type, size_bytes, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING result_id
	`
	
	var resultID int64
	err = p.db.GetContext(ctx, &resultID, query, analysisID, resultType, storageType, storageKey, contentType, sizeBytes, metadataJSON)
	if err != nil {
		return 0, fmt.Errorf("failed to create result record: %v", err)
	}

	log.Printf("Created result record with ID: %d for analysis ID: %d", resultID, analysisID)
	return resultID, nil
}

func (p *PostgresService) GetResultsByAnalysisUUID(ctx context.Context, analysisUUID string) ([]ResultRecord, error) {
	query := `
		SELECT r.result_id, r.analysis_id, r.result_type, r.storage_type, 
		r.storage_key, r.content_type, r.size_bytes, r.created_at, r.metadata
		FROM biomarker.results r
		JOIN biomarker.analyses a ON r.analysis_id = a.analysis_id
		WHERE a.analysis_uuid = $1
	`
	
	var results []ResultRecord
	err := p.db.SelectContext(ctx, &results, query, analysisUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to query results: %v", err)
	}

	// Parse metadata JSON for each result
	for i := range results {
		if results[i].Metadata != nil {
			results[i].MetadataMap = make(map[string]string)
			if err := json.Unmarshal(results[i].Metadata, &results[i].MetadataMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal result metadata: %v", err)
			}
		}
	}

	return results, nil
}

// GetLatestAnalysesByFilePath gets the latest analyses for a file path
func (p *PostgresService) GetLatestAnalysesByFilePath(ctx context.Context, filePath string, limit int) ([]AnalysisDetails, error) {
	if limit <= 0 {
		limit = 10 // Default limit
	}

	// First get the file ID
	var fileID int64
	err := p.db.GetContext(ctx, &fileID, "SELECT file_id FROM biomarker.files WHERE file_path = $1", filePath)
	if err != nil {
		if errors.Is(err, sqlx.ErrNoRows) {
			return nil, nil // File not found
		}
		return nil, fmt.Errorf("failed to get file ID: %v", err)
	}

	// Get analysis records
	query := `
		SELECT analysis_id, analysis_uuid, file_id, analysis_type, status,
		started_at, completed_at, duration_ms, error_message, created_by, metadata
		FROM biomarker.analyses
		WHERE file_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	
	var analyses []AnalysisRecord
	err = p.db.SelectContext(ctx, &analyses, query, fileID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query analyses: %v", err)
	}

	// Get file details
	var file FileRecord
	err = p.db.GetContext(ctx, &file, `
		SELECT file_id, file_path, file_name, file_type, file_size, 
		created_at, last_modified, checksum, metadata
		FROM biomarker.files
		WHERE file_id = $1
	`, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file details: %v", err)
	}

	// Parse file metadata
	if file.Metadata != nil {
		file.MetadataMap = make(map[string]string)
		if err := json.Unmarshal(file.Metadata, &file.MetadataMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal file metadata: %v", err)
		}
	}

	// Combine results
	var analysisDetails []AnalysisDetails
	for _, analysis := range analyses {
		// Parse analysis metadata
		if analysis.Metadata != nil {
			analysis.MetadataMap = make(map[string]string)
			if err := json.Unmarshal(analysis.Metadata, &analysis.MetadataMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal analysis metadata: %v", err)
			}
		}

		// Get results for this analysis
		results, err := p.GetResultsByAnalysisUUID(ctx, analysis.AnalysisUUID)
		if err != nil {
			return nil, err
		}

		details := AnalysisDetails{
			AnalysisRecord: analysis,
			FileRecord:     file,
			Results:        results,
		}

		analysisDetails = append(analysisDetails, details)
	}

	return analysisDetails, nil
}

// ListAnalyses lists all analyses with optional filters
func (p *PostgresService) ListAnalyses(ctx context.Context, status string, limit, offset int) ([]AnalysisDetails, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	
	if offset < 0 {
		offset = 0
	}
	
	// Base query
	baseQuery := `
		SELECT a.analysis_id, a.analysis_uuid, a.file_id, a.analysis_type, a.status,
		a.started_at, a.completed_at, a.duration_ms, a.error_message, a.created_by, a.metadata
		FROM biomarker.analyses a
	`
	
	// Add filters
	var args []interface{}
	argCount := 1
	
	whereClause := ""
	if status != "" {
		whereClause = " WHERE a.status = $1"
		args = append(args, status)
		argCount++
	}
	
	// Add ordering and pagination
	query := baseQuery + whereClause + 
		" ORDER BY a.created_at DESC LIMIT $" + fmt.Sprintf("%d", argCount) + 
		" OFFSET $" + fmt.Sprintf("%d", argCount+1)
	
	args = append(args, limit, offset)
	
	// Execute query
	var analyses []AnalysisRecord
	err := p.db.SelectContext(ctx, &analyses, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list analyses: %v", err)
	}
	
	// Assemble full details for each analysis
	var results []AnalysisDetails
	for _, analysis := range analyses {
		// Get file details
		var file FileRecord
		err := p.db.GetContext(ctx, &file, `
			SELECT file_id, file_path, file_name, file_type, file_size, 
			created_at, last_modified, checksum, metadata
			FROM biomarker.files
			WHERE file_id = $1
		`, analysis.FileID)
		if err != nil {
			log.Printf("Warning: failed to get file details for analysis %s: %v", analysis.AnalysisUUID, err)
			continue
		}
		
		// Parse metadata
		if analysis.Metadata != nil {
			analysis.MetadataMap = make(map[string]string)
			if err := json.Unmarshal(analysis.Metadata, &analysis.MetadataMap); err != nil {
				log.Printf("Warning: failed to unmarshal analysis metadata: %v", err)
			}
		}
		
		if file.Metadata != nil {
			file.MetadataMap = make(map[string]string)
			if err := json.Unmarshal(file.Metadata, &file.MetadataMap); err != nil {
				log.Printf("Warning: failed to unmarshal file metadata: %v", err)
			}
		}
		
		// Get results
		analysisResults, err := p.GetResultsByAnalysisUUID(ctx, analysis.AnalysisUUID)
		if err != nil {
			log.Printf("Warning: failed to get results for analysis %s: %v", analysis.AnalysisUUID, err)
		}
		
		details := AnalysisDetails{
			AnalysisRecord: analysis,
			FileRecord:     file,
			Results:        analysisResults,
		}
		
		results = append(results, details)
	}
	
	return results, nil
}