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

type server struct {
	store  *store.Store
	apiKey string
}

const pollTimeout = 30 * time.Second

func main() {
	apiKey := getEnv("RMM_API_KEY", "changeme")
	addr := getEnv("RMM_ADDR", ":8080")
	dataFile := os.Getenv("RMM_DATA_FILE") // empty = in-memory mode

	s, err := store.New(dataFile)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}

	srv := &server{store: s, apiKey: apiKey}

	if dataFile == "" {
		log.Printf("RMM Server starting on %s (storage: in-memory)", addr)
	} else {
		log.Printf("RMM Server starting on %s (storage: %s)", addr, dataFile)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /heartbeat", srv.auth(srv.handleHeartbeat))
	mux.HandleFunc("POST /task-result", srv.auth(srv.handleTaskResult))
	mux.HandleFunc("GET /clients", srv.auth(srv.handleListClients))
	mux.HandleFunc("POST /clients/{id}/task", srv.auth(srv.handleEnqueueTask))
	mux.HandleFunc("GET /tasks/{id}/result", srv.auth(srv.handleGetTaskResult))
	mux.HandleFunc("GET /poll", srv.auth(srv.handlePoll))

	log.Fatal(http.ListenAndServe(addr, mux))
}

// auth is a middleware that validates the X-API-Key header
func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != s.apiKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// handleHeartbeat handles POST /heartbeat
// The client reports its status and receives pending tasks in return.
func (s *server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req api.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if req.ClientID == "" {
		http.Error(w, "client_id missing", http.StatusBadRequest)
		return
	}

	s.store.Upsert(req)

	tasks := s.store.PopTasks(req.ClientID)
	resp := api.HeartbeatResponse{Tasks: tasks}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Printf("[%s] Heartbeat received (uptime: %.0fs)", req.ClientID, req.Uptime)
}

// handleTaskResult handles POST /task-result
// The client submits the result of a previously received task.
func (s *server) handleTaskResult(w http.ResponseWriter, r *http.Request) {
	var result api.TaskResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	s.store.SaveTaskResult(result)

	if result.Error != "" {
		log.Printf("[%s] Task %s ERROR: %s", result.ClientID, result.TaskID, result.Error)
	} else {
		log.Printf("[%s] Task %s OK:\n%s", result.ClientID, result.TaskID, result.Output)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListClients handles GET /clients
// Returns all known clients with their current online status.
func (s *server) handleListClients(w http.ResponseWriter, r *http.Request) {
	clients := s.store.All()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

// handleEnqueueTask handles POST /clients/{id}/task
// Adds a task to the client's queue and wakes up any active long-poll connection.
func (s *server) handleEnqueueTask(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	if _, ok := s.store.Get(clientID); !ok {
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

	s.store.AddTask(clientID, task)
	log.Printf("[%s] Task %s (%s) enqueued", clientID, task.ID, task.Type)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"task_id": task.ID})
}

// handleGetTaskResult handles GET /tasks/{id}/result
// Returns the result of a completed task by its ID.
func (s *server) handleGetTaskResult(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	result, err := s.store.GetTaskResult(taskID)
	if err != nil {
		http.Error(w, "Task result not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handlePoll handles GET /poll
// Long-polling endpoint: the client blocks here until tasks are available or
// the timeout expires. On timeout, responds with an empty task list so the
// client immediately reconnects. The client must send its client_id as a query
// parameter: GET /poll?client_id=<id>
func (s *server) handlePoll(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "client_id query parameter missing", http.StatusBadRequest)
		return
	}

	// If tasks are already queued, return them immediately without blocking
	if tasks := s.store.PopTasks(clientID); len(tasks) > 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.HeartbeatResponse{Tasks: tasks})
		log.Printf("[%s] Poll: %d task(s) dispatched immediately", clientID, len(tasks))
		return
	}

	// No tasks yet – subscribe and wait
	notify := s.store.Subscribe(clientID)
	defer s.store.Unsubscribe(clientID)

	select {
	case <-notify:
		tasks := s.store.PopTasks(clientID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.HeartbeatResponse{Tasks: tasks})
		log.Printf("[%s] Poll: %d task(s) dispatched after wake-up", clientID, len(tasks))
	case <-time.After(pollTimeout):
		// Return an empty response so the client reconnects
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.HeartbeatResponse{})
	case <-r.Context().Done():
		// Client disconnected
		log.Printf("[%s] Poll: client disconnected", clientID)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
