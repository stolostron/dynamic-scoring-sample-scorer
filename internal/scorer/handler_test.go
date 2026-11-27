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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testConfigs() EndpointConfigs {
	source := SourceConfig{
		Type: "Prometheus",
		Path: "/api/v1/query_range",
		Params: PrometheusParams{
			Query: "up",
			Range: 3600,
			Step:  60,
		},
	}
	return EndpointConfigs{
		Average: Config{
			Name:   "dynamic-scoring-sample-scorer",
			Source: source,
			Scoring: ScoringConfig{
				Path: "/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_average",
					Interval: 30,
				},
			},
		},
		MinimumHeadroom: Config{
			Name:   "dynamic-scoring-sample-scorer-minimum-headroom",
			Source: source,
			Scoring: ScoringConfig{
				Path: "/minimum-headroom/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_minimum_headroom",
					Interval: 30,
				},
			},
		},
		Static: Config{
			Name:               "dynamic-scoring-sample-scorer-static",
			IncludeClusterName: new(true),
			Source:             SourceConfig{Type: "None"},
			Scoring: ScoringConfig{
				Path:   "/static/scoring",
				Params: ScoringParams{Name: "sample_static", Interval: 30},
			},
		},
	}
}

func TestHealthcheck(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodGet, "/healthz", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q, want health response", got)
	}
}

func TestConfigOmitsEmptyPrometheusHost(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodGet, "/average/config", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if strings.Contains(response.Body.String(), `"host"`) {
		t.Fatalf("config unexpectedly included empty source host: %s", response.Body.String())
	}

	var config Config
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if config.Scoring.Params.Name != "sample_cpu_average" {
		t.Fatalf("score name = %q, want sample_cpu_average", config.Scoring.Params.Name)
	}
}

func TestMinimumHeadroomConfigDescribesItsEndpoint(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodGet, "/minimum-headroom/config", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var config Config
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatalf("decode minimum-headroom config response: %v", err)
	}
	if config.Scoring.Path != "/minimum-headroom/scoring" {
		t.Fatalf("scoring path = %q, want /minimum-headroom/scoring", config.Scoring.Path)
	}
	if config.Scoring.Params.Name != "sample_cpu_minimum_headroom" {
		t.Fatalf("score name = %q, want sample_cpu_minimum_headroom", config.Scoring.Params.Name)
	}
}

func TestStaticConfigOptsIntoClusterNameAndUsesNoSource(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodGet, "/static/config", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var config Config
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatalf("decode static config: %v", err)
	}
	if config.IncludeClusterName == nil || !*config.IncludeClusterName {
		t.Fatalf("includeClusterName = %v, want true", config.IncludeClusterName)
	}
	if config.Source.Type != "None" || config.Scoring.Path != "/static/scoring" {
		t.Fatalf("unexpected static config: %#v", config)
	}
}

func TestStaticScoringUsesLatestMountedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scores.json")
	handler := NewHandler(testConfigs(), path)
	writeScores := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatalf("write scores file: %v", err)
		}
	}

	writeScores(`{"clusters":{"spoke-a":90,"spoke-b":40}}`)
	response := request(
		t, handler, http.MethodPost, "/static/scoring",
		`{"clusterName":"spoke-a","data":null}`,
	)
	assertStaticScore(t, response, 90)

	writeScores(`{"clusters":{"spoke-a":10,"spoke-b":95}}`)
	response = request(
		t, handler, http.MethodPost, "/static/scoring",
		`{"clusterName":"spoke-a","data":null}`,
	)
	assertStaticScore(t, response, 10)
}

func TestStaticScoringUsesDefaultForMissingOrInvalidConfig(t *testing.T) {
	t.Setenv("STATIC_SCORER_DEFAULT_VALUE", "-7")
	path := filepath.Join(t.TempDir(), "scores.json")
	handler := NewHandler(testConfigs(), path)
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "missing file"},
		{name: "malformed JSON", data: "not-json"},
		{name: "null score", data: `{"clusters":{"spoke-a":null}}`},
		{name: "string score", data: `{"clusters":{"spoke-a":"5"}}`},
		{name: "fractional score", data: `{"clusters":{"spoke-a":5.5}}`},
		{name: "score below range", data: `{"clusters":{"spoke-a":-101}}`},
		{name: "score above range", data: `{"clusters":{"spoke-a":101}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.data != "" {
				if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
					t.Fatalf("write scores file: %v", err)
				}
			}
			response := request(t, handler, http.MethodPost, "/static/scoring", `{"clusterName":"spoke-a"}`)
			assertStaticScore(t, response, -7)
		})
	}
}

func TestStaticScoringUsesDefaultForUnlistedCluster(t *testing.T) {
	t.Setenv("STATIC_SCORER_DEFAULT_VALUE", "3")
	path := filepath.Join(t.TempDir(), "scores.json")
	if err := os.WriteFile(path, []byte(`{"clusters":{"spoke-a":90}}`), 0600); err != nil {
		t.Fatalf("write scores file: %v", err)
	}
	response := request(t, NewHandler(testConfigs(), path), http.MethodPost, "/static/scoring", `{"clusterName":"unknown"}`)
	assertStaticScore(t, response, 3)
}

func TestStaticScoresRejectInvalidDocumentShape(t *testing.T) {
	for _, input := range []string{
		`{}`,
		`{"clusters":null}`,
		`{"clusters":{"spoke-a":5},"unexpected":true}`,
		`{"clusters":{"spoke-a":5}} {}`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseStaticScores([]byte(input)); err == nil {
				t.Fatalf("parseStaticScores(%s) succeeded; want validation error", input)
			}
		})
	}
}

func TestStaticScoringDefaultsOnlyForSpokeWithInvalidValue(t *testing.T) {
	t.Setenv("STATIC_SCORER_DEFAULT_VALUE", "3")
	path := filepath.Join(t.TempDir(), "scores.json")
	if err := os.WriteFile(path, []byte(`{"clusters":{"spoke-a":90,"spoke-b":null,"spoke-c":5.5,"spoke-d":101}}`), 0600); err != nil {
		t.Fatalf("write scores file: %v", err)
	}
	handler := NewHandler(testConfigs(), path)

	for cluster, want := range map[string]float64{
		"spoke-a": 90,
		"spoke-b": 3,
		"spoke-c": 3,
		"spoke-d": 3,
		"unknown": 3,
	} {
		t.Run(cluster, func(t *testing.T) {
			response := request(
				t, handler, http.MethodPost, "/static/scoring",
				`{"clusterName":"`+cluster+`"}`,
			)
			assertStaticScore(t, response, want)
		})
	}
}

func TestStaticScoreDefaultFallsBackWhenEnvironmentValueIsInvalid(t *testing.T) {
	for _, value := range []string{"bad", "1.5", "-101", "101", "2147483648"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("STATIC_SCORER_DEFAULT_VALUE", value)
			if got := StaticScoreDefault(); got != defaultStaticScore {
				t.Fatalf("default score = %d, want fallback %d", got, defaultStaticScore)
			}
		})
	}
}

func assertStaticScore(t *testing.T, response *httptest.ResponseRecorder, want float64) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result ScoringResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode static response: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Score != want {
		t.Fatalf("results = %#v, want one score %v", result.Results, want)
	}
}

func TestScoringAveragesEachSeriesAndPreservesLabels(t *testing.T) {
	body := `{
		"data": [
			{"metric":{"node":"worker-1"},"values":[[1718600000,"5"],[1718600060,"10"],[1718600120,"15"]]},
			{"metric":{"node":"worker-2"},"values":[[1718600000,"2"],[1718600060,"6"]]}
		]
	}`
	response := request(t, NewHandler(testConfigs()), http.MethodPost, "/average/scoring", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	var result ScoringResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode scoring response: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(result.Results))
	}
	if result.Results[0].Score != 10 {
		t.Errorf("worker-1 score = %v, want 10", result.Results[0].Score)
	}
	if result.Results[0].Metric["node"] != "worker-1" {
		t.Errorf("worker-1 metric labels = %#v", result.Results[0].Metric)
	}
	if result.Results[1].Score != 4 {
		t.Errorf("worker-2 score = %v, want 4", result.Results[1].Score)
	}
}

func TestMinimumHeadroomReturnsMinimumPerSeriesAndPreservesLabels(t *testing.T) {
	response := request(
		t,
		NewHandler(testConfigs()),
		http.MethodPost,
		"/minimum-headroom/scoring",
		`{"data":[{"metric":{"node":"worker-1"},"values":[[1,"75"],[2,"40"],[3,"60"]]}]}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	var result ScoringResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode minimum-headroom response: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(result.Results))
	}
	if result.Results[0].Score != 40 {
		t.Errorf("minimum score = %v, want 40", result.Results[0].Score)
	}
	if result.Results[0].Metric["node"] != "worker-1" {
		t.Errorf("metric labels = %#v, want worker-1 node", result.Results[0].Metric)
	}
}

func TestScoringHandlesNullDataAndInvalidSamples(t *testing.T) {
	handler := NewHandler(testConfigs())
	tests := []struct {
		name        string
		body        string
		wantResults int
		wantScore   float64
	}{
		{name: "null data", body: `{"data":null}`, wantResults: 0},
		{
			name:        "invalid samples",
			body:        `{"data":[{"metric":{"node":"worker-1"},"values":[[1,"NaN"],[2,"not-a-number"]]}]}`,
			wantResults: 1,
			wantScore:   0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := request(t, handler, http.MethodPost, "/average/scoring", test.body)
			if response.Code != http.StatusOK {
				t.Fatalf("body %s: status = %d, want %d", test.body, response.Code, http.StatusOK)
			}
			var result ScoringResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode scoring response: %v", err)
			}
			if len(result.Results) != test.wantResults {
				t.Fatalf("got %d results, want %d", len(result.Results), test.wantResults)
			}
			if test.wantResults > 0 && result.Results[0].Score != test.wantScore {
				t.Errorf("invalid sample score = %v, want 0", result.Results[0].Score)
			}
		})
	}
}

func TestScoringRejectsMalformedJSON(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodPost, "/average/scoring", `{"data":[`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestEndpointRejectsWrongMethod(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodPost, "/average/config", "")
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	handler.ServeHTTP(recorder, request)
	return recorder
}
