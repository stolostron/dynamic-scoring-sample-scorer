package scorer

import (
	"fmt"
	"os"
	"strconv"
)

const defaultPrometheusQuery = `avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100`

// Config describes the source query and scoring endpoint returned by /config.
type Config struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Source      SourceConfig  `json:"source"`
	Scoring     ScoringConfig `json:"scoring"`
}

// EndpointConfigs contains the Prometheus-backed examples served by the API.
type EndpointConfigs struct {
	Average         Config
	MinimumHeadroom Config
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

// ConfigsFromEnv builds the example configurations, allowing deployment-specific
// Prometheus details to be supplied without baking them into the image.
func ConfigsFromEnv() (EndpointConfigs, error) {
	prometheusRange, err := intFromEnv("PROMETHEUS_RANGE", 3600)
	if err != nil {
		return EndpointConfigs{}, err
	}
	prometheusStep, err := intFromEnv("PROMETHEUS_STEP", 60)
	if err != nil {
		return EndpointConfigs{}, err
	}
	scoringInterval, err := intFromEnv("SCORING_INTERVAL", 30)
	if err != nil {
		return EndpointConfigs{}, err
	}
	if prometheusRange <= 0 {
		return EndpointConfigs{}, fmt.Errorf("PROMETHEUS_RANGE must be greater than zero")
	}
	if prometheusStep <= 0 {
		return EndpointConfigs{}, fmt.Errorf("PROMETHEUS_STEP must be greater than zero")
	}
	if scoringInterval <= 0 {
		return EndpointConfigs{}, fmt.Errorf("SCORING_INTERVAL must be greater than zero")
	}

	source := SourceConfig{
		Type: "Prometheus",
		Host: os.Getenv("PROMETHEUS_HOST"),
		Path: stringFromEnv("PROMETHEUS_PATH", "/api/v1/query_range"),
		Params: PrometheusParams{
			Query: stringFromEnv("PROMETHEUS_QUERY", defaultPrometheusQuery),
			Range: prometheusRange,
			Step:  prometheusStep,
		},
	}
	return EndpointConfigs{
		Average: Config{
			Name:        "sample-scorer",
			Description: "Average CPU headroom across the cluster",
			Source:      source,
			Scoring: ScoringConfig{
				Path: "/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_average",
					Interval: scoringInterval,
				},
			},
		},
		MinimumHeadroom: Config{
			Name:        "sample-scorer-minimum-headroom",
			Description: "Minimum CPU headroom observed across the cluster",
			Source:      source,
			Scoring: ScoringConfig{
				Path: "/minimum-headroom/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_minimum_headroom",
					Interval: scoringInterval,
				},
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
