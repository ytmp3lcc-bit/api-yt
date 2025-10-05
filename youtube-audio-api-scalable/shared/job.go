// shared/job.go
package shared

import (
	"time"
)

// Metadata structure for response
type Metadata struct {
	Title    string  `json:"title"`
	Uploader string  `json:"uploader"`
	Duration float64 `json:"duration"`
	AudioURL string  `json:"audio_url"` // Direct audio stream URL from yt-dlp
	Ext      string  `json:"ext"`
	Abr      int     `json:"abr"`
}

type Request struct {
	URL string `json:"url"`
}

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// Job represents the state of an audio extraction and conversion task
type Job struct {
	ID               string     `json:"job_id"`
	OriginalURL      string     `json:"original_url"` // The YouTube URL submitted by the user
	Status           JobStatus  `json:"status"`
	Metadata         *Metadata  `json:"metadata,omitempty"`
	DownloadEndpoint string     `json:"download_endpoint,omitempty"` // URL to the converted MP3
	Error            string     `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	FilePath         string     `json:"-"` // Internal path to the file, not exposed via API
}
