from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_healthcheck():
    response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_config_describes_prometheus_source_and_scoring_endpoint():
    response = client.get("/config")

    assert response.status_code == 200
    config = response.json()
    assert config["source"]["type"] == "Prometheus"
    assert config["source"]["path"] == "/api/v1/query_range"
    assert config["source"]["params"]["range"] == 3600
    assert config["source"]["params"]["step"] == 60
    assert config["scoring"]["path"] == "/scoring"
    assert config["scoring"]["params"]["name"] == "sample_cpu_average"


def test_scoring_returns_average_for_each_series_and_preserves_metric_labels():
    response = client.post(
        "/scoring",
        json={
            "data": [
                {
                    "metric": {"node": "worker-1"},
                    "values": [[1718600000, "5"], [1718600060, "10"]],
                },
                {
                    "metric": {"node": "worker-2"},
                    "values": [[1718600000, "2"], [1718600060, "6"]],
                },
            ]
        },
    )

    assert response.status_code == 200
    assert response.json() == {
        "results": [
            {"metric": {"node": "worker-1"}, "score": 7.5},
            {"metric": {"node": "worker-2"}, "score": 4.0},
        ]
    }


def test_scoring_accepts_empty_or_null_data():
    empty_response = client.post("/scoring", json={"data": []})
    null_response = client.post("/scoring", json={"data": None})

    assert empty_response.status_code == 200
    assert empty_response.json() == {"results": []}
    assert null_response.status_code == 200
    assert null_response.json() == {"results": []}
