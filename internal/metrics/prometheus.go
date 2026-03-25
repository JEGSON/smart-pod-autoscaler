package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// PrometheusFetcher fetches a metric value using a PromQL query
type PrometheusFetcher struct {
	api   prometheusv1.API
	query string
}

// NewPrometheusFetcher creates a new PrometheusFetcher
func NewPrometheusFetcher(endpoint, query string) (*PrometheusFetcher, error) {
	client, err := prometheusapi.NewClient(prometheusapi.Config{
		Address: endpoint, // e.g "http://prometheus:9090"
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &PrometheusFetcher{
		api:   prometheusv1.NewAPI(client),
		query: query,
	}, nil
}

// Fetch executes the PromQL query and returns the result as int64
func (p *PrometheusFetcher) Fetch(ctx context.Context) (int64, error) {
	// Query Prometheus at the current time
	result, warnings, err := p.api.Query(ctx, p.query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("prometheus query failed: %w", err)
	}

	// Log warnings if any (non fatal)
	if len(warnings) > 0 {
		fmt.Printf("Prometheus warnings: %v\n", warnings)
	}

	// The result is a Vector — we expect a single value back
	vector, ok := result.(model.Vector)
	if !ok {
		return 0, fmt.Errorf("unexpected prometheus result type: %T", result)
	}

	if len(vector) == 0 {
		// No data returned — treat as 0 (nothing to scale for)
		return 0, nil
	}

	// Take the first sample value and convert to int64
	value := int64(vector[0].Value)
	return value, nil
}
