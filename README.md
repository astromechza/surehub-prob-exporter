# surehub-prob-exporter

A prometheus exporter for data from the SurePet surehub.io API.

The container image is published to `ghcr.io/astromechza/surehub-prom-exporter:main`.

### Endpoints on port `8080`

- `/metrics` - the Prometheus endpoint
- `/alive` - liveness probe (is the web server running)
- `/ready` - readiness probe (did the last surehub poll succeed)

### Required environment variables

- `SUREHUB_EMAIL` - the email address you use to login to <https://surehub.io>
- `SUREHUB_PASSWORD` - the password you use to login to <https://surehub.io>

When deployed through the Score file to Kubernetes using `score-k8s` it is assumed that you have a secret named `surehub-credential` with `email` and `password` keys in it.
