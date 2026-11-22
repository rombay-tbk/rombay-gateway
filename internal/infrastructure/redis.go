package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is a lazily connected client used for readiness in Phase 1.
// Later phases store rate-limit counters, idempotency state, and webhook jobs here.
// Durability is limited to Redis AOF (appendfsync everysec): a crash can lose
// about one second of writes. Keys must not be evicted.
type Redis struct {
	client *redis.Client
}

// RedisOptions configures the Redis client.
type RedisOptions struct {
	Addr         string
	Password     string
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// NewRedis builds a client. The TCP connection is established on Check.
func NewRedis(opts RedisOptions) *Redis {
	timeout := opts.DialTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	read := opts.ReadTimeout
	if read <= 0 {
		read = timeout
	}
	write := opts.WriteTimeout
	if write <= 0 {
		write = timeout
	}
	return &Redis{client: redis.NewClient(&redis.Options{
		Addr:         opts.Addr,
		Password:     opts.Password,
		DialTimeout:  timeout,
		ReadTimeout:  read,
		WriteTimeout: write,
		MaxRetries:   0,
	})}
}

// Name identifies the readiness check.
func (r *Redis) Name() string { return "redis" }

// Check pings Redis. A failure means the process must not receive traffic.
func (r *Redis) Check(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Close releases the connection pool.
func (r *Redis) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}
