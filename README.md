# Dynamic Scoring Sample Scorer

A Go scoring API for the Dynamic Scoring Framework (DSF), based on the original
DSF sample scorer. It provides two Prometheus-backed CPU-idle examples and a
static per-cluster example for QE.

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/average/config` | Configures the average CPU-idle example |
| `POST` | `/average/scoring` | Returns the average per input time series |
| `GET` | `/minimum/config` | Configures the minimum CPU-idle example |
| `POST` | `/minimum/scoring` | Returns the minimum per input time series |
| `GET` | `/static/config` | Configures the static QE example |
| `POST` | `/static/scoring` | Returns the configured score for the requesting spoke |
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

The response preserves metric labels. For the default CPU-idle query, the
average example returns the average idle percentage per series:

```json
{
  "results": [
    {"metric": {"node": "worker-1"}, "score": 7.5}
  ]
}
```

The minimum example uses the same request shape but returns the minimum CPU-idle
percentage in the query range for each series.

## Run locally

```sh
make run
```

The Makefile uses the Go version declared in `go.mod`.

Then check the configuration and health endpoint:

```sh
curl http://localhost:8000/average/config
curl http://localhost:8000/healthz
```

Submit the included sample payload:

```sh
curl -X POST http://localhost:8000/average/scoring \
  -H 'Content-Type: application/json' \
  --data-binary @examples/cpu-timeseries.json
```

Run the tests with:

```sh
make test
```

`make lint` installs the pinned `golangci-lint` version into `bin/` if needed.
Chart targets similarly install the pinned Helm version into `bin/`.

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
`quay.io/open-cluster-management/dynamic-scoring-sample-scorer:latest`.

## Configuration

`/average/config` provides the Prometheus query, range, step, scoring path, score name,
and interval for the average example. `/minimum/config` provides the
same source configuration with its own score name and endpoint path.

By default, the query returns one cluster-wide CPU-idle series. The average
example averages its samples; the minimum example returns the lowest sample in
the query range. Both examples report CPU idle percentage, not headroom. A
lower minimum-idle score indicates a busier cluster. If you customize
`PROMETHEUS_QUERY`, the meaning and direction of scores depend on that query.

The examples publish scores named `sample_cpu_idle_average` and
`sample_cpu_idle_minimum`. The static QE example publishes `sample_static`.

Set `PROMETHEUS_HOST` in the scorer's runtime environment if both config
endpoints should provide the source host. If it is unset, the host is omitted
so it can be supplied by the `DynamicScorer` CR. If you customize the query to
return multiple series, set a `scoreDimensionFormat` that gives each result a
distinct, cross-cluster-comparable dimension.

## Static QE example

The static scoring endpoints are available for QE tests but are not registered
by the Helm chart. Apply the sample `DynamicScorer` CR when needed:

```sh
oc apply -f examples/static-scorer/dynamicscorer.yaml
```

See [`examples/static-scorer/README.md`](examples/static-scorer/README.md) for
the ConfigMap format and usage. Static scores must be whole numbers from `-100`
to `100`, inclusive. The scorer uses the configured default (currently `0`)
when the ConfigMap or its JSON is unavailable, or when a spoke's entry is
missing or invalid. An invalid entry falls back only for that spoke; other
valid entries continue to be used.
