package scorer

import (
	"fmt"
	"os"
	"strconv"
)

const defaultPrometheusQuery = `sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[1m]))`

// Config describes the source query and scoring endpoint returned by /config.
type Config struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Source      SourceConfig  `json:"source"`
	Scoring     ScoringConfig `json:"scoring"`
}

type SourceConfig struct {
	Type   string           `json:"type"`
	Host   string           `json:"host,omitempty"`
	Path   string           `json:"path"`
	Params PrometheusParams `json:"params"`
}

type PrometheusParams struct {
	Query string `json:"query"`
	Range int    `json:"range"`
	Step  int    `json:"step"`
}

type ScoringConfig struct {
	Path   string        `json:"path"`
	Params ScoringParams `json:"params"`
}

type ScoringParams struct {
	Name     string `json:"name"`
	Interval int    `json:"interval"`
}

// ConfigFromEnv builds the scorer configuration, allowing deployment-specific
// Prometheus details to be supplied without baking them into the image.
func ConfigFromEnv() (Config, error) {
	prometheusRange, err := intFromEnv("PROMETHEUS_RANGE", 3600)
	if err != nil {
		return Config{}, err
	}
	prometheusStep, err := intFromEnv("PROMETHEUS_STEP", 60)
	if err != nil {
		return Config{}, err
	}
	scoringInterval, err := intFromEnv("SCORING_INTERVAL", 30)
	if err != nil {
		return Config{}, err
	}
	if prometheusRange <= 0 {
		return Config{}, fmt.Errorf("PROMETHEUS_RANGE must be greater than zero")
	}
	if prometheusStep <= 0 {
		return Config{}, fmt.Errorf("PROMETHEUS_STEP must be greater than zero")
	}
	if scoringInterval <= 0 {
		return Config{}, fmt.Errorf("SCORING_INTERVAL must be greater than zero")
	}

	// If unset here, the prometheus host will need to be set in the dynamicscorer CR
	host := os.Getenv("PROMETHEUS_HOST")

	return Config{
		Name:        stringFromEnv("SCORER_NAME", "sample-scorer"),
		Description: stringFromEnv("SCORER_DESCRIPTION", "A sample average score for time series data"),
		Source: SourceConfig{
			Type: "Prometheus",
			Host: host,
			Path: stringFromEnv("PROMETHEUS_PATH", "/api/v1/query_range"),
			Params: PrometheusParams{
				Query: stringFromEnv("PROMETHEUS_QUERY", defaultPrometheusQuery),
				Range: prometheusRange,
				Step:  prometheusStep,
			},
		},
		Scoring: ScoringConfig{
			Path: stringFromEnv("SCORING_PATH", "/scoring"),
			Params: ScoringParams{
				Name:     stringFromEnv("SCORE_NAME", "sample_cpu_average"),
				Interval: scoringInterval,
			},
		},
	}, nil
}

func stringFromEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func intFromEnv(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}
