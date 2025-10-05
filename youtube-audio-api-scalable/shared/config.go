// shared/config.go
package shared

import (
	"log"
	"os"
	"strconv"
)

const (
	DefaultAPIGatewayPort = "8080"
	DefaultWorkerPort     = "8081" // Workers might have their own HTTP endpoint for health checks or admin
	DefaultMaxWorkers     = 3
	DefaultAdminToken     = "super-secret-admin-token-change-me" // CHANGE THIS IN PRODUCTION
)

// Config holds global configuration for the services
type Config struct {
	APIGatewayPort string
	WorkerPort     string
	MaxWorkers     int
	AdminToken     string
	// Database connection string, Queue connection string, S3 bucket name etc. would go here
	// For this example, we'll keep them simple as in-memory stubs
}

// LoadConfig loads configuration from environment variables or uses defaults
func LoadConfig() *Config {
	maxWorkersStr := os.Getenv("MAX_WORKERS")
	maxWorkers, err := strconv.Atoi(maxWorkersStr)
	if err != nil || maxWorkers <= 0 {
		maxWorkers = DefaultMaxWorkers
		log.Printf("INFO: MAX_WORKERS not set or invalid, using default: %d", maxWorkers)
	}

	return &Config{
		APIGatewayPort: os.Getenv("API_GATEWAY_PORT"),
		WorkerPort:     os.Getenv("WORKER_PORT"),
		MaxWorkers:     maxWorkers,
		AdminToken:     os.Getenv("ADMIN_TOKEN"),
	}
}
