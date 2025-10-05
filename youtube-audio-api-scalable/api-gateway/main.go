// api-gateway/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"youtube-audio-api-scalable/shared" // Import shared package

	"github.com/google/uuid"
)

// Global instances for our conceptual database and message queue
var (
	cfg *shared.Config
	db  shared.DatabaseClient
	mq  shared.MessageQueueClient
)

func main() {
	cfg = shared.LoadConfig()
	if cfg.APIGatewayPort == "" {
		cfg.APIGatewayPort = shared.DefaultAPIGatewayPort
	}
	log.Printf("API Gateway starting on port %s", cfg.APIGatewayPort)

	// Initialize our conceptual in-memory database
	db = shared.NewInMemoryDB()
	log.Println("Initialized in-memory database.")

	// Initialize our conceptual in-memory message queue
	// A buffer size of 100 is chosen as an example. In production, this would be an external MQ.
	mq = shared.NewInMemoryQueue(100)
	defer mq.Close() // Ensure the queue is closed on shutdown
	log.Println("Initialized in-memory message queue.")

	http.HandleFunc("/extract", handleExtract)
	http.HandleFunc("/status/", handleStatus)
	http.HandleFunc("/health", handleHealth)

	// Admin endpoints (with a simple middleware for auth)
	adminRouter := http.NewServeMux()
	adminRouter.HandleFunc("/admin/jobs", handleAdminListJobs)
	adminRouter.HandleFunc("/admin/jobs/", handleAdminGetJob)
	adminRouter.HandleFunc("/admin/delete/", handleAdminDeleteJob)
	// adminRouter.HandleFunc("/admin/cache", handleAdminGetCache) // Cache endpoints for later
	// adminRouter.HandleFunc("/admin/cache/clear", handleAdminClearCache)

	http.Handle("/admin/", adminAuthMiddleware(adminRouter))

	fmt.Printf("🚀 API Gateway Server running on http://localhost:%s\n", cfg.APIGatewayPort)
	log.Fatal(http.ListenAndServe(":"+cfg.APIGatewayPort, nil))
}

// Enable CORS for browser requests
func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// adminAuthMiddleware provides a basic bearer token authentication for admin routes
func adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w) // CORS for admin too
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		token := r.Header.Get("Authorization")
		if token != "Bearer "+cfg.AdminToken { // Simple bearer token auth
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleExtract: Starts a job, pushes to queue, and returns immediately
func handleExtract(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var req shared.Request // Use shared.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "Missing YouTube URL", http.StatusBadRequest)
		return
	}

	jobID := uuid.New().String()
	now := time.Now()
	job := &shared.Job{ // Use shared.Job
		ID:          jobID,
		OriginalURL: req.URL,
		Status:      shared.JobStatusPending,
		CreatedAt:   now,
	}

	// 1. Store initial job status in DB
	if err := db.CreateJob(job); err != nil {
		log.Printf("ERROR: Failed to create job %s in DB: %v", jobID, err)
		http.Error(w, "Failed to initialize job", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Job %s created in DB with status %s", jobID, job.Status)

	// 2. Publish job to message queue
	jobMessage := shared.JobMessage{
		JobID:       jobID,
		OriginalURL: req.URL,
	}
	if err := mq.Publish(jobMessage); err != nil {
		log.Printf("ERROR: Failed to publish job %s to queue: %v", jobID, err)
		// Mark job as failed in DB since it couldn't be queued
		job.Status = shared.JobStatusFailed
		job.Error = fmt.Sprintf("Failed to queue job: %v", err)
		db.UpdateJob(job) // Attempt to update status in DB
		http.Error(w, "Failed to submit job to processing queue", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Job %s published to message queue", jobID)

	// 3. Respond immediately to client
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":       jobID,
		"status":       string(job.Status),
		"message":      "Audio extraction started. Check status at /status/" + jobID,
		"instructions": "A worker service will process this job and update its status. Polling /status/{job_id} is recommended.",
	})
	fmt.Printf("🎬 API Gateway received job %s for URL: %s\n", jobID, req.URL)
}

// handleStatus: Checks job status from the database
func handleStatus(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	jobID := filepath.Base(r.URL.Path) // Extract job ID from /status/{job_id}

	job, err := db.GetJob(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// handleHealth: Basic health check for the API Gateway
func handleHealth(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == http.MethodOptions {
		return
	}

	// In a real system, you'd check DB connection, MQ connection, etc.
	// For now, assume if the server is up, it's healthy.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "API Gateway is healthy",
	})
}

// handleAdminListJobs: Lists all jobs from the database
func handleAdminListJobs(w http.ResponseWriter, r *http.Request) {
	// Auth handled by middleware
	jobs, err := db.GetAllJobs()
	if err != nil {
		log.Printf("ERROR: Failed to get all jobs for admin: %v", err)
		http.Error(w, "Failed to retrieve jobs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

// handleAdminGetJob: Get details for a specific job from the database
func handleAdminGetJob(w http.ResponseWriter, r *http.Request) {
	// Auth handled by middleware
	jobID := filepath.Base(r.URL.Path) // Extract job ID from /admin/jobs/{job_id}

	job, err := db.GetJob(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// handleAdminDeleteJob: Deletes a job from the database and conceptually removes its file
func handleAdminDeleteJob(w http.ResponseWriter, r *http.Request) {
	// Auth handled by middleware
	if r.Method == http.MethodOptions {
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	jobID := filepath.Base(r.URL.Path) // Extract job ID from /admin/delete/{job_id}

	job, err := db.GetJob(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Conceptual file deletion (in a real system, this would interact with Object Storage)
	if job.FilePath != "" {
		// Attempt to delete the file from shared.OutputDir if it exists locally
		fullPath := filepath.Join(shared.OutputDir, jobID+".mp3")
		if _, statErr := os.Stat(fullPath); statErr == nil { // Check if file exists
			if rmErr := os.Remove(fullPath); rmErr != nil {
				log.Printf("WARN: Failed to delete local file %s for job %s: %v", fullPath, jobID, rmErr)
				// Don't fail the whole request, just log, as DB deletion is more critical
			} else {
				log.Printf("INFO: Deleted local file: %s", fullPath)
			}
		}
	}

	if err := db.DeleteJob(jobID); err != nil {
		log.Printf("ERROR: Failed to delete job %s from DB: %v", jobID, err)
		http.Error(w, "Failed to delete job", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Deleted job %s from DB", jobID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("Job %s and associated file (if existed) deleted successfully.", jobID),
	})
}
