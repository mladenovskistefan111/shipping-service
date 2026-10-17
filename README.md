# shipping-service

A gRPC service that provides shipping quotes and "ships" orders for the platform-demo e-commerce platform. It is a simulation: quotes are a flat rate and `ShipOrder` just fabricates a tracking ID. It is stateless, has no downstream dependencies, and is called by `checkout-service` (and by `frontend` for the cart-page shipping estimate).

Part of the [platform-demo](https://github.com/mladenovskistefan111) platform — microservices, observability (LGTM stack), GitOps (Argo CD), policy enforcement (Kyverno), infrastructure provisioning (Crossplane) and an internal developer portal (Backstage). See the master `README.md` in the platform-demo root for the whole-system view.

## At a glance

| | |
|---|---|
| **Language / runtime** | Go 1.25 (static binary, `CGO_ENABLED=0`) |
| **Image** | `mladenovskistefan/shipping-service` |
| **Protocol / port** | gRPC (HTTP/2, plaintext) on `50051` |
| **Metrics** | Pushed over OTLP/gRPC (Prometheus registry bridged into OpenTelemetry); `/metrics` on `9090` is still served for debugging |
| **Health** | Standard `grpc.health.v1.Health/Check` on `50051` — static `SERVING` |
| **State** | None |
| **Depends on** | Nothing (optional: OTLP collector, Pyroscope) |
| **Called by** | `checkout-service` (`GetQuote`, `ShipOrder`), `frontend` (`GetQuote`) |
| **gRPC reflection** | Enabled — `grpcurl` works without `-proto` |

## API

Package `hipstershop`, service `ShippingService` ([`proto/shipping.proto`](proto/shipping.proto)).

| Method | Request | Response | Behaviour |
|---|---|---|---|
| `GetQuote` | `{ Address address, repeated CartItem items }` | `{ Money cost_usd }` | Sums item quantities. **0 items → $0.00; any items → flat $8.99 USD** (address is ignored) |
| `ShipOrder` | `{ Address address, repeated CartItem items }` | `{ string tracking_id }` | Builds a tracking ID like `XK-21345-109876543` from random letters/digits and the length of `"<street>, <city>, <state>"`. Nothing is dispatched or stored. The request **must include an address** — a nil address dereferences a nil pointer, and gRPC-Go does not recover handler panics, so it would crash the process |

`Money` = `{ currency_code, int64 units, int32 nanos }`. `frontend` calls `GetQuote` with a nil address and the cart items for the cart-page estimate; checkout passes the real address.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `50051` | gRPC listen port |
| `METRICS_PORT` | `9090` | Prometheus + pprof HTTP port |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset (telemetry off) | OTLP/**gRPC** endpoint for **traces, metrics and logs** (scheme required), e.g. `http://alloy.monitoring.svc.cluster.local:4317`. Legacy fallback: `COLLECTOR_SERVICE_ADDR` (`host:port`). Telemetry is on iff an endpoint is set |
| `ENABLE_TRACING` | — | Legacy switch: `0` forces all telemetry off even if an endpoint is set (it is no longer needed to turn telemetry on) |
| `OTEL_METRIC_EXPORT_INTERVAL` | `15000` | Milliseconds between metric pushes (Go also accepts durations such as `15s`) |
| `OTEL_SERVICE_NAME` | `shipping-service` | `service.name` — becomes the Prometheus `job` and the Loki `service_name`; keep the default, dashboards depend on it |
| `PYROSCOPE_ADDR` | `http://pyroscope:4040` | Profiles use the Pyroscope SDK push protocol (**not** OTLP). Point it at Alloy (`http://alloy.monitoring.svc.cluster.local:4040`), which forwards to Pyroscope, or directly at Pyroscope |
| `ENABLE_PROFILING` | off | `1` starts the Pyroscope SDK. (pprof HTTP is **not** gated by this — it's always on) |

Notes:

- The Pyroscope application name is hard-coded to `shipping-service`.
- Unlike `checkout-service` and `product-catalog-service`, pprof endpoints are registered unconditionally. Do not expose `:9090` outside the cluster; keep it ClusterIP-only / NetworkPolicy-restricted.
- Telemetry is **pushed**, not scraped: traces, metrics and logs share one OTLP/gRPC endpoint (Alloy); profiles go to Pyroscope (via Alloy). `service.instance.id` is the pod hostname, so replicas stay distinct in Mimir.
- A Prometheus `/metrics` endpoint is still served on `METRICS_PORT` for debugging, but nothing has to scrape it — the same registry is bridged into OTLP, so metric names are unchanged.
- Logs: the `logrus` logger keeps writing JSON to stdout and additionally mirrors every record over OTLP (with trace ids when a context is attached).

## Container image

Multi-stage [`Dockerfile`](Dockerfile):

| Stage | Base | What happens |
|---|---|---|
| builder | `golang:1.25.9-alpine` | `go mod download`, then `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" ./cmd/server` |
| runtime | `alpine:3.21` | single binary at `/app/shipping-service` |

- **Entrypoint:** `/app/shipping-service`. `GOTRACEBACK=single`.
- **User: root** — no `USER` in the Dockerfile; the image runs as UID 0 (verified). Set a non-root `runAsUser` (e.g. `10001`) in Kubernetes; the binary needs no privileges.
- **Architecture: `amd64` only** (`GOARCH=amd64` hard-coded) — `exec format error` on `arm64` nodes.
- **EXPOSE:** `50051`, `9090`. **No `HEALTHCHECK`.**
- **Image size:** ~10 MB. No `.dockerignore`, so `.git` is sent in the build context.

## Kubernetes deployment notes

**Workload:** stateless `Deployment`; scale freely (callers pin connections per pod — see "gRPC load balancing" in the master README).

**Ports to declare**

| Name | Port | Protocol | Expose via Service? |
|---|---|---|---|
| `grpc` | `50051` | TCP (HTTP/2) | Yes — callers dial `shipping-service:50051` |
| `metrics` | `9090` | TCP (HTTP/1.1) | Optional (debugging/pprof). Not scraped by default — metrics are pushed over OTLP |

**Service DNS name expected by callers:** `shipping-service` (`frontend` and `checkout-service`: `SHIPPING_SERVICE_ADDR=shipping-service:50051`). `payment-service` also uses `50051` — different Service, no clash.

**Probes**

```yaml
readinessProbe:
  grpc: { port: 50051 }
  periodSeconds: 5
livenessProbe:
  grpc: { port: 50051 }
  periodSeconds: 10
  failureThreshold: 3
```

Startup is near-instant; no `startupProbe` needed.

**Security context**

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 10001        # image runs as root by default; any non-zero UID works
  runAsGroup: 10001
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities: { drop: ["ALL"] }
```

**Resources** — requests sized from measured idle usage in the test stack (see the chart's `values.yaml` for limits):

```yaml
resources:
  requests: { cpu: 50m, memory: 32Mi }
  limits:   { cpu: "500m", memory: 128Mi }
```

**Config / secrets:** none.

**Telemetry:** pushed, not scraped — OTLP/gRPC to Alloy (`global.monitoring.otlpEndpoint`), profiles to Alloy `:4040`. Metrics carry `job="shipping-service"` and `instance=<pod>`; no scrape annotations or `ServiceMonitor` are needed.

**Helm chart:** [`platform-demo-helm/shipping-service`](https://github.com/mladenovskistefan111/platform-demo-helm/tree/main/shipping-service) (built on the shared `platform-common` library chart). It ships with HA defaults — 2 replicas,
HPA, PodDisruptionBudget, topology spread, NetworkPolicy, restricted pod security context, probes and the full env contract (see Configuration) —
and was verified on a 3-node kind cluster under the `restricted` Pod Security Standard.

**Deployment and admission policies:** Argo CD deploys the chart from `platform-demo-helm` (`envs/<env>/values/shipping-service.yaml` pins the image digest; promotion is a pull request). In the cluster Kyverno enforces, for this service: pods named `app.kubernetes.io/name: shipping-service` that run under the ServiceAccount of the same name (Cilium derives the network identity from that label), an image from this repository pinned by digest and signed by our release pipeline, a read-only root filesystem, resource requests and a memory limit, readiness and liveness probes and no API token. The Helm CI evaluates the rendered chart against these policies before merge; see [`platform-demo-helm/policies`](https://github.com/mladenovskistefan111/platform-demo-helm/tree/main/policies).

## Observability

| Signal | Implementation | Destination |
|---|---|---|
| **Traces** | OpenTelemetry (`otelgrpc`/`otelhttp` handlers), OTLP/gRPC exporter, `AlwaysSample`, W3C TraceContext + Baggage; the trace context is propagated to every downstream call | Alloy `:4317` → Tempo |
| **Metrics** | The Prometheus default registry (`grpc_server_handled_total`, `grpc_server_handling_seconds_*`, `grpc_server_started_total`, `grpc_server_msg_*`, Go runtime/process) is bridged into OpenTelemetry and pushed over OTLP/gRPC every 15 s — metric names are unchanged. `/metrics` is still served for debugging | Alloy → Mimir |
| **Logs** | `logrus` JSON to stdout **and** an OTLP/gRPC hook (`otellogrus`) | Alloy → Loki |
| **Profiles** | Pyroscope Go SDK (CPU, alloc/inuse objects & space) plus pprof HTTP (always on) | Alloy `:4040` → Pyroscope |

Every series carries `job="shipping-service"` (from `service.name`) and `instance=<pod name>` (`service.instance.id`), which is what the Grafana dashboards select on. Logs land in Loki with `service_name="shipping-service"` and trace/span ids, so a log line links to its trace.

## Running locally

Requirements: Go 1.25+, Docker, [`grpcurl`](https://github.com/fullstorydev/grpcurl).

```bash
go build ./... && go run ./cmd/server

docker build -t shipping-service .
docker run --rm -p 50051:50051 -p 9090:9090 shipping-service

go test ./...
```

### Manual gRPC testing (reflection is on)

```bash
grpcurl -plaintext -d '{
  "address": {"street_address": "1 Main St", "city": "Brooklyn", "state": "NY", "country": "US", "zip_code": 11201},
  "items": [{"product_id": "OLJCESPC7Z", "quantity": 2}]
}' localhost:50051 hipstershop.ShippingService/GetQuote

grpcurl -plaintext -d '{
  "address": {"street_address": "1 Main St", "city": "Brooklyn", "state": "NY", "country": "US", "zip_code": 11201},
  "items": [{"product_id": "OLJCESPC7Z", "quantity": 1}]
}' localhost:50051 hipstershop.ShippingService/ShipOrder

grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
curl -s localhost:9090/metrics | grep grpc_server
curl -s localhost:9090/debug/pprof/ | head
```

## Project structure

```
├── cmd/server/main.go            # entrypoint: tracing, profiling, gRPC server, metrics + pprof server
├── internal/shipping/
│   ├── service.go                # GetQuote / ShipOrder handlers
│   ├── quote.go                  # flat-rate quote logic ($8.99)
│   └── tracker.go                # random tracking-ID generation
├── proto/                        # shipping.proto + generated Go
├── Dockerfile
├── go.mod / go.sum
├── .trivyignore
└── docs/                         # empty placeholder
```

## Known limitations

- Simulation only: flat $8.99, no address validation, nothing is actually shipped or stored.
- `ShipOrder` dereferences `req.Address` without a nil check; a request with no address crashes the process (no recovery interceptor).
- pprof is always exposed on the metrics port.
- `amd64`-only image that runs as root unless the pod overrides it.
