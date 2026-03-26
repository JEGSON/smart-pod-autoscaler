# Smart Pod Autoscaler

[![Go Report Card](https://goreportcard.com/badge/github.com/JEGSON/smart-pod-autoscaler)](https://goreportcard.com/report/github.com/JEGSON/smart-pod-autoscaler)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Helm Chart](https://img.shields.io/badge/helm-chart-blue)](https://jegson.github.io/smart-pod-autoscaler)
[![Go Version](https://img.shields.io/badge/go-1.24-blue)](https://golang.org)

A production-grade **Kubernetes Operator** written in Go that scales
deployments based on external metrics — going beyond what the default
HPA supports.

## Why This Exists

The default Kubernetes HPA only scales on CPU and Memory. Real
workloads need to scale on **business metrics**:

- 📨 Kafka consumer group lag
- 📊 Custom Prometheus queries (p99 latency, error rate, request rate)
- 🐇 RabbitMQ queue depth *(coming soon)*
- ☁️ AWS SQS queue length *(coming soon)*

## Quick Start
```bash
# Add the Helm repo
helm repo add smart-scaler https://jegson.github.io/smart-pod-autoscaler
helm repo update

# Install the operator
helm install smart-scaler smart-scaler/smart-pod-autoscaler \
  --namespace smart-scaler \
  --create-namespace
```

## Usage

Create a `SmartScaler` resource pointing at your deployment:
```yaml
apiVersion: autoscaler.autoscaler.io/v1alpha1
kind: SmartScaler
metadata:
  name: worker-scaler
  namespace: default
spec:
  targetDeployment: queue-worker
  minReplicas: 2
  maxReplicas: 20
  metric:
    source: prometheus
    endpoint: "http://prometheus:9090"
    query: "sum(kafka_consumer_group_lag)"
  scalingPolicy:
    scaleUpThreshold: 1000   # scale up when lag > 1000 per replica
    scaleDownThreshold: 100  # scale down when lag < 100 per replica
    cooldownSeconds: 60      # wait 60s between scaling actions
```
```bash
kubectl apply -f smartscaler.yaml
kubectl get smartscalers
```

## How It Works
```
External Metric (Prometheus/Kafka)
        │
        ▼
SmartScaler Controller (Go)
        │
        ├── Fetch metric value
        ├── Calculate desired replicas
        ├── Apply cooldown window
        └── Patch Deployment replicas
```

The controller runs a reconcile loop every 30 seconds, fetches the
configured metric, and scales the target deployment up or down based
on your thresholds.

## Supported Metric Sources

| Source | Status | Config |
|--------|--------|--------|
| Prometheus | ✅ Stable | PromQL query |
| Kafka | ✅ Stable | Consumer group lag |
| RabbitMQ | 🔜 Coming soon | Queue depth |
| AWS SQS | 🔜 Coming soon | Queue length |

## Architecture
```
smart-pod-autoscaler/
├── api/v1alpha1/          ← CRD types (SmartScaler)
├── internal/
│   ├── controller/        ← reconcile loop
│   ├── metrics/           ← pluggable metric fetchers
│   └── webhook/           ← validation + defaulting
├── helm/                  ← Helm chart
└── monitoring/            ← Grafana dashboard + Prometheus config
```

## Validation

The operator validates your `SmartScaler` at apply time:

- `targetDeployment` must be set
- `minReplicas` must be ≥ 1
- `maxReplicas` must be > `minReplicas` and ≤ 100
- `metric.source` must be `prometheus` or `kafka`
- `scaleUpThreshold` must be > `scaleDownThreshold`

## Observability

The controller exposes Prometheus metrics at `:8080/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `smartscaler_scale_up_total` | Counter | Total scale up actions |
| `smartscaler_scale_down_total` | Counter | Total scale down actions |
| `smartscaler_current_replicas` | Gauge | Current replica count |
| `smartscaler_current_metric_value` | Gauge | Current metric value |
| `smartscaler_reconcile_errors_total` | Counter | Reconcile errors |

A pre-built Grafana dashboard is included in `monitoring/`.

## Helm Values

| Key | Default | Description |
|-----|---------|-------------|
| `replicaCount` | `2` | Controller replicas (HA) |
| `image.repository` | `ghcr.io/jegson/smart-pod-autoscaler` | Image |
| `image.tag` | `latest` | Image tag |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
| `metrics.enabled` | `true` | Expose Prometheus metrics |

## Contributing

Contributions are welcome! The easiest way to contribute is adding
a new metric source:

1. Create `internal/metrics/yoursource.go`
2. Implement the `MetricFetcher` interface:
```go
   type MetricFetcher interface {
       Fetch(ctx context.Context) (int64, error)
   }
```
3. Add a case in `fetchMetric()` in the controller
4. Add tests in `internal/metrics/yoursource_test.go`
5. Submit a PR!

## Roadmap

- [ ] RabbitMQ metric source
- [ ] AWS SQS metric source
- [ ] Predictive scaling (ML-based)
- [ ] Multi-metric scaling (combine sources)
- [ ] Grafana dashboard on Grafana Cloud
- [ ] Helm chart on Artifact Hub

## License

MIT — see [LICENSE](LICENSE)

## Author

Built by [JEGSON](https://github.com/JEGSON) — jegsonola.oj@gmail.com
