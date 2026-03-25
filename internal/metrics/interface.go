package metrics

import "context"

// MetricFetcher is the contract every metric source must implement.
// To add a new source (RabbitMQ, SQS, etc), just implement this interface.
type MetricFetcher interface {
	// Fetch returns the current metric value from the external source
	Fetch(ctx context.Context) (int64, error)
}
