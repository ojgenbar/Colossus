# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Colossus is an over-engineered thumbnail processing app demonstrating microservices architecture with event-driven processing. A user uploads an image via the UI/API, it's stored in MinIO (S3), a Kafka message triggers the converter to create a thumbnail, and the result is stored back in MinIO.

## Architecture

```
UI (Flutter/Nginx) → Backend (Go/Gin) → MinIO (raw bucket)
                                       → Kafka (raw_queue topic)
                                              ↓
                              Converter (Go) → MinIO (processed bucket)
```

**Three application services:**
- **backend/** — Go REST API (Gin). Accepts image uploads, stores in S3, publishes to Kafka. Ports: 10001 (API), 20001 (metrics).
- **converter/** — Go worker. Consumes Kafka messages, downloads raw images, resizes by factor k=2 (nearest neighbor), uploads thumbnails. Port: 20002 (metrics).
- **ui/** — Flutter web app served by Nginx. Reverse proxies `/api/*` to backend. Supports upload, lookup by UUID, side-by-side raw/processed comparison.

**Infrastructure dependencies:** MinIO (S3), Kafka + Zookeeper, VictoriaMetrics, Grafana, VMAgent, VMAlert, Alertmanager, KMinion, Kowl.

## Build Commands

### Go services (backend, converter)
```bash
cd backend && go build -ldflags '-extldflags "-static"' -o /backend
cd converter && go build -ldflags '-extldflags "-static"' -o /converter
```

### Flutter UI
```bash
cd ui && flutter pub get && flutter build web --release
```

### Docker
```bash
docker build -t ojgen/colossus-backend:latest backend/
docker build -t ojgen/colossus-converter:latest converter/
docker build -t ojgen/colossus-ui:latest ui/
```

### Running locally (Docker Compose — deprecated in favor of K8s)
```bash
cp example_configs/.env example_configs/* ./
docker-compose up -d --build
```

### Kubernetes
Production manifests in `manifests/`, dev resources in `manifests-dev/` (secrets, PVs).

## Configuration

Both Go services use environment variables mapped to YAML config files. Example configs are in `example_configs/`. Key env vars:

- `S3_AUTH_ENDPOINT`, `S3_AUTH_ACCESS_KEY_ID`, `S3_AUTH_SECRET_ACCESS_KEY` — MinIO connection
- `S3_BUCKETS_RAW_BUCKET_NAME`, `S3_BUCKETS_PROCESSED_BUCKET_NAME` — bucket names
- `KAFKA_BOOTSTRAP_SERVERS`, `KAFKA_TOPIC_NAME` / `KAFKA_TOPIC` — Kafka connection
- `SERVERS_MAIN_ADDR` (backend), `SERVERS_SYSTEM_ADDR` (converter) — listen addresses

## Key Ports

| Service | Port | Purpose |
|---------|------|---------|
| Backend API | 10001 | REST API |
| MinIO API | 10101 | S3 API |
| MinIO Console | 10102 | Web UI |
| Kafka | 10103 | Broker |
| Kowl | 10104 | Kafka UI |
| VictoriaMetrics | 10105 | Metrics DB |
| Grafana | 10106 | Dashboards |

## Monitoring

All services export Prometheus metrics. VMAgent scrapes every 10s and forwards to VictoriaMetrics. Grafana has pre-configured dashboards for all services. VMAlert handles alerting rules.

## Code Patterns

- Go services use multi-stage Docker builds with distroless final images
- K8s deployments run non-root with read-only filesystems
- UUID-based filenames: `{uuid}-raw.{ext}` and `{uuid}-processed.{ext}`
- Graceful shutdown via SIGINT/SIGTERM signal handling
- Image format support: JPEG, PNG, GIF, BMP, TIFF
