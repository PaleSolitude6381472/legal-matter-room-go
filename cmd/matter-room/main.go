package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"example.com/legal-matter-room/internal/matterroom"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	workflow := matterroom.NewWorkflow(matterroom.NewClient(apiKey))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /matters/match", func(w http.ResponseWriter, r *http.Request) {
		var intake matterroom.MatterIntake
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&intake); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid intake"})
			return
		}
		assignment, err := workflow.Match(r.Context(), intake)
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *matterroom.InfraiError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				status = apiErr.HTTPStatus
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, assignment)
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("matter room listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
