package scorer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			Name:   "sample-scorer",
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
			Name:   "sample-scorer-minimum-headroom",
			Source: source,
			Scoring: ScoringConfig{
				Path: "/minimum-headroom/scoring",
				Params: ScoringParams{
					Name:     "sample_cpu_minimum_headroom",
					Interval: 30,
				},
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
	response := request(t, NewHandler(testConfigs()), http.MethodGet, "/config", "")
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

func TestScoringAveragesEachSeriesAndPreservesLabels(t *testing.T) {
	body := `{
		"data": [
			{"metric":{"node":"worker-1"},"values":[[1718600000,"5"],[1718600060,"10"],[1718600120,"15"]]},
			{"metric":{"node":"worker-2"},"values":[[1718600000,"2"],[1718600060,"6"]]}
		]
	}`
	response := request(t, NewHandler(testConfigs()), http.MethodPost, "/scoring", body)
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
			response := request(t, handler, http.MethodPost, "/scoring", test.body)
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
	response := request(t, NewHandler(testConfigs()), http.MethodPost, "/scoring", `{"data":[`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestEndpointRejectsWrongMethod(t *testing.T) {
	response := request(t, NewHandler(testConfigs()), http.MethodPost, "/config", "")
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
