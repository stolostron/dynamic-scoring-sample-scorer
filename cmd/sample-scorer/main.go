package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tesshuflower/dynamic-scoring-framework-sample-scorer/internal/scorer"
)

func main() {
	config, err := scorer.ConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid scorer configuration: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           scorer.NewHandler(config),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("starting sample scorer on :%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("sample scorer server failed: %v", err)
	}
}
