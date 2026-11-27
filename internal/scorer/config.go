/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scorer

import (
	"fmt"
	"os"
	"strconv"
)

const staticScoresPathEnv = "STATIC_SCORES_PATH"

const (
	defaultPrometheusQuery  = `avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100`
	defaultStaticScoresPath = "/etc/static-scorer/scores.json"
	defaultStaticScore      = 0
)

// Config describes the source query and scoring endpoint returned by a scorer config endpoint.
type Config struct {
	Name               string        `json:"name"`
	Description        string        `json:"description"`
	IncludeClusterName *bool         `json:"includeClusterName,omitempty"`
	Source             SourceConfig  `json:"source"`
	Scoring            ScoringConfig `json:"scoring"`
}

// EndpointConfigs contains the Prometheus-backed examples served by the API.
type EndpointConfigs struct {
	Average Config
	Minimum Config
	Static  Config
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
			Name:        "dynamic-scoring-sample-scorer",
			Description: "Average CPU idle percentage across the cluster",
			Source:      source,
			Scoring: ScoringConfig{
				Path: "/average/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_idle_average",
					Interval: scoringInterval,
				},
			},
		},
		Minimum: Config{
			Name:        "dynamic-scoring-sample-scorer-minimum",
			Description: "Minimum CPU idle percentage observed across the cluster",
			Source:      source,
			Scoring: ScoringConfig{
				Path: "/minimum/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_idle_minimum",
					Interval: scoringInterval,
				},
			},
		},
		Static: Config{
			Name:               "dynamic-scoring-sample-scorer-static",
			Description:        "Static test scores configured per managed cluster",
			IncludeClusterName: new(true),
			Source: SourceConfig{
				Type: "None",
			},
			Scoring: ScoringConfig{
				Path: "/static/scoring",
				Params: ScoringParams{
					Name:     "sample_static",
					Interval: scoringInterval,
				},
			},
		},
	}, nil
}

// StaticScoresPath returns the configured static-score file path.
func StaticScoresPath() string {
	return stringFromEnv(staticScoresPathEnv, defaultStaticScoresPath)
}

// StaticScoreDefault returns the valid default score used when static score data
// is missing or invalid.
func StaticScoreDefault() int32 {
	value, err := strconv.ParseInt(os.Getenv("STATIC_SCORER_DEFAULT_VALUE"), 10, 32)
	if err != nil || value < -100 || value > 100 {
		return defaultStaticScore
	}
	return int32(value)
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
