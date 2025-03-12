package analyzer

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
)

type DescriptiveAnalysisMetadata struct {
	AnalysisID    string            `json:"analysisId"`
	FilePath      string            `json:"filePath"`
	Status        string            `json:"status"` // "success", "failed", "timeout"
	OutputPath    string            `json:"outputPath"`
	StartTime     time.Time         `json:"startTime"`
	EndTime       time.Time         `json:"endTime"`
	Duration      time.Duration     `json:"duration"`
	ErrorMessage  string            `json:"errorMessage,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// TODO: analysis connection to R backend using roger/Rserve
// FOR NOW, we will use os/exec for PoC
type DescriptiveService struct {
	// Path to R executable
	RExecutable string
	// Directory containing R scripts
	ScriptsDir string
	// Timeout for R script execution in seconds
	Timeout int
}

func NewDescriptiveService(rExecutable, scriptsDir string, timeoutSeconds int) (*DescriptiveService, error) {
	// attempt to find R executable if not in PATH:
	if rExecutable == "" {
		// Try to find Rscript in PATH
		rPath, err := exec.LookPath("Rscript")
		if err == nil {
			rExecutable = rPath
		} else {
			// Try common locations based on OS
			var possiblePaths []string
			
			if runtime.GOOS == "windows" {
				possiblePaths = []string{
					"C:\\Program Files\\R\\R-4.2.0\\bin\\Rscript.exe",
					"C:\\Program Files\\R\\R-4.1.0\\bin\\Rscript.exe",
					"C:\\Program Files\\R\\R-4.0.0\\bin\\Rscript.exe",
				}
			} else {
				// Linux/macOS paths
				possiblePaths = []string{
					"/usr/bin/Rscript",
					"/usr/local/bin/Rscript",
					"/opt/R/bin/Rscript",
				}
			}
			
			for _, path := range possiblePaths {
				if _, err := os.Stat(path); err == nil {
					rExecutable = path
					break
				}
			}
			
			if rExecutable == "" {
				return nil, errors.New("could not find R executable, please specify path explicitly")
			}
		}
	}

	// Verify scripts directory exists
	if _, err := os.Stat(scriptsDir); err != nil {
		return nil, fmt.Errorf("scripts directory not found: %v", err)
	}

	// Set default timeout if not provided
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300 // 5 minutes default
	}

	log.Printf("Analysis service initialized with R executable: %s", rExecutable)
	log.Printf("Using R scripts from: %s", scriptsDir)

	return &DescriptiveService{
		RExecutable: rExecutable,
		ScriptsDir:  scriptsDir,
		Timeout:     timeoutSeconds,
	}, nil
}

// Delegates analysis to R (doesn't actually perform analysis)
// TODO: generalize once we have 2-3 more R scripts, fine to do this for now
func (s *DescriptiveService) ExecuteAnalysis(filePath string) (*DescriptiveAnalysisMetadata, error) {
	//File & Script verification (in case files/folders are moved/missing)
	analysisID := uuid.New().String()

	outputDir := filepath.Join(os.TempDir(), "biomarker-analysis", time.Now().Format("20060102"))
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return createFailedResult(analysisID, filePath, fmt.Sprintf("Failed to create output directory: %v", err)), err
	}

	baseFileName := filepath.Base(filePath)
	outputFile := filepath.Join(outputDir, fmt.Sprintf("analysis_%s_%s.html", 
		baseFileName[:len(baseFileName)-len(filepath.Ext(baseFileName))], 
		analysisID[:8]))

	scriptName := "wr_dummy_analysis.R"

	fileExt := filepath.Ext(filePath)
	if fileExt != ".csv" && fileExt != ".sas7bdat" {
		err := fmt.Errorf("unsupported file type: %s", fileExt)
		return createFailedResult(analysisID, filePath, err.Error()), err
	}

	scriptPath := filepath.Join(s.ScriptsDir, scriptName)

	// R will handle the parsing of data (read_csv/read_sas through haven package)
	if _, err := os.Stat(scriptPath); err != nil {
		errMsg := fmt.Sprintf("R script not found: %s", scriptPath)
		return createFailedResult(analysisID, filePath, errMsg), errors.New(errMsg)
	}

	//Running the R script through cmd line -
	startTime := time.Now()
	cmd := exec.Command(s.RExecutable, scriptPath, filePath, outputFile)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

		//logging, to reduce lines in prod
	log.Printf("Starting R analysis for file: %s", filePath)
	log.Printf("Analysis ID: %s", analysisID)
	log.Printf("Output will be written to: %s", outputFile)

	err := runWithTimeout(cmd, time.Duration(s.Timeout)*time.Second)
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	//verifying outputs:
	if err != nil {
		errorMsg := fmt.Sprintf("R script execution failed: %v\nStderr: %s", err, stderr.String())
		log.Printf(errorMsg)
		return createFailedResult(analysisID, filePath, errorMsg), err
	}
	//
	if _, err := os.Stat(outputFile); err != nil {
		errorMsg := fmt.Sprintf("R script did not generate expected output file: %v", err)
		log.Printf(errorMsg)
		return createFailedResult(analysisID, filePath, errorMsg), errors.New(errorMsg)
	}

	// Success! Create the analysis result
	result := &DescriptiveAnalysisMetadata{
		AnalysisID:   analysisID,
		FilePath:     filePath,
		Status:       "success",
		OutputPath:   outputFile,
		StartTime:    startTime,
		EndTime:      endTime,
		Duration:     duration,
		Metadata: map[string]string{
			"fileType":     fileExt,
			"analysisType": "descriptive",
			"rScript":      scriptName,
			"rOutput":      stdout.String(),
		},
	}


	log.Printf("Analysis completed successfully for file: %s", filePath)
	log.Printf("Analysis duration: %v", duration)
	log.Printf("Output saved to: %s", outputFile)
	
	return result, nil
}

// message template in case the execution fails
func createFailedResult(analysisID, filePath, errorMessage string) *DescriptiveAnalysisMetadata {
	return &DescriptiveAnalysisMetadata{
		AnalysisID:   analysisID,
		FilePath:     filePath,
		Status:       "failed",
		OutputPath:   "",
		StartTime:    time.Now(),
		EndTime:      time.Now(),
		Duration:     0,
		ErrorMessage: errorMessage,
		Metadata: map[string]string{
			"fileType":     filepath.Ext(filePath),
			"analysisType": "descriptive",
		},
	}
}

// command line execution of Scripts
func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	//channel signals when the process finishes
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	//waiting on command line completion or timeout
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process after timeout: %v", err)
		}
		return errors.New("process timed out")
	}

}