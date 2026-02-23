package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/einfachnuralex/rmm/internal/api"
)

func main() {
	serverURL := getEnv("RMM_SERVER", "http://localhost:8080")
	apiKey := getEnv("RMM_API_KEY", "changeme")
	interval := getDuration("RMM_INTERVAL", 30*time.Second)

	hostname, _ := os.Hostname()
	clientID := getEnv("RMM_CLIENT_ID", hostname)

	log.Printf("RMM Client starting | ID: %s | Server: %s | Interval: %s", clientID, serverURL, interval)

	client := &http.Client{Timeout: 15 * time.Second}
	startTime := time.Now()

	for {
		uptime := time.Since(startTime).Seconds()

		req := api.HeartbeatRequest{
			ClientID: clientID,
			Hostname: hostname,
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			Uptime:   uptime,
		}

		resp, err := sendHeartbeat(client, serverURL, apiKey, req)
		if err != nil {
			log.Printf("Heartbeat failed: %v", err)
		} else {
			for _, task := range resp.Tasks {
				go handleTask(client, serverURL, apiKey, clientID, task)
			}
		}

		time.Sleep(interval)
	}
}

func sendHeartbeat(client *http.Client, serverURL, apiKey string, req api.HeartbeatRequest) (*api.HeartbeatResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", serverURL+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", apiKey)

	httpResp, err := client.Do(httpReq)
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

func handleTask(client *http.Client, serverURL, apiKey, clientID string, task api.Task) {
	log.Printf("Task received: %s (type: %s)", task.ID, task.Type)

	result := api.TaskResult{
		TaskID:   task.ID,
		ClientID: clientID,
	}

	switch task.Type {
	case "exec":
		output, err := runExec(task.Payload)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Output = output
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

func runExec(payload map[string]string) (string, error) {
	command, ok := payload["command"]
	if !ok || command == "" {
		return "", fmt.Errorf("'command' missing in payload")
	}

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/C", command}
	} else {
		args = []string{"sh", "-c", command}
	}

	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("execution failed: %w", err)
	}
	return string(out), nil
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
	log.Printf("Task %s result sent (status: %d)", result.TaskID, resp.StatusCode)
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
