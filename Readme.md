# RMM – Remote Management System (Go)

Lightweight client-server system for managing PCs in a local network.
**No external dependencies** – Go standard library only.

## Project Structure

```
rmm/
├── cmd/
│   ├── server/main.go       # Server binary
│   └── client/main.go       # Client binary
├── internal/
│   ├── api/types.go         # Shared data types
│   └── store/store.go       # JSON persistence
├── go.mod
└── Makefile
```

## How It Works

The client runs two parallel loops:

- **Heartbeat loop** – reports status (uptime, OS, hostname) to the server at a regular interval
- **Poll loop** – holds an open HTTP connection to `GET /poll` (long polling). The server blocks until a task is available, then responds immediately. The client reconnects right away after each response.

This means tasks are dispatched in near-real-time without the server ever initiating outbound connections.

```
Client                            Server
  |                                 |
  |-- GET /poll ------------------->|  (blocks, waiting for a task)
  |                                 |  ... a task arrives via POST /clients/{id}/task ...
  |<-- 200 { tasks: [...] } --------|
  |-- executes tasks                |
  |-- POST /task-result ----------->|
  |-- GET /poll ------------------->|  (reconnects immediately)
  |                                 |
```

## Build

```bash
# Build both binaries
make build

# Build server or client individually
make build-server
make build-client

# Cross-compile client for Windows
make build-windows

# Cross-compile client for Linux
make build-linux
```

## Running

### Server

```bash
RMM_API_KEY=secretkey \
RMM_ADDR=:8080 \
RMM_DATA_FILE=clients.json \
./bin/rmm-server
```

| Variable        | Default        | Description                        |
|-----------------|----------------|------------------------------------|
| `RMM_API_KEY`   | `changeme`     | Shared API key                     |
| `RMM_ADDR`      | `:8080`        | Server bind address                |
| `RMM_DATA_FILE` | `clients.json` | Path to the JSON persistence file  |

### Client (on each PC)

```bash
RMM_API_KEY=secretkey \
RMM_SERVER=http://192.168.1.10:8080 \
RMM_HEARTBEAT_INTERVAL=30s \
./bin/rmm-client
```

| Variable                  | Default                 | Description                              |
|---------------------------|-------------------------|------------------------------------------|
| `RMM_API_KEY`             | `changeme`              | Must match the server key                |
| `RMM_SERVER`              | `http://localhost:8080` | Server URL                               |
| `RMM_HEARTBEAT_INTERVAL`  | `30s`                   | How often to send a status heartbeat     |
| `RMM_CLIENT_ID`           | Hostname                | Override the client ID                   |

## API Endpoints

All endpoints require the header: `X-API-Key: <your-key>`

### `GET /poll?client_id=<id>`
Long-poll endpoint for the client. Blocks until a task is available or 30 seconds elapse.
The client reconnects immediately after each response.

```bash
curl -H "X-API-Key: secretkey" \
  "http://localhost:8080/poll?client_id=my-pc"
```

### `GET /clients`
List all known clients with their current online status.
A client is considered online if its last heartbeat was within the past 2 minutes.

```bash
curl -H "X-API-Key: secretkey" http://localhost:8080/clients
```

### `POST /clients/{id}/task`
Enqueue a task for a client. Delivered on the next poll response (near-instant).
Returns the `task_id` to use for fetching the result later.

```bash
# Run a shell command
curl -X POST -H "X-API-Key: secretkey" \
  -H "Content-Type: application/json" \
  -d '{"type":"exec","payload":{"command":"df -h"}}' \
  http://localhost:8080/clients/my-pc/task
# -> {"task_id":"task-1718000000000000000"}

# Query a resource
curl -X POST -H "X-API-Key: secretkey" \
  -H "Content-Type: application/json" \
  -d '{"type":"resource","payload":{"name":"os"}}' \
  http://localhost:8080/clients/my-pc/task
```

### `GET /tasks/{id}/result`
Retrieve the result of a completed task by its ID.
Returns 404 if the task has not completed yet.

```bash
curl -H "X-API-Key: secretkey" \
  http://localhost:8080/tasks/task-1718000000000000000/result
```

### `POST /heartbeat`
Used by the client to report its status. Not intended for manual use.

### `POST /task-result`
Used by the client to submit task results. Not intended for manual use.

## Task Types

### `exec`
Runs a shell command on the client and returns combined stdout/stderr output and the exit code.

- Linux/macOS: executed via `sh -c`
- Windows: executed via `cmd /C`

```json
{ "type": "exec", "payload": { "command": "ipconfig /all" } }
```

### `resource`
Queries a specific system value without spawning a shell.

| `name`     | Extra field       | Returns                    |
|------------|-------------------|----------------------------|
| `hostname` |                   | Machine hostname            |
| `os`       |                   | OS and architecture         |
| `pid`      |                   | Client process ID           |
| `env`      | `key` (required)  | Value of an env variable    |

```json
{ "type": "resource", "payload": { "name": "env", "key": "PATH" } }
```

## Full Workflow Example

```bash
# 1. Send a task and capture the task ID
TASK_ID=$(curl -s -X POST -H "X-API-Key: secretkey" \
  -H "Content-Type: application/json" \
  -d '{"type":"exec","payload":{"command":"uptime"}}' \
  http://localhost:8080/clients/my-pc/task | jq -r .task_id)

# 2. Fetch the result once the client has executed it
curl -H "X-API-Key: secretkey" \
  http://localhost:8080/tasks/$TASK_ID/result
```

## Ideas for Extension
- [ ] HTTPS / TLS support
- [ ] Web UI for client overview
- [ ] Additional resource types (CPU, RAM, disk usage)
- [ ] Install client as a Windows service or systemd unit
- [ ] Persist task results to disk across server restarts
