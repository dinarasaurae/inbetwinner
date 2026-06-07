# NFR Evidence Dashboard

This branch adds a single presentation-ready Grafana dashboard for non-functional requirement evidence:

- Availability: HTTP health probes for the gateway, application services and MinIO.
- Performance: API Gateway request rate, p95/p99 latency and health probe latency.
- Reliability: 5xx error rate, queue backlog and firing NFR alerts.
- Capacity: container CPU and memory saturation.
- Infrastructure: PostgreSQL exporter, Redis exporter and RabbitMQ built-in Prometheus plugin signals.

## Run locally

Start the application stack with the monitoring overlay:

```sh
make monitoring-up
```

Open Grafana:

```text
http://localhost:3007/d/inbetwin-nfr-evidence/inbetwin-nfr-evidence-board
```

Default credentials are `admin` / `admin`, and anonymous Viewer access is enabled for local presentation use.

## Run on the remote server

Deploy this branch or merge these files into the branch that is already deployed on the server. From the repository root on the server:

```sh
docker compose -f docker-compose.prod.yml -f docker-compose.monitoring.yml up -d
```

Grafana and Prometheus are bound to `127.0.0.1` by default, so open them through an SSH tunnel:

```sh
ssh -L 3007:127.0.0.1:3007 -L 9090:127.0.0.1:9090 user@server
```

Then open:

```text
http://localhost:3007/d/inbetwin-nfr-evidence/inbetwin-nfr-evidence-board
```

Check Prometheus targets on the server:

```sh
docker compose -f docker-compose.prod.yml -f docker-compose.monitoring.yml ps
curl -fsS http://127.0.0.1:9090/-/ready
```

## Capture evidence for a presentation

1. Start the stack with `make monitoring-up`.
2. Generate realistic traffic, for example through the API Gateway benchmark script or manual product scenarios.
3. Keep the Grafana time range on the observed test window.
4. Use the top row as the executive slide: availability, p95 latency, 5xx error rate and active NFR alerts.
5. Use the lower panels as supporting evidence for service health, gateway RED metrics, capacity and infrastructure state.

## Dashboard provisioning

Grafana provisions the dashboard from:

```text
monitoring/grafana/dashboards/inbetwin-nfr-evidence.json
```

Prometheus scrape and alert rules are in:

```text
monitoring/prometheus/prometheus.yml
monitoring/prometheus/rules/nfr-alerts.yml
```

The API Gateway exposes Prometheus metrics on `/metrics`. Prometheus scrapes this endpoint as the main user-facing RED metric source, because gateway traffic represents the public API surface.

## Evidence targets shown on the dashboard

- Availability: at least 99%.
- API Gateway p95 latency: at most 300 ms.
- Gateway 5xx error rate: at most 1%.
- Health probe latency: at most 500 ms.
- Container CPU warning: above 80% for 10 minutes.

These targets are presentation defaults. If the formal NFR document uses different thresholds, update the dashboard thresholds and `monitoring/prometheus/rules/nfr-alerts.yml` together.
