// shared/db.go
package shared

import (
	"fmt"
	"sync"
)

// DatabaseClient is a conceptual interface for interacting with job data
type DatabaseClient interface {
	CreateJob(job *Job) error
	GetJob(jobID string) (*Job, error)
	UpdateJob(job *Job) error
	DeleteJob(jobID string) error
	GetAllJobs() ([]*Job, error) // For admin purposes
}

// InMemoryDB implements DatabaseClient using an in-memory map
type InMemoryDB struct {
	jobs      map[string]*Job
	jobsMutex sync.RWMutex
}

// NewInMemoryDB creates a new in-memory database instance
func NewInMemoryDB() *InMemoryDB {
	return &InMemoryDB{
		jobs: make(map[string]*Job),
	}
}

// CreateJob adds a new job to the database
func (db *InMemoryDB) CreateJob(job *Job) error {
	db.jobsMutex.Lock()
	defer db.jobsMutex.Unlock()

	if _, exists := db.jobs[job.ID]; exists {
		return fmt.Errorf("job with ID %s already exists", job.ID)
	}
	db.jobs[job.ID] = job
	return nil
}

// GetJob retrieves a job by its ID
func (db *InMemoryDB) GetJob(jobID string) (*Job, error) {
	db.jobsMutex.RLock()
	defer db.jobsMutex.RUnlock()

	job, exists := db.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job with ID %s not found", jobID)
	}
	// Return a copy to prevent external modification without UpdateJob
	copiedJob := *job
	return &copiedJob, nil
}

// UpdateJob updates an existing job in the database
func (db *InMemoryDB) UpdateJob(job *Job) error {
	db.jobsMutex.Lock()
	defer db.jobsMutex.Unlock()

	if _, exists := db.jobs[job.ID]; !exists {
		return fmt.Errorf("job with ID %s not found for update", job.ID)
	}
	db.jobs[job.ID] = job
	return nil
}

// DeleteJob removes a job from the database
func (db *InMemoryDB) DeleteJob(jobID string) error {
	db.jobsMutex.Lock()
	defer db.jobsMutex.Unlock()

	if _, exists := db.jobs[jobID]; !exists {
		return fmt.Errorf("job with ID %s not found for deletion", jobID)
	}
	delete(db.jobs, jobID)
	return nil
}

// GetAllJobs retrieves all jobs (for admin/monitoring)
func (db *InMemoryDB) GetAllJobs() ([]*Job, error) {
	db.jobsMutex.RLock()
	defer db.jobsMutex.RUnlock()

	allJobs := make([]*Job, 0, len(db.jobs))
	for _, job := range db.jobs {
		copiedJob := *job
		allJobs = append(allJobs, &copiedJob)
	}
	return allJobs, nil
}
