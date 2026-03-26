# Smart Pod Autoscaler

A production-grade Kubernetes operator written in Go that scales deployments
based on external metrics — Prometheus and Kafka consumer group lag.

## Install
```bash
helm repo add smart-scaler https://JEGSON.github.io/smart-pod-autoscaler
helm repo update
helm install smart-scaler smart-scaler/smart-pod-autoscaler \
  --namespace smart-scaler \
  --create-namespace
```

## Usage

After installing, create a SmartScaler resource:
```yaml
apiVersion: autoscaler.autoscaler.io/v1alpha1
kind: SmartScaler
metadata:
  name: my-scaler
spec:
  targetDeployment: my-deployment
  minReplicas: 2
  maxReplicas: 20
  metric:
    source: prometheus
    endpoint: "http://prometheus:9090"
    query: "sum(http_requests_total)"
  scalingPolicy:
    scaleUpThreshold: 1000
    scaleDownThreshold: 100
    cooldownSeconds: 60
```

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `replicaCount` | `2` | Controller replicas |
| `image.repository` | `ghcr.io/JEGSON/smart-pod-autoscaler` | Image repo |
| `image.tag` | `latest` | Image tag |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
| `metrics.enabled` | `true` | Expose Prometheus metrics | 
