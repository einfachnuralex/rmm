package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/einfachnuralex/rmm/internal/api"
)

const offlineThreshold = 2 * time.Minute

type Store struct {
	mu          sync.RWMutex
	filePath    string
	clients     map[string]*api.ClientRecord
	tasks       map[string][]api.Task     // clientID -> pending tasks
	taskResults map[string]api.TaskResult // taskID -> result
	notify      map[string]chan struct{}  // clientID -> long-poll wake-up channel
}

func New(filePath string) (*Store, error) {
	s := &Store{
		filePath:    filePath,
		clients:     make(map[string]*api.ClientRecord),
		tasks:       make(map[string][]api.Task),
		taskResults: make(map[string]api.TaskResult),
		notify:      make(map[string]chan struct{}),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Upsert updates or creates a client record
func (s *Store) Upsert(req api.HeartbeatRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.clients[req.ClientID]
	if !ok {
		r = &api.ClientRecord{ClientID: req.ClientID}
		s.clients[req.ClientID] = r
	}
	r.Hostname = req.Hostname
	r.OS = req.OS
	r.Arch = req.Arch
	r.UptimeSec = req.Uptime
	r.LastContact = time.Now()
	r.Online = true

	_ = s.save()
}

// Subscribe returns a channel that is closed when a new task is available for
// the client. Any previously existing channel for the client is replaced.
func (s *Store) Subscribe(clientID string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{}, 1)
	s.notify[clientID] = ch
	return ch
}

// Unsubscribe removes the long-poll channel for the client.
func (s *Store) Unsubscribe(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.notify, clientID)
}

// PopTasks returns and removes all pending tasks for the given client
func (s *Store) PopTasks(clientID string) []api.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := s.tasks[clientID]
	delete(s.tasks, clientID)
	return tasks
}

// AddTask enqueues a task for the given client and wakes up any active long-poll
func (s *Store) AddTask(clientID string, task api.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[clientID] = append(s.tasks[clientID], task)

	// Signal the waiting long-poll connection, if any
	if ch, ok := s.notify[clientID]; ok {
		select {
		case ch <- struct{}{}:
		default: // already signalled, don't block
		}
	}
}

// SaveTaskResult persists the result of an executed task
func (s *Store) SaveTaskResult(result api.TaskResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskResults[result.TaskID] = result
}

// GetTaskResult retrieves a task result by task ID
func (s *Store) GetTaskResult(taskID string) (api.TaskResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result, ok := s.taskResults[taskID]
	if !ok {
		return api.TaskResult{}, fmt.Errorf("task result not found: %s", taskID)
	}
	return result, nil
}

// All returns all known clients with an up-to-date online status
func (s *Store) All() []api.ClientRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	result := make([]api.ClientRecord, 0, len(s.clients))
	for _, r := range s.clients {
		r.Online = now.Sub(r.LastContact) < offlineThreshold
		result = append(result, *r)
	}
	return result
}

// Get returns a single client record by ID
func (s *Store) Get(clientID string) (*api.ClientRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.clients[clientID]
	if !ok {
		return nil, false
	}
	cp := *r
	return &cp, true
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		return nil // empty file is fine
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.clients)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.clients, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0600)
}
