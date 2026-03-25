package metrics

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
)

// KafkaFetcher fetches consumer group lag from a Kafka broker
type KafkaFetcher struct {
	client        sarama.Client
	admin         sarama.ClusterAdmin
	topic         string
	consumerGroup string
}

// NewKafkaFetcher creates a new KafkaFetcher
func NewKafkaFetcher(brokerURL, topic, consumerGroup string) (*KafkaFetcher, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	// Connect to the broker
	client, err := sarama.NewClient([]string{brokerURL}, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kafka: %w", err)
	}

	// Create admin client to query consumer group offsets
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka admin: %w", err)
	}

	return &KafkaFetcher{
		client:        client,
		admin:         admin,
		topic:         topic,
		consumerGroup: consumerGroup,
	}, nil
}

// Fetch calculates total consumer group lag across all partitions
func (k *KafkaFetcher) Fetch(ctx context.Context) (int64, error) {
	// Get the latest offset per partition from the broker
	partitions, err := k.client.Partitions(k.topic)
	if err != nil {
		return 0, fmt.Errorf("failed to get partitions: %w", err)
	}

	// Get the consumer group committed offsets
	offsetMap, err := k.admin.ListConsumerGroupOffsets(k.consumerGroup, map[string][]int32{
		k.topic: partitions,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get consumer group offsets: %w", err)
	}

	// Calculate total lag = latest offset - committed offset per partition
	var totalLag int64
	for _, partition := range partitions {
		latestOffset, err := k.client.GetOffset(k.topic, partition, sarama.OffsetNewest)
		if err != nil {
			return 0, fmt.Errorf("failed to get latest offset for partition %d: %w", partition, err)
		}

		block := offsetMap.GetBlock(k.topic, partition)
		if block == nil {
			continue
		}

		committedOffset := block.Offset
		lag := latestOffset - committedOffset
		if lag < 0 {
			lag = 0
		}
		totalLag += lag
	}

	return totalLag, nil
}

// Close cleans up Kafka connections
func (k *KafkaFetcher) Close() error {
	if err := k.admin.Close(); err != nil {
		return err
	}
	return k.client.Close()
}
