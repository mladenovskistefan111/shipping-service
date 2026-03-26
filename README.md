# shipping-service

A gRPC service that handles shipping quotes and order dispatch for the platform-demo e-commerce platform. It calculates shipping costs based on item count and generates tracking IDs for shipped orders. Part of a broader microservices platform built with full observability, GitOps, and internal developer platform tooling.

## Overview

The service exposes two gRPC methods:

| Method | Description |
|---|---|
| `GetQuote` | Returns a shipping cost in USD based on the number of items in the cart |
| `ShipOrder` | Mocks dispatching an order to an address and returns a tracking ID |

**Port:** `50051` (gRPC)  
**Metrics Port:** `9090` (Prometheus + pprof)  
**Protocol:** gRPC  
**Language:** Go  
**Called by:** `checkout-service`

## Requirements

- Go 1.25+
- Docker
- `grpcurl` for manual testing

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `PORT` | No | gRPC server port (default: `50051`) |
| `METRICS_PORT` | No | Prometheus + pprof HTTP port (default: `9090`) |
| `ENABLE_TRACING` | No | Set to `1` to enable OpenTelemetry tracing |
| `COLLECTOR_SERVICE_ADDR` | No | OTLP gRPC collector address e.g. `alloy:4317` (required if tracing enabled) |
| `ENABLE_PROFILING` | No | Set to `1` to enable Pyroscope continuous profiling |
| `PYROSCOPE_ADDR` | No | Pyroscope endpoint (default: `http://pyroscope:4040`) |
| `OTEL_SERVICE_NAME` | No | Service name reported to OTel (default: `shipping-service`) |

## Running Locally

### 1. Build and run

```bash
go build ./...
go run ./cmd/server
```

### 2. Run with Docker

```bash
docker build -t shipping-service .

docker run -p 50051:50051 -p 9090:9090 \
  -e ENABLE_TRACING=1 \
  -e COLLECTOR_SERVICE_ADDR=alloy:4317 \
  -e ENABLE_PROFILING=1 \
  -e PYROSCOPE_ADDR=http://pyroscope:4040 \
  shipping-service
```

## Testing

### Manual gRPC testing

Install `grpcurl` — this service has **gRPC reflection enabled** so no `-proto` flag needed:

```bash
# get a shipping quote
grpcurl -plaintext \
  -d '{
    "address": {"street_address": "1 Main St", "city": "Brooklyn", "state": "NY", "country": "US", "zip_code": 11201},
    "items": [{"product_id": "OLJCESPC7Z", "quantity": 2}]
  }' \
  localhost:50051 \
  hipstershop.ShippingService/GetQuote

# ship an order
grpcurl -plaintext \
  -d '{
    "address": {"street_address": "1 Main St", "city": "Brooklyn", "state": "NY", "country": "US", "zip_code": 11201},
    "items": [{"product_id": "OLJCESPC7Z", "quantity": 1}]
  }' \
  localhost:50051 \
  hipstershop.ShippingService/ShipOrder

# health check
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
```

### Generate traffic

```bash
while true; do
  grpcurl -plaintext \
    -d '{"address": {"street_address": "1 Main St", "city": "Brooklyn", "state": "NY"}, "items": [{"product_id": "OLJCESPC7Z", "quantity": 2}]}' \
    localhost:50051 \
    hipstershop.ShippingService/GetQuote
  sleep 1
done
```

## Project Structure

```
├── cmd/server/
│   └── main.go                # Entrypoint — tracing, profiling, gRPC server bootstrap
├── internal/shipping/
│   ├── service.go             # gRPC handler implementations
│   ├── quote.go               # Shipping cost calculation logic
│   └── tracker.go             # Tracking ID generation
├── proto/
│   ├── shipping.proto         # Service and message definitions
│   ├── shipping.pb.go         # Generated protobuf code
│   └── shipping_grpc.pb.go    # Generated gRPC code
├── go.mod
├── go.sum
└── Dockerfile
```

## Observability

- **Traces** — OTLP gRPC → Alloy → Tempo via `otelgrpc.NewServerHandler()`. Enabled via `ENABLE_TRACING=1`.
- **Metrics** — Prometheus endpoint on `:9090/metrics`, scraped by Alloy → Mimir. Uses `go-grpc-prometheus` interceptors which expose `grpc_server_handled_total`, `grpc_server_handling_seconds`, and `grpc_server_started_total`.
- **Logs** — JSON structured logs via `logrus` to stdout, collected by Alloy via Docker socket → Loki.
- **Profiles** — CPU, alloc objects, alloc space, inuse objects, inuse space via Pyroscope push SDK. Also exposes `pprof` HTTP endpoints on `:9090/debug/pprof/`. Enabled via `ENABLE_PROFILING=1`.

## Part Of

This service is part of [platform-demo](https://github.com/mladenovskistefan111) — a full platform engineering project featuring microservices, observability (LGTM stack), GitOps (Argo CD), policy enforcement (Kyverno), infrastructure provisioning (Crossplane), and an internal developer portal (Backstage).