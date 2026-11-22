package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

// Kafka is a cluster client used to prove broker connectivity.
// Phase 1 does not join a consumer group and does not create topics.
// rombay-core writes payment.created into outbox_events and has no relay yet,
// so there is no published event for this client to consume.
type Kafka struct {
	client    *kafka.Client
	transport *kafka.Transport
}

// KafkaOptions configures the Kafka client.
type KafkaOptions struct {
	Brokers  []string
	ClientID string
	Timeout  time.Duration
}

// NewKafka builds a client. Metadata is requested on Check.
func NewKafka(opts KafkaOptions) (*Kafka, error) {
	if len(opts.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: at least one broker is required")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	clientID := opts.ClientID
	if clientID == "" {
		clientID = "rombay-gateway"
	}
	transport := &kafka.Transport{
		ClientID:    clientID,
		DialTimeout: timeout,
	}
	return &Kafka{
		transport: transport,
		client: &kafka.Client{
			Addr:      kafka.TCP(opts.Brokers...),
			Transport: transport,
			Timeout:   timeout,
		},
	}, nil
}

// Name identifies the readiness check.
func (k *Kafka) Name() string { return "kafka" }

// Check requests cluster metadata. An empty broker list is treated as failure.
func (k *Kafka) Check(ctx context.Context) error {
	res, err := k.client.Metadata(ctx, &kafka.MetadataRequest{})
	if err != nil {
		return fmt.Errorf("kafka metadata: %w", err)
	}
	if res == nil || len(res.Brokers) == 0 {
		return fmt.Errorf("kafka metadata: no brokers")
	}
	return nil
}

// Close drops idle broker connections.
func (k *Kafka) Close() error {
	if k == nil || k.transport == nil {
		return nil
	}
	k.transport.CloseIdleConnections()
	return nil
}
