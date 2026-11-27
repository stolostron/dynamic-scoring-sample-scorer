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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const maxScoringRequestBytes = 10 << 20

// TimeSeries is a Prometheus matrix result. Values contain [timestamp, value]
// pairs; Prometheus encodes sample values as JSON strings.
type TimeSeries struct {
	Metric map[string]string   `json:"metric"`
	Values [][]json.RawMessage `json:"values"`
}

type ScoringPayload struct {
	ClusterName string       `json:"clusterName"`
	Data        []TimeSeries `json:"data"`
}

type Score struct {
	Metric map[string]string `json:"metric"`
	Score  float64           `json:"score"`
}

type ScoringResponse struct {
	Results []Score `json:"results"`
}

// NewHandler returns an HTTP handler implementing the DSF scorer endpoints.
func NewHandler(configs EndpointConfigs, staticScoresPath ...string) http.Handler {
	scoresPath := defaultStaticScoresPath
	if len(staticScoresPath) > 0 && staticScoresPath[0] != "" {
		scoresPath = staticScoresPath[0]
	}
	defaultScore := StaticScoreDefault()
	mux := http.NewServeMux()

	// Average example - this is our default sample
	mux.HandleFunc("/average/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, configs.Average)
	})
	mux.HandleFunc("/average/scoring", scoringHandler(averageFiniteValues))

	// Minimum CPU idle example
	mux.HandleFunc("/minimum/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, configs.Minimum)
	})
	mux.HandleFunc("/minimum/scoring", scoringHandler(minimumFiniteValue))

	// Static scores example - will return a static score per spoke cluster defined in a user-configured configmap
	mux.HandleFunc("/static/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, configs.Static)
	})
	mux.HandleFunc("/static/scoring", staticScoringHandler(scoresPath, defaultScore))

	// /healthz is a process-level liveness endpoint shared by all scorer examples.
	// It does not report per-scorer health or check the optional static scores file.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

// StaticScores maps managed-cluster names to their configured scores.
type StaticScores struct {
	Clusters map[string]json.RawMessage `json:"clusters"`
}

func staticScoringHandler(path string, defaultScore int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxScoringRequestBytes)
		var payload ScoringPayload
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, "invalid scoring request JSON", http.StatusBadRequest)
			return
		}
		if err := ensureEOF(decoder); err != nil {
			http.Error(w, "scoring request must contain one JSON value", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.ClusterName) == "" {
			http.Error(w, "clusterName is required", http.StatusBadRequest)
			return
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			writeStaticScore(w, defaultScore)
			return
		}
		scores, err := parseStaticScores(contents)
		if err != nil {
			writeStaticScore(w, defaultScore)
			return
		}
		score, ok := scores.scoreFor(payload.ClusterName)
		if !ok {
			writeStaticScore(w, defaultScore)
			return
		}
		writeJSON(w, http.StatusOK, ScoringResponse{Results: []Score{{
			Metric: map[string]string{},
			Score:  float64(score),
		}}})
	}
}

func parseStaticScores(contents []byte) (StaticScores, error) {
	var scores StaticScores
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scores); err != nil {
		return StaticScores{}, err
	}
	if err := ensureEOF(decoder); err != nil {
		return StaticScores{}, err
	}
	if scores.Clusters == nil {
		return StaticScores{}, errors.New(`"clusters" must be an object`)
	}
	return scores, nil
}

func (s StaticScores) scoreFor(cluster string) (int32, bool) {
	raw, ok := s.Clusters[cluster]
	if !ok {
		return 0, false
	}
	return parseStaticScore(raw)
}

func parseStaticScore(raw json.RawMessage) (int32, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '"' || trimmed[0] == '{' || trimmed[0] == '[' ||
		bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("true")) ||
		bytes.Equal(trimmed, []byte("false")) {
		return 0, false
	}
	parsed, err := strconv.ParseInt(string(trimmed), 10, 32)
	if err != nil {
		return 0, false
	}
	if parsed < -100 || parsed > 100 {
		return 0, false
	}
	return int32(parsed), true
}

func writeStaticScore(w http.ResponseWriter, score int32) {
	writeJSON(w, http.StatusOK, ScoringResponse{Results: []Score{{
		Metric: map[string]string{},
		Score:  float64(score),
	}}})
}

func scoringHandler(aggregate func([][]json.RawMessage) float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxScoringRequestBytes)
		var payload ScoringPayload
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, "invalid scoring request JSON", http.StatusBadRequest)
			return
		}
		if err := ensureEOF(decoder); err != nil {
			http.Error(w, "scoring request must contain one JSON value", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, scorePayload(payload, aggregate))
	}
}

func scorePayload(payload ScoringPayload, aggregate func([][]json.RawMessage) float64) ScoringResponse {
	results := make([]Score, 0, len(payload.Data))
	for _, series := range payload.Data {
		metric := series.Metric
		if metric == nil {
			metric = map[string]string{}
		}
		results = append(results, Score{
			Metric: metric,
			Score:  aggregate(series.Values),
		})
	}
	return ScoringResponse{Results: results}
}

func averageFiniteValues(samples [][]json.RawMessage) float64 {
	values := finiteValues(samples)
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, value := range values {
		sum += value
	}
	average := sum / float64(len(values))
	if math.IsNaN(average) || math.IsInf(average, 0) {
		return 0
	}
	return average
}

func minimumFiniteValue(samples [][]json.RawMessage) float64 {
	values := finiteValues(samples)
	if len(values) == 0 {
		return 0
	}
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func finiteValues(samples [][]json.RawMessage) []float64 {
	values := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if len(sample) < 2 {
			continue
		}
		value, err := parseNumericValue(sample[1])
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		values = append(values, value)
	}
	return values
}

func parseNumericValue(raw json.RawMessage) (float64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0, strconv.ErrSyntax
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return 0, err
		}
		return strconv.ParseFloat(value, 64)
	}
	return strconv.ParseFloat(string(trimmed), 64)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
