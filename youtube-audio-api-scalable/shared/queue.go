// shared/queue.go
package shared

import (
	"fmt"
	"log"
	"sync"
)

// JobMessage represents the data sent through the queue for a job
type JobMessage struct {
	JobID       string
	OriginalURL string
}

// MessageQueueClient is a conceptual interface for a message queue
type MessageQueueClient interface {
	Publish(message JobMessage) error
	Consume() (<-chan JobMessage, error)
	Close() // In a real queue, this would close connections
}

// InMemoryQueue implements MessageQueueClient using a Go channel
type InMemoryQueue struct {
	queue chan JobMessage
	stop  chan struct{}
	once  sync.Once
}

// NewInMemoryQueue creates a new in-memory queue instance
func NewInMemoryQueue(bufferSize int) *InMemoryQueue {
	return &InMemoryQueue{
		queue: make(chan JobMessage, bufferSize),
		stop:  make(chan struct{}),
	}
}

// Publish sends a message to the queue
func (q *InMemoryQueue) Publish(message JobMessage) error {
	select {
	case q.queue <- message:
		log.Printf("Queue: Published job %s", message.JobID)
		return nil
	case <-q.stop:
		return fmt.Errorf("queue is closed, cannot publish")
	default:
		return fmt.Errorf("queue is full, cannot publish job %s", message.JobID)
	}
}

// Consume returns a channel from which messages can be received
func (q *InMemoryQueue) Consume() (<-chan JobMessage, error) {
	return q.queue, nil
}

// Close stops the queue from accepting new messages and closes the underlying channel
func (q *InMemoryQueue) Close() {
	q.once.Do(func() {
		log.Println("Queue: Closing...")
		close(q.stop)
		// Give some time for consumers to finish processing existing messages if needed
		// For simplicity, we just close the channel directly here
		close(q.queue)
	})
}
