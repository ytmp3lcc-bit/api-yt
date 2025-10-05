// shared/config.go
package shared

import (
	"log"
	"os"
	"strconv"
    "strings"
)

const (
	DefaultAPIGatewayPort = "8080"
	DefaultWorkerPort     = "8081" // Workers might have their own HTTP endpoint for health checks or admin
	DefaultMaxWorkers     = 3
	DefaultAdminToken     = "super-secret-admin-token-change-me" // CHANGE THIS IN PRODUCTION
    DefaultAllowedOrigins = "*"
    DefaultAllowedVideoHosts = "youtube.com,youtu.be"
    DefaultRateLimitRPM   = 300
    DefaultMaxVideoDurationSeconds = 1200 // 20 minutes
    DefaultQueueName      = "jobs"
)

// Config holds global configuration for the services
type Config struct {
	APIGatewayPort string
	WorkerPort     string
	MaxWorkers     int
	AdminToken     string
    // Redis (optional). If RedisAddr is empty, in-memory implementations are used.
    RedisAddr      string
    RedisPassword  string
    RedisDB        int
    // Queue configuration
    QueueName      string
    QueueMaxLength int
    // CORS and URL validation
    AllowedOrigins     []string
    AllowedVideoHosts  []string
    // Rate limiting (requests per minute per IP)
    RateLimitRPM int
    // Public base URL for API (used by worker for download link construction)
    PublicAPIBaseURL string
    // External binaries configuration
    YtDlpPath  string
    FFmpegPath string
    // Content limits
    MaxVideoDurationSeconds int
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

    // Redis
    redisDB := 0
    if v := os.Getenv("REDIS_DB"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n >= 0 {
            redisDB = n
        }
    }

    // Rate limit
    rateLimit := DefaultRateLimitRPM
    if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            rateLimit = n
        }
    }

    // Queue length (optional)
    queueMaxLen := 0
    if v := os.Getenv("QUEUE_MAX_LENGTH"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n >= 0 {
            queueMaxLen = n
        }
    }

    // Max video duration seconds
    maxDur := DefaultMaxVideoDurationSeconds
    if v := os.Getenv("MAX_VIDEO_DURATION_SECONDS"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            maxDur = n
        }
    }

    // Admin token defaulting
    adminToken := os.Getenv("ADMIN_TOKEN")
    if strings.TrimSpace(adminToken) == "" {
        adminToken = DefaultAdminToken
        log.Printf("WARN: ADMIN_TOKEN not set. Using default development token. DO NOT USE IN PRODUCTION.")
    }

    // Allowed origins and video hosts
    allowedOriginsCSV := os.Getenv("ALLOWED_ORIGINS")
    if strings.TrimSpace(allowedOriginsCSV) == "" {
        allowedOriginsCSV = DefaultAllowedOrigins
    }
    allowedOrigins := splitAndClean(allowedOriginsCSV)

    allowedHostsCSV := os.Getenv("ALLOWED_VIDEO_HOSTS")
    if strings.TrimSpace(allowedHostsCSV) == "" {
        allowedHostsCSV = DefaultAllowedVideoHosts
    }
    allowedVideoHosts := splitAndClean(allowedHostsCSV)

	return &Config{
		APIGatewayPort: os.Getenv("API_GATEWAY_PORT"),
		WorkerPort:     os.Getenv("WORKER_PORT"),
		MaxWorkers:     maxWorkers,
        AdminToken:     adminToken,
        RedisAddr:      os.Getenv("REDIS_ADDR"),
        RedisPassword:  os.Getenv("REDIS_PASSWORD"),
        RedisDB:        redisDB,
        QueueName:      valueOrDefault(os.Getenv("QUEUE_NAME"), DefaultQueueName),
        QueueMaxLength: queueMaxLen,
        AllowedOrigins:    allowedOrigins,
        AllowedVideoHosts: allowedVideoHosts,
        RateLimitRPM:      rateLimit,
        PublicAPIBaseURL:  os.Getenv("PUBLIC_API_BASE_URL"),
        YtDlpPath:         os.Getenv("YTDLP_PATH"),
        FFmpegPath:        os.Getenv("FFMPEG_PATH"),
        MaxVideoDurationSeconds: maxDur,
	}
}

// valueOrDefault returns fallback if s is empty
func valueOrDefault(s string, fallback string) string {
    if strings.TrimSpace(s) == "" {
        return fallback
    }
    return s
}

// splitAndClean splits a comma-separated list and trims spaces; empty entries are removed
func splitAndClean(csv string) []string {
    if strings.TrimSpace(csv) == "" {
        return []string{}
    }
    parts := strings.Split(csv, ",")
    var out []string
    for _, p := range parts {
        t := strings.TrimSpace(p)
        if t != "" {
            out = append(out, t)
        }
    }
    if len(out) == 0 {
        return []string{"*"}
    }
    return out
}
