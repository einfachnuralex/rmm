package api

import "time"

// HeartbeatRequest is sent by the client to the server on every interval
type HeartbeatRequest struct {
	ClientID string  `json:"client_id"` // Unique ID (e.g. hostname)
	Hostname string  `json:"hostname"`
	OS       string  `json:"os"`
	Arch     string  `json:"arch"`
	Uptime   float64 `json:"uptime_seconds"`
}

// HeartbeatResponse optionally contains tasks for the client to execute
type HeartbeatResponse struct {
	Tasks []Task `json:"tasks,omitempty"`
}

// Task describes a unit of work the client should perform
type Task struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`    // "exec" | "resource"
	Payload map[string]string `json:"payload"` // e.g. {"command": "hostname"}
}

// TaskResult holds the outcome of an executed task
type TaskResult struct {
	TaskID   string `json:"task_id"`
	ClientID string `json:"client_id"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
}

// ClientRecord is the persisted representation of a known client
type ClientRecord struct {
	ClientID    string    `json:"client_id"`
	Hostname    string    `json:"hostname"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	UptimeSec   float64   `json:"uptime_seconds"`
	LastContact time.Time `json:"last_contact"`
	Online      bool      `json:"online"`
}
