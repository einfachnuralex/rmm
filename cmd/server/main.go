package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/einfachnuralex/rmm/internal/api"
	"github.com/einfachnuralex/rmm/internal/store"
)

func main() {
	apiKey := getEnv("RMM_API_KEY", "changeme")
	addr := getEnv("RMM_ADDR", ":8080")
	dataFile := getEnv("RMM_DATA_FILE", "clients.json")

	s, err := store.New(dataFile)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}

	log.Printf("RMM Server starting on %s (data file: %s)", addr, dataFile)

	mux := http.NewServeMux()

	// Middleware: validate API key
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key != apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	// POST /heartbeat - client check-in
	mux.HandleFunc("POST /heartbeat", auth(func(w http.ResponseWriter, r *http.Request) {
		var req api.HeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if req.ClientID == "" {
			http.Error(w, "client_id missing", http.StatusBadRequest)
			return
		}
		s.Upsert(req)

		tasks := s.PopTasks(req.ClientID)
		resp := api.HeartbeatResponse{Tasks: tasks}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)

		log.Printf("[%s] Heartbeat received (uptime: %.0fs)", req.ClientID, req.Uptime)
	}))

	// POST /task-result - client delivers the result of an executed task
	mux.HandleFunc("POST /task-result", auth(func(w http.ResponseWriter, r *http.Request) {
		var result api.TaskResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if result.Error != "" {
			log.Printf("[%s] Task %s ERROR: %s", result.ClientID, result.TaskID, result.Error)
		} else {
			log.Printf("[%s] Task %s OK:\n%s", result.ClientID, result.TaskID, result.Output)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// GET /clients - list all known clients
	mux.HandleFunc("GET /clients", auth(func(w http.ResponseWriter, r *http.Request) {
		clients := s.All()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(clients)
	}))

	// POST /clients/{id}/task - enqueue a task for a specific client
	mux.HandleFunc("POST /clients/{id}/task", auth(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.PathValue("id")
		if _, ok := s.Get(clientID); !ok {
			http.Error(w, "Client not found", http.StatusNotFound)
			return
		}
		var task api.Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if task.ID == "" {
			task.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
		}
		s.AddTask(clientID, task)
		log.Printf("[%s] Task %s (%s) enqueued", clientID, task.ID, task.Type)
		w.WriteHeader(http.StatusAccepted)
	}))

	log.Fatal(http.ListenAndServe(addr, mux))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
