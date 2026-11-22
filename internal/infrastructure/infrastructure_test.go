package infrastructure

import (
	"context"
	"testing"
	"time"
)

func TestRedisPingRefused(t *testing.T) {
	t.Parallel()

	client := NewRedis(RedisOptions{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Check(ctx); err == nil {
		t.Fatal("expected redis ping to fail")
	}
	if client.Name() != "redis" {
		t.Fatalf("name = %s", client.Name())
	}
}

func TestKafkaMetadataRefused(t *testing.T) {
	t.Parallel()

	client, err := NewKafka(KafkaOptions{
		Brokers: []string{"127.0.0.1:1"},
		Timeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Check(ctx); err == nil {
		t.Fatal("expected kafka metadata to fail")
	}
}

func TestNewKafkaRejectsEmptyBrokers(t *testing.T) {
	t.Parallel()
	if _, err := NewKafka(KafkaOptions{}); err == nil {
		t.Fatal("expected error")
	}
}
