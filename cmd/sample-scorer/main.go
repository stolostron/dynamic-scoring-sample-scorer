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
