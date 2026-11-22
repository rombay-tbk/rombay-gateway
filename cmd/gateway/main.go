package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rombay-tbk/rombay-gateway/internal/app"
	"github.com/rombay-tbk/rombay-gateway/internal/config"
	"github.com/rombay-tbk/rombay-gateway/internal/infrastructure"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	redisClient := infrastructure.NewRedis(infrastructure.RedisOptions{
		Addr:        cfg.RedisAddr,
		Password:    cfg.RedisPassword,
		DialTimeout: cfg.DependencyTimeout,
	})
	kafkaClient, err := infrastructure.NewKafka(infrastructure.KafkaOptions{
		Brokers:  cfg.KafkaBrokers,
		ClientID: cfg.KafkaClientID,
		Timeout:  cfg.DependencyTimeout,
	})
	if err != nil {
		_ = redisClient.Close()
		return err
	}

	runner, err := app.Start(app.Options{
		ListenAddr:        cfg.ListenAddr,
		Logger:            logger,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		DependencyTimeout: cfg.DependencyTimeout,
		Dependencies:      []app.Dependency{redisClient, kafkaClient},
	})
	if err != nil {
		return errors.Join(err, redisClient.Close(), kafkaClient.Close())
	}

	logger.Info("gateway listening",
		"addr", runner.Addr(),
		"redis", cfg.RedisAddr,
		"kafka_brokers", cfg.KafkaBrokers,
	)
	if err := runner.Wait(ctx); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}
