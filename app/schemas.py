"""Request and response models for the DSF scoring API."""

from pydantic import BaseModel, Field


class TimeSeries(BaseModel):
    metric: dict[str, str] = Field(default_factory=dict)
    # Prometheus matrix samples are [timestamp, "value"] pairs.
    values: list[tuple[float, str]] = Field(default_factory=list)


class ScoringPayload(BaseModel):
    # `data` can be null when the agent has no source results.
    data: list[TimeSeries] | None = None


class Score(BaseModel):
    metric: dict[str, str]
    score: float


class ScoringResponse(BaseModel):
    results: list[Score]


class PrometheusParams(BaseModel):
    query: str
    range: int
    step: int


class SourceConfig(BaseModel):
    type: str
    host: str | None = None
    path: str
    params: PrometheusParams


class ScoringParams(BaseModel):
    name: str
    interval: int


class ScoringConfig(BaseModel):
    path: str
    params: ScoringParams


class ConfigResponse(BaseModel):
    name: str
    description: str
    source: SourceConfig
    scoring: ScoringConfig
