package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisDB implements DatabaseClient using Redis as a key-value store
// Keys: job:<id> => JSON(Job)
// Sorted set for listing: jobs (score: createdAt unix)
type RedisDB struct {
	client *redis.Client
}

func NewRedisDB(client *redis.Client) *RedisDB {
	return &RedisDB{client: client}
}

func (r *RedisDB) jobKey(id string) string { return fmt.Sprintf("job:%s", id) }

func (r *RedisDB) CreateJob(job *Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := r.jobKey(job.ID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return fmt.Errorf("job with ID %s already exists", job.ID)
	}
	b, _ := json.Marshal(job)
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, b, 0)
	pipe.ZAdd(ctx, "jobs", redis.Z{Score: float64(job.CreatedAt.Unix()), Member: job.ID})
	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisDB) GetJob(jobID string) (*Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	val, err := r.client.Get(ctx, r.jobKey(jobID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("job with ID %s not found", jobID)
		}
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(val, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *RedisDB) UpdateJob(job *Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := r.jobKey(job.ID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("job with ID %s not found for update", job.ID)
	}
	b, _ := json.Marshal(job)
	return r.client.Set(ctx, key, b, 0).Err()
}

func (r *RedisDB) DeleteJob(jobID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := r.client.TxPipeline()
	pipe.Del(ctx, r.jobKey(jobID))
	pipe.ZRem(ctx, "jobs", jobID)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisDB) GetAllJobs() ([]*Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ids, err := r.client.ZRevRange(ctx, "jobs", 0, -1).Result()
	if err != nil {
		return nil, err
	}
	jobs := make([]*Job, 0, len(ids))
	for _, id := range ids {
		j, err := r.GetJob(id)
		if err == nil {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}
