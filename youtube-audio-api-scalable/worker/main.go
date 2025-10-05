// worker/main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"youtube-audio-api-scalable/shared" // Import shared package
)

// Global instances for our conceptual database and message queue
var (
	cfg           *shared.Config
	db            shared.DatabaseClient
	mq            shared.MessageQueueClient
	workerLimiter chan struct{} // Semaphore to limit concurrent processing tasks
)

func main() {
	cfg = shared.LoadConfig()
	if cfg.WorkerPort == "" {
		cfg.WorkerPort = shared.DefaultWorkerPort
	}
	log.Printf("Worker Service starting on port %s with %d max concurrent jobs", cfg.WorkerPort, cfg.MaxWorkers)

	// Initialize our conceptual in-memory database (must be the same instance as API Gateway for this example)
	// In a real distributed system, workers would connect to a persistent, central DB.
	db = shared.NewInMemoryDB()
	log.Println("Initialized conceptual in-memory database for worker (NOTE: this should be a shared persistent DB in prod).")

	// Initialize our conceptual in-memory message queue (must be the same instance as API Gateway for this example)
	mq = shared.NewInMemoryQueue(100)
	defer mq.Close()
	log.Println("Initialized conceptual in-memory message queue for worker (NOTE: this should be a shared external MQ in prod).")

	// Create a buffered channel to act as a semaphore for limiting concurrent workers
	workerLimiter = make(chan struct{}, cfg.MaxWorkers)

	// Start consuming messages from the queue in a goroutine
	go startQueueConsumer()

	// --- Worker Service HTTP Endpoints (e.g., for health checks or admin) ---
	http.HandleFunc("/health", handleHealth)

	fmt.Printf("⚙️ Worker Service running on http://localhost:%s\n", cfg.WorkerPort)
	log.Fatal(http.ListenAndServe(":"+cfg.WorkerPort, nil))
}

// startQueueConsumer continuously consumes messages from the queue
func startQueueConsumer() {
	messages, err := mq.Consume()
	if err != nil {
		log.Fatalf("FATAL: Failed to start consuming from queue: %v", err)
	}
	log.Println("INFO: Worker started consuming messages from queue...")

	for msg := range messages {
		// Acquire a token from the limiter channel. This will block if MaxWorkers are already busy.
		workerLimiter <- struct{}{}
		log.Printf("INFO: Worker acquired token for job %s. Current active jobs: %d/%d", msg.JobID, len(workerLimiter), cfg.MaxWorkers)

		// Process the job in a new goroutine so the consumer doesn't block
		go func(jobMessage shared.JobMessage) {
			defer func() {
				// Release the token back to the limiter channel when the job is done
				<-workerLimiter
				log.Printf("INFO: Worker released token for job %s. Remaining active jobs: %d/%d", jobMessage.JobID, len(workerLimiter), cfg.MaxWorkers)
			}()
			processJob(jobMessage)
		}(msg)
	}
	log.Println("INFO: Queue consumer stopped.")
}

// processJob executes yt-dlp and ffmpeg for a specific job
func processJob(jobMessage shared.JobMessage) {
	jobID := jobMessage.JobID
	originalURL := jobMessage.OriginalURL
	log.Printf("🛠️ Worker processing job %s for URL: %s", jobID, originalURL)

	// Retrieve job from DB to get its current state (optional, but good practice)
	job, err := db.GetJob(jobID)
	if err != nil {
		log.Printf("ERROR: Worker failed to retrieve job %s from DB: %v", jobID, err)
		// Try to log/handle, but can't update status without the job
		return
	}

	// Update job status to processing
	now := time.Now()
	job.Status = shared.JobStatusProcessing
	job.StartedAt = &now
	if err := db.UpdateJob(job); err != nil {
		log.Printf("ERROR: Worker failed to update job %s status to Processing in DB: %v", jobID, err)
		// Continue processing, but DB might be inconsistent
	}

	// --- Step 1: Extract direct audio stream URL via yt-dlp ---
	audioURL, meta, ytDlpErr := getAudioStream(originalURL)
	if ytDlpErr != nil {
		handleJobFailure(job, fmt.Sprintf("yt-dlp failed: %v", ytDlpErr))
		return
	}
	log.Printf("INFO: Job %s - Audio stream extracted successfully: %s", jobID, audioURL)

	// --- Step 2: Convert stream to MP3 file using ffmpeg ---
	filePath, ffmpegErr := convertToMP3(audioURL, jobID) // Pass jobID for consistent naming
	if ffmpegErr != nil {
		handleJobFailure(job, fmt.Sprintf("ffmpeg failed: %v", ffmpegErr))
		return
	}
	log.Printf("INFO: Job %s - Conversion completed successfully: %s", jobID, filePath)

	// --- Step 3: Job completed successfully - Update DB ---
	completedNow := time.Now()
	job.Status = shared.JobStatusCompleted
	job.Metadata = meta
	job.FilePath = filePath
	job.DownloadEndpoint = fmt.Sprintf("http://localhost:%s/download/%s", cfg.APIGatewayPort, jobID) // Point to API Gateway's download endpoint
	job.CompletedAt = &completedNow

	if err := db.UpdateJob(job); err != nil {
		log.Printf("ERROR: Worker failed to update job %s status to Completed in DB: %v", jobID, err)
		// If DB update fails, the job might remain "processing" or get stuck. Requires monitoring.
	} else {
		log.Printf("✅ Job %s completed. Download endpoint: %s", jobID, job.DownloadEndpoint)
	}
}

// handleJobFailure updates a job's status to failed in the database
func handleJobFailure(job *shared.Job, errMsg string) {
	failedNow := time.Now()
	job.Status = shared.JobStatusFailed
	job.Error = errMsg
	job.CompletedAt = &failedNow // Mark completion time even for failures
	if err := db.UpdateJob(job); err != nil {
		log.Printf("ERROR: Worker failed to update job %s status to Failed in DB: %v", job.ID, err)
	}
	log.Printf("❌ Job %s failed: %s", job.ID, errMsg)
}

// getAudioStream: Retrieves audio stream URL and metadata using yt-dlp
func getAudioStream(videoURL string) (string, *shared.Metadata, error) {
	cmd := exec.Command("./yt-dlp", "-f", "bestaudio", "--dump-single-json", "--no-warnings", videoURL)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("yt-dlp failed: %v\nOutput: %s", err, out.String())
	}

	// Temporary struct to unmarshal yt-dlp's output
	var data struct {
		Title    string  `json:"title"`
		Uploader string  `json:"uploader"`
		Duration float64 `json:"duration"`
		URL      string  `json:"url"` // This is the direct audio stream URL
		Ext      string  `json:"ext"`
		Abr      int     `json:"abr"`
	}

	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return "", nil, fmt.Errorf("JSON parse error: %v\nOutput: %s", err, out.String())
	}

	// Assign to our Metadata struct
	meta := &shared.Metadata{
		Title:    data.Title,
		Uploader: data.Uploader,
		Duration: data.Duration,
		AudioURL: data.URL, // Assign the direct stream URL here
		Ext:      data.Ext,
		Abr:      data.Abr,
	}

	return data.URL, meta, nil
}

// convertToMP3: Converts audio stream URL to MP3 file, uses jobID for naming
func convertToMP3(audioURL string, jobID string) (string, error) {
	outputDir := shared.OutputDir
	outputPath := filepath.Join(outputDir, jobID+".mp3")

	// Ensure output directory exists (created by API Gateway already, but good for resilience)
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	start := time.Now()

	cmd := exec.Command("./ffmpeg", "-y", "-i", audioURL, "-vn", "-ab", "192k", "-ar", "44100", "-f", "mp3", outputPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, out.String())
	}

	elapsed := time.Since(start)
	log.Printf("⏱️ Conversion time for job %s: %.2fs", jobID, elapsed.Seconds())

	return outputPath, nil
}

// handleHealth: Basic health check for the Worker Service
func handleHealth(w http.ResponseWriter, r *http.Request) {
	// CORS for health endpoint
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// In a real system, you'd check DB/MQ connections and if workers are actively processing
	status := "ok"
	message := "Worker Service is healthy and consuming from queue."
	if len(workerLimiter) == cfg.MaxWorkers {
		message = "Worker Service is healthy but all workers are currently busy."
	}
	// (Optional: Check if the message queue connection is active)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":         status,
		"message":        message,
		"active_workers": fmt.Sprintf("%d/%d", len(workerLimiter), cfg.MaxWorkers),
	})
}
