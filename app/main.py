"""Minimal Prometheus-backed scoring API for the Dynamic Scoring Framework."""

import logging
import math
import os

from fastapi import FastAPI

from app.schemas import (
    ConfigResponse,
    PrometheusParams,
    Score,
    ScoringConfig,
    ScoringParams,
    ScoringPayload,
    ScoringResponse,
    SourceConfig,
)

logger = logging.getLogger(__name__)

PROMETHEUS_HOST = os.getenv("PROMETHEUS_HOST") or None
PROMETHEUS_PATH = os.getenv("PROMETHEUS_PATH", "/api/v1/query_range")
PROMETHEUS_QUERY = os.getenv(
    "PROMETHEUS_QUERY",
    'sum by (node, namespace, pod) '
    '(rate(container_cpu_usage_seconds_total{container!="", pod!=""}[1m]))',
)
PROMETHEUS_RANGE = int(os.getenv("PROMETHEUS_RANGE", "3600"))
PROMETHEUS_STEP = int(os.getenv("PROMETHEUS_STEP", "60"))
SCORER_NAME = os.getenv("SCORER_NAME", "sample-scorer")
SCORER_DESCRIPTION = os.getenv(
    "SCORER_DESCRIPTION", "A sample average score for time series data"
)
SCORE_NAME = os.getenv("SCORE_NAME", "sample_cpu_average")
SCORING_INTERVAL = int(os.getenv("SCORING_INTERVAL", "30"))
SCORING_PATH = os.getenv("SCORING_PATH", "/scoring")

app = FastAPI(title="DSF Sample Scorer")


def average_finite_values(series_values: list[tuple[float, str]]) -> float:
    """Return the arithmetic mean of finite Prometheus sample values."""
    values: list[float] = []
    for _, raw_value in series_values:
        try:
            value = float(raw_value)
        except (TypeError, ValueError):
            logger.warning("Skipping non-numeric Prometheus value %r", raw_value)
            continue
        if math.isfinite(value):
            values.append(value)

    if not values:
        return 0.0
    return sum(values) / len(values)


@app.post("/scoring", response_model=ScoringResponse)
async def score_timeseries(payload: ScoringPayload) -> ScoringResponse:
    """Return one average score for each input time series."""
    results = [
        Score(
            metric=series.metric.copy(),
            score=average_finite_values(series.values),
        )
        for series in (payload.data or [])
    ]
    return ScoringResponse(results=results)


@app.get("/healthz")
async def healthcheck() -> dict[str, str]:
    return {"status": "ok"}


@app.get(
    "/config",
    response_model=ConfigResponse,
    response_model_exclude_none=True,
)
async def get_config() -> ConfigResponse:
    """Describe the source query and scoring endpoint to the DSF controller."""
    return ConfigResponse(
        name=SCORER_NAME,
        description=SCORER_DESCRIPTION,
        source=SourceConfig(
            type="Prometheus",
            host=PROMETHEUS_HOST,
            path=PROMETHEUS_PATH,
            params=PrometheusParams(
                query=PROMETHEUS_QUERY,
                range=PROMETHEUS_RANGE,
                step=PROMETHEUS_STEP,
            ),
        ),
        scoring=ScoringConfig(
            path=SCORING_PATH,
            params=ScoringParams(
                name=SCORE_NAME,
                interval=SCORING_INTERVAL,
            ),
        ),
    )
