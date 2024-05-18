# surehub-prom-exporter

A Prometheus exporter for data from the SurePet surehub.io API documented by this [beta OpenAPI doc](https://app-api.beta.surehub.io/index.html).

I built this in order to pull data from my <https://www.surepetcare.com/en-gb/pet-feeder> to track when and how much my two cats are eating and push it into my Prometheus and Grafana stack. I wish I could have got the data out directly from the hub locally instead of bouncing it through the cloud API but the device firmwares are fairly locked down at this point. 

The container image is published to `ghcr.io/astromechza/surehub-prom-exporter:main`.

### Endpoints on port `8080`

- `/metrics` - the Prometheus endpoint
- `/alive` - liveness probe (is the web server running)
- `/ready` - readiness probe (did the last surehub poll succeed)

### Required environment variables

- `SUREHUB_EMAIL` - the email address you use to login to <https://surehub.io>
- `SUREHUB_PASSWORD` - the password you use to login to <https://surehub.io>

When deployed through the Score file to Kubernetes using `score-k8s` it is assumed that you have a secret named `surehub-credential` with `email` and `password` keys in it.

### Prometheus service monitor

If you are using the Prometheus operator, then you should also install a service monitor like:

```yaml
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: surehub-prom-exporter
  namespace: monitoring
spec:
  namespaceSelector:
    matchNames:
      - <namespace>
  selector:
    matchLabels:
      app.kubernetes.io/name: surehub-prom-exporter
      app.kubernetes.io/managed-by: score-k8s
  endpoints:
    - port: web
      path: /metrics
      relabelings:
      - regex: pod
        action: labeldrop
      metricRelabelings:
      - regex: instance
        action: labeldrop
```

### Example Prometheus metrics

```
TODO
```
