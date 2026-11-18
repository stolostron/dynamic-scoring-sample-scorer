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
	"strconv"
)

const maxScoringRequestBytes = 10 << 20

// TimeSeries is a Prometheus matrix result. Values contain [timestamp, value]
// pairs; Prometheus encodes sample values as JSON strings.
type TimeSeries struct {
	Metric map[string]string   `json:"metric"`
	Values [][]json.RawMessage `json:"values"`
}

type ScoringPayload struct {
	Data []TimeSeries `json:"data"`
}

type Score struct {
	Metric map[string]string `json:"metric"`
	Score  float64           `json:"score"`
}

type ScoringResponse struct {
	Results []Score `json:"results"`
}

// NewHandler returns an HTTP handler implementing the DSF scorer endpoints.
func NewHandler(configs EndpointConfigs) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, configs.Average)
	})
	mux.HandleFunc("/minimum-headroom/config", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, configs.MinimumHeadroom)
	})
	mux.HandleFunc("/scoring", scoringHandler(averageFiniteValues))
	mux.HandleFunc("/minimum-headroom/scoring", scoringHandler(minimumFiniteValue))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
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
