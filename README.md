# Dynamic Scoring Framework Sample Scorer

A Go Prometheus-backed scoring API for the Dynamic Scoring Framework (DSF).
It is based on the DSF `sample-scorer` and computes the average value for each
time series it receives.

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/config` | Returns the scorer and Prometheus source configuration |
| `POST` | `/scoring` | Returns one average score per input time series |
| `GET` | `/healthz` | Liveness check |

The scoring request uses the Prometheus matrix shape:

```json
{
  "data": [
    {
      "metric": {"node": "worker-1"},
      "values": [[1718600000, "5"], [1718600060, "10"]]
    }
  ]
}
```

The response preserves the metric labels and returns the average of the numeric
sample values:

```json
{
  "results": [
    {"metric": {"node": "worker-1"}, "score": 7.5}
  ]
}
```

## Run locally

```sh
make run
```

The Makefile uses the Go version declared in `go.mod`

Then check the configuration and health endpoint:

```sh
curl http://localhost:8000/config
curl http://localhost:8000/healthz
```

Submit the included sample payload:

```sh
curl -X POST http://localhost:8000/scoring \
  -H 'Content-Type: application/json' \
  --data-binary @examples/cpu-timeseries.json
```

Run the tests with:

```sh
make test
```

Build a local binary with:

```sh
make build
```

## Build and publish the image

The Makefile follows the framework's container-target conventions. Override
`IMAGE_REGISTRY`, `IMAGE_NAME`, `IMAGE_TAG`, or `CONTAINER_TOOL` as needed:

```sh
make image IMAGE_REGISTRY=quay.io/my-org IMAGE_TAG=dev
make image-push IMAGE_REGISTRY=quay.io/my-org IMAGE_TAG=dev
```

The default image is
`quay.io/open-cluster-management/dynamic-scoring-framework-sample-scorer:latest`.

## Configuration

`/config` provides the Prometheus query, range, step, scoring path, score name,
and interval. Set `PROMETHEUS_HOST` in the scorer's runtime environment if the
config endpoint should provide the source host. If it is unset, the source host
is omitted so it can be supplied by the `DynamicScorer` CR.

The service is intentionally a small starting point. Additional Prometheus-backed
examples can be added as separate config/scoring endpoint pairs; static scoring
is not included.
