# Orbit

A Go-based truck simulation with HTTP and WebSocket APIs, Prometheus metrics, and optional admin profiling hooks.

## Running the server

```
go run ./backend/cmd/orbitserver \
  -addr :8080 \
  -trucks 2000 \
  -update-interval 1s \
  -enable-admin=false
```

* Metrics are exposed at `http://localhost:8080/metrics` in Prometheus format.
* Admin profiling endpoints (pprof) are served under `/admin/debug/pprof` when `-enable-admin` is set.
* Correlation IDs are read from `X-Correlation-ID` or generated per request and echoed back in responses.

## Load testing

Use the provided helper to stress the truck API with 2,000+ simulated vehicles:

```
./scripts/loadtest.sh
```

Environment variables:

* `RATE` – requests per second to sustain (default `2500`).
* `DURATION` – how long to run the attack (default `60s`).
* `TARGET` – API endpoint to hit (default `http://localhost:8080/api/trucks`).

The script installs [`vegeta`](https://github.com/tsenart/vegeta) automatically if missing.

## Development

* Lint: `go vet ./...`
* Format: `gofmt -w .`
* Tests: `go test ./...`

Continuous Integration runs format, lint, and test checks via GitHub Actions.
