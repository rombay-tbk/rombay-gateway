package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(func(key string) string {
		switch key {
		case "REDIS_ADDR":
			return "localhost:6379"
		case "KAFKA_BROKERS":
			return " localhost:9092, kafka:9092 "
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != "8080" || cfg.ListenAddr != ":8080" {
		t.Fatalf("http listen = %s %s", cfg.HTTPPort, cfg.ListenAddr)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("level = %v", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 15*time.Second || cfg.DependencyTimeout != 2*time.Second {
		t.Fatalf("timeouts = %s %s", cfg.ShutdownTimeout, cfg.DependencyTimeout)
	}
	if cfg.KafkaClientID != "rombay-gateway" {
		t.Fatalf("client id = %s", cfg.KafkaClientID)
	}
	if len(cfg.KafkaBrokers) != 2 || cfg.KafkaBrokers[0] != "localhost:9092" || cfg.KafkaBrokers[1] != "kafka:9092" {
		t.Fatalf("brokers = %#v", cfg.KafkaBrokers)
	}
}

func TestLoadRequiresDependencies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		env  map[string]string
	}{
		{name: "redis", env: map[string]string{"KAFKA_BROKERS": "localhost:9092"}},
		{name: "kafka", env: map[string]string{"REDIS_ADDR": "localhost:6379"}},
		{name: "empty broker", env: map[string]string{"REDIS_ADDR": "localhost:6379", "KAFKA_BROKERS": "localhost:9092,,kafka:9092"}},
		{name: "port", env: map[string]string{"REDIS_ADDR": "localhost:6379", "KAFKA_BROKERS": "localhost:9092", "HTTP_PORT": "0"}},
		{name: "level", env: map[string]string{"REDIS_ADDR": "localhost:6379", "KAFKA_BROKERS": "localhost:9092", "LOG_LEVEL": "verbose"}},
		{name: "timeout", env: map[string]string{"REDIS_ADDR": "localhost:6379", "KAFKA_BROKERS": "localhost:9092", "SHUTDOWN_TIMEOUT": "0s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(func(key string) string { return tc.env[key] })
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadNilGetenv(t *testing.T) {
	t.Parallel()
	if _, err := Load(nil); err == nil {
		t.Fatal("expected error")
	}
}
