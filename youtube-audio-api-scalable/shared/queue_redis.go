package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisQueue implements MessageQueueClient using Redis streams (XADD/XREAD)
// Stream: cfg.QueueName
// Consumer group could be added later for scaling workers.
type RedisQueue struct {
	client *redis.Client
	name   string
	maxLen int
}

func NewRedisQueue(client *redis.Client, name string, maxLen int) *RedisQueue {
	return &RedisQueue{client: client, name: name, maxLen: maxLen}
}

func (q *RedisQueue) Publish(message JobMessage) error {
	if q.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
    b, _ := json.Marshal(message)
    args := &redis.XAddArgs{Stream: q.name, Values: map[string]any{"data": b}}
    if q.maxLen > 0 {
        args.MaxLen = int64(q.maxLen)
        args.Approx = true
    }
    return q.client.XAdd(ctx, args).Err()
}

func (q *RedisQueue) Consume() (<-chan JobMessage, error) {
	out := make(chan JobMessage)
	if q.client == nil {
		close(out)
		return out, fmt.Errorf("redis client is nil")
	}
	go func() {
		defer close(out)
		ctx := context.Background()
		lastID := "$" // start from new messages
		for {
			res, err := q.client.XRead(ctx, &redis.XReadArgs{Streams: []string{q.name, lastID}, Block: 0, Count: 10}).Result()
			if err != nil {
				// on context cancel or close, exit
				return
			}
			for _, stream := range res {
				for _, msg := range stream.Messages {
					lastID = msg.ID
					if raw, ok := msg.Values["data"].(string); ok {
						var jm JobMessage
						if err := json.Unmarshal([]byte(raw), &jm); err == nil {
							out <- jm
						}
					}
				}
			}
		}
	}()
	return out, nil
}

func (q *RedisQueue) Close() {}
