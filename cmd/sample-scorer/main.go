package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/stolostron/dynamic-scoring-framework-sample-scorer/internal/scorer"
)

func main() {
	configs, err := scorer.ConfigsFromEnv()
	if err != nil {
		log.Fatalf("invalid scorer configuration: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           scorer.NewHandler(configs),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("starting sample scorer on :%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("sample scorer server failed: %v", err)
	}
}
