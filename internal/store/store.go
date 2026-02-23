package store

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/einfachnuralex/rmm/internal/api"
)

const offlineThreshold = 2 * time.Minute

type Store struct {
	mu       sync.RWMutex
	filePath string
	clients  map[string]*api.ClientRecord
	tasks    map[string][]api.Task // clientID -> pending tasks
}

func New(filePath string) (*Store, error) {
	s := &Store{
		filePath: filePath,
		clients:  make(map[string]*api.ClientRecord),
		tasks:    make(map[string][]api.Task),
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

// PopTasks returns and removes all pending tasks for the given client
func (s *Store) PopTasks(clientID string) []api.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks := s.tasks[clientID]
	delete(s.tasks, clientID)
	return tasks
}

// AddTask enqueues a task for the given client
func (s *Store) AddTask(clientID string, task api.Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[clientID] = append(s.tasks[clientID], task)
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
