// internal/config/config.go
package config

import (
	"github.com/kelseyhightower/envconfig"
)

// All config for the application
type Config struct {
	RabbitMQ    RabbitMQConfig    `envconfig:"RABBITMQ"`
	S3          S3Config          `envconfig:"S3"`
	Redis       RedisConfig       `envconfig:"REDIS"`
	FileWatcher FileWatcherConfig `envconfig:"FILEWATCHER"`
	Analysis AnalysisConfig `envconfig:"ANALYSIS"`
}

//TODO: change configs once RabbitMQ is configurated
type RabbitMQConfig struct {
	URI      string `envconfig:"URI" default:"amqp://guest:guest@localhost:5672/"`
	Exchange string `envconfig:"EXCHANGE" default:"biomarker"`
}

//TODO - confirm S3 file upload location
type S3Config struct {
	Bucket    string `envconfig:"BUCKET" default:"gsso-analyses"`
	Region    string `envconfig:"REGION" default:"us-west-2"`
	AccessKey string `envconfig:"ACCESS_KEY"`
	SecretKey string `envconfig:"SECRET_KEY"`
}

// CURRENTLY DEFAULT FIELDS - change once redis is configured
type RedisConfig struct {
	Addr     string `envconfig:"ADDR" default:"localhost:6379"`
	Password string `envconfig:"PASSWORD" default:""`
	DB       int    `envconfig:"DB" default:"0"`
}

//stores config for which folders to watch and how often - currently default
type FileWatcherConfig struct {
	Directories        []string `envconfig:"DIRECTORIES" default:"/tmp/FOLDER-TO-BE-NAMED"`
	SupportedExtensions []string `envconfig:"SUPPORTED_EXTENSIONS" default:".csv,.sas7bdat"`
	PollInterval       int      `envconfig:"POLL_INTERVAL" default:"5"` // in seconds
}

type AnalysisConfig struct {
	RExecutable  string `envconfig:"R_EXECUTABLE"` // Path to R executable (empty to auto-detect)
	ScriptsDir   string `envconfig:"SCRIPTS_DIR" default:"./scripts/r"` // Directory containing R scripts
	Timeout      int    `envconfig:"TIMEOUT" default:"300"` // Timeout in seconds
	OutputDir    string `envconfig:"OUTPUT_DIR" default:""` // Output directory (empty for system temp)
	RetainOutput bool   `envconfig:"RETAIN_OUTPUT" default:"true"` // Whether to keep output files after upload
}

// BIOMARKER prefix will be applied to all .env variables.
// e.g. setting RabbitMQ uri: -> BIOMARKER_RABBITMQ_URI
func Load() (*Config, error) {
	var cfg Config
	err := envconfig.Process("BIOMARKER", &cfg)
	return &cfg, err
}