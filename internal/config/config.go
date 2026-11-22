package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPPort          = "8080"
	defaultLogLevel          = "info"
	defaultShutdownTimeout   = 15 * time.Second
	defaultDependencyTimeout = 2 * time.Second
	defaultKafkaClientID     = "rombay-gateway"
)

// Config is the explicit process configuration loaded from the environment.
type Config struct {
	HTTPPort          string
	ListenAddr        string
	LogLevel          slog.Level
	ShutdownTimeout   time.Duration
	DependencyTimeout time.Duration
	RedisAddr         string
	RedisPassword     string
	KafkaBrokers      []string
	KafkaClientID     string
}

// Load reads configuration from getenv. REDIS_ADDR and KAFKA_BROKERS are
// required because readiness treats both as hard dependencies.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("config: getenv is nil")
	}

	port := valueOr(getenv, "HTTP_PORT", defaultHTTPPort)
	if err := validatePort(port); err != nil {
		return Config{}, err
	}

	levelName := valueOr(getenv, "LOG_LEVEL", defaultLogLevel)
	level, err := parseLevel(levelName)
	if err != nil {
		return Config{}, err
	}

	shutdown, err := parseDuration("SHUTDOWN_TIMEOUT", valueOr(getenv, "SHUTDOWN_TIMEOUT", defaultShutdownTimeout.String()))
	if err != nil {
		return Config{}, err
	}
	dependency, err := parseDuration("DEPENDENCY_TIMEOUT", valueOr(getenv, "DEPENDENCY_TIMEOUT", defaultDependencyTimeout.String()))
	if err != nil {
		return Config{}, err
	}

	redisAddr := strings.TrimSpace(getenv("REDIS_ADDR"))
	if redisAddr == "" {
		return Config{}, fmt.Errorf("config: REDIS_ADDR is required")
	}

	brokers, err := parseBrokers(getenv("KAFKA_BROKERS"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPPort:          port,
		ListenAddr:        ":" + port,
		LogLevel:          level,
		ShutdownTimeout:   shutdown,
		DependencyTimeout: dependency,
		RedisAddr:         redisAddr,
		RedisPassword:     getenv("REDIS_PASSWORD"),
		KafkaBrokers:      brokers,
		KafkaClientID:     valueOr(getenv, "KAFKA_CLIENT_ID", defaultKafkaClientID),
	}, nil
}

func valueOr(getenv func(string) string, key, fallback string) string {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func validatePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("config: HTTP_PORT must be an integer from 1 to 65535")
	}
	return nil
}

func parseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("config: LOG_LEVEL must be debug, info, warn, or error")
	}
}

func parseDuration(name, raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config: %s must be a positive duration", name)
	}
	return d, nil
}

func parseBrokers(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("config: KAFKA_BROKERS is required")
	}
	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		broker := strings.TrimSpace(part)
		if broker == "" {
			return nil, fmt.Errorf("config: KAFKA_BROKERS contains an empty broker")
		}
		brokers = append(brokers, broker)
	}
	return brokers, nil
}
