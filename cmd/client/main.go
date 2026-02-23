package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/einfachnuralex/rmm/internal/api"
	"github.com/einfachnuralex/rmm/internal/utils"
)

func main() {
	serverURL := getEnv("RMM_SERVER", "http://localhost:8080")
	apiKey := getEnv("RMM_API_KEY", "changeme")
	heartbeatInterval := getDuration("RMM_HEARTBEAT_INTERVAL", 30*time.Second)

	hostname, _ := os.Hostname()
	clientID := getEnv("RMM_CLIENT_ID", hostname)

	log.Printf("RMM Client starting | ID: %s | Server: %s | Heartbeat: %s", clientID, serverURL, heartbeatInterval)

	// Use a shared HTTP client with a long timeout to support long polling.
	// The poll request can block for up to 30s on the server side.
	httpClient := &http.Client{Timeout: 60 * time.Second}

	// Goroutine 1: periodic heartbeat for status reporting
	go runHeartbeatLoop(httpClient, serverURL, apiKey, clientID, hostname, heartbeatInterval)

	// Goroutine 2: long-poll loop for immediate task dispatch
	runPollLoop(httpClient, serverURL, apiKey, clientID)
}

// runHeartbeatLoop periodically reports client status to the server.
func runHeartbeatLoop(client *http.Client, serverURL, apiKey, clientID, hostname string, interval time.Duration) {
	for {
		uptime, err := utils.HostUptime()
		if err != nil {
			log.Printf("Failed to read host uptime: %v", err)
		}
		req := api.HeartbeatRequest{
			ClientID: clientID,
			Hostname: hostname,
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Uptime:   uptime,
		}
		if err := sendHeartbeat(client, serverURL, apiKey, req); err != nil {
			log.Printf("Heartbeat failed: %v", err)
		}
		time.Sleep(interval)
	}
}

// runPollLoop blocks on GET /poll and dispatches received tasks.
// It reconnects immediately after each response or error.
func runPollLoop(client *http.Client, serverURL, apiKey, clientID string) {
	for {
		resp, err := poll(client, serverURL, apiKey, clientID)
		if err != nil {
			log.Printf("Poll failed: %v – retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, task := range resp.Tasks {
			go handleTask(client, serverURL, apiKey, clientID, task)
		}
		// No sleep here: reconnect immediately to stay ready for the next task
	}
}

// poll performs a single long-poll request and returns the server response.
func poll(client *http.Client, serverURL, apiKey, clientID string) (*api.HeartbeatResponse, error) {
	req, err := http.NewRequest("GET", serverURL+"/poll?client_id="+clientID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)

	httpResp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with %d", httpResp.StatusCode)
	}

	var resp api.HeartbeatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// sendHeartbeat sends a heartbeat to the server (status only, no tasks expected).
func sendHeartbeat(client *http.Client, serverURL, apiKey string, req api.HeartbeatRequest) error {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", serverURL+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", apiKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("server responded with %d", httpResp.StatusCode)
	}
	log.Printf("Heartbeat sent (uptime: %.0fs)", req.Uptime)
	return nil
}

func handleTask(client *http.Client, serverURL, apiKey, clientID string, task api.Task) {
	log.Printf("Task received: %s (type: %s)", task.ID, task.Type)

	result := api.TaskResult{
		TaskID:   task.ID,
		ClientID: clientID,
	}

	switch task.Type {
	case "exec":
		output, exitCode, err := runExec(task.Payload)
		result.Output = output
		result.ExitCode = exitCode
		if err != nil {
			result.Error = err.Error()
		}

	case "resource":
		output, err := getResource(task.Payload)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Output = output
		}

	default:
		result.Error = fmt.Sprintf("Unknown task type: %s", task.Type)
	}

	sendResult(client, serverURL, apiKey, result)
}

// runExec runs a shell command and returns its output, exit code, and any error.
// The output is always populated (even on non-zero exit), the exit code reflects
// the process exit status, and error is only set for unexpected failures.
func runExec(payload map[string]string) (output string, exitCode int, err error) {
	command, ok := payload["command"]
	if !ok || command == "" {
		return "", 1, fmt.Errorf("'command' missing in payload")
	}

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/C", command}
	} else {
		args = []string{"sh", "-c", command}
	}

	out, execErr := exec.Command(args[0], args[1:]...).CombinedOutput()
	output = string(out)

	if execErr != nil {
		var exitErr *exec.ExitError
		if errors.As(execErr, &exitErr) {
			// Command ran but exited with a non-zero status — this is not an
			// unexpected failure, so we return the exit code without wrapping
			// the error further.
			return output, exitErr.ExitCode(), nil
		}
		// Unexpected failure (e.g. binary not found)
		return output, 1, fmt.Errorf("execution failed: %w", execErr)
	}

	return output, 0, nil
}

func getResource(payload map[string]string) (string, error) {
	resource, ok := payload["name"]
	if !ok {
		return "", fmt.Errorf("'name' missing in payload")
	}

	switch strings.ToLower(resource) {
	case "hostname":
		h, err := os.Hostname()
		return h, err
	case "os":
		return runtime.GOOS + "/" + runtime.GOARCH, nil
	case "env":
		key := payload["key"]
		if key == "" {
			return "", fmt.Errorf("'key' missing for env resource")
		}
		return os.Getenv(key), nil
	case "pid":
		return fmt.Sprintf("%d", os.Getpid()), nil
	default:
		return "", fmt.Errorf("unknown resource: %s", resource)
	}
}

func sendResult(client *http.Client, serverURL, apiKey string, result api.TaskResult) {
	body, _ := json.Marshal(result)
	req, err := http.NewRequest("POST", serverURL+"/task-result", bytes.NewReader(body))
	if err != nil {
		log.Printf("Failed to prepare result: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send result: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("Task %s result sent (exit_code: %d, status: %d)", result.TaskID, result.ExitCode, resp.StatusCode)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
