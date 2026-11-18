# Dynamic Scoring Framework Sample Scorer

A Go Prometheus-backed scoring API for the Dynamic Scoring Framework (DSF),
based on the DSF `sample-scorer`. It exposes two scoring examples: average CPU
headroom and minimum CPU headroom over the query range.

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/config` | Configures the average CPU-headroom example |
| `POST` | `/scoring` | Returns the average per input time series |
| `GET` | `/minimum-headroom/config` | Configures the minimum CPU-headroom example |
| `POST` | `/minimum-headroom/scoring` | Returns the minimum per input time series |
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

The response preserves the metric labels. The average example returns the
average of numeric sample values:

```json
{
  "results": [
    {"metric": {"node": "worker-1"}, "score": 7.5}
  ]
}
```

The minimum-headroom example uses the same request shape but returns the
minimum numeric sample value for each series.

## Run locally

```sh
make run
```

The Makefile uses the Go version declared in `go.mod`.

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

Run lint with `make lint`; it installs the pinned `golangci-lint` version into
`bin/` if it is not already present.

Run all verification checks, including Helm chart lint, with:

```sh
make verify
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
and interval for the average example. `/minimum-headroom/config` provides the
same source configuration with its own score name and endpoint path. By default,
the Prometheus query returns one cluster-wide average CPU-idle series, which
means higher scores represent more CPU headroom and each example produces one
dimension per cluster.

Set `PROMETHEUS_HOST` in the scorer's runtime environment if both config
endpoints should provide the source host. If it is unset, the host is omitted
so it can be supplied by the `DynamicScorer` CR. If you customize the query to
return multiple series, set a `scoreDimensionFormat` that gives each result a
distinct, cross-cluster-comparable dimension.

Static scoring is not included.
