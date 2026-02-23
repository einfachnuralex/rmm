# RMM – Remote Management System (Go)

Leichtgewichtiges Client-Server-System zur Verwaltung von PCs im lokalen Netzwerk.
**Keine externen Abhängigkeiten** – nur die Go-Standardbibliothek.

## Projektstruktur

```
rmm/
├── cmd/
│   ├── server/main.go       # Server-Binary
│   └── client/main.go       # Client-Binary
├── internal/
│   ├── api/types.go         # Gemeinsame Datentypen
│   └── store/store.go       # JSON-Persistenz
├── go.mod
└── Makefile
```

## Bauen

```bash
# Beide Binaries bauen
make build

# Nur Server / Client
make build-server
make build-client

# Client für Windows cross-compilieren
make build-windows

# Client für Linux cross-compilieren
make build-linux
```

## Starten

### Server

```bash
RMM_API_KEY=geheimkey \
RMM_ADDR=:8080 \
RMM_DATA_FILE=clients.json \
./bin/rmm-server
```

| Variable        | Standard       | Beschreibung                    |
|-----------------|----------------|---------------------------------|
| `RMM_API_KEY`   | `changeme`     | Gemeinsamer API-Schlüssel       |
| `RMM_ADDR`      | `:8080`        | Bind-Adresse des Servers        |
| `RMM_DATA_FILE` | `clients.json` | Pfad zur Persistenz-Datei       |

### Client (auf jedem PC)

```bash
RMM_API_KEY=geheimkey \
RMM_SERVER=http://192.168.1.10:8080 \
RMM_INTERVAL=30s \
./bin/rmm-client
```

| Variable         | Standard                  | Beschreibung                         |
|------------------|---------------------------|--------------------------------------|
| `RMM_API_KEY`    | `changeme`                | Muss mit Server übereinstimmen       |
| `RMM_SERVER`     | `http://localhost:8080`   | URL des Servers                      |
| `RMM_INTERVAL`   | `30s`                     | Heartbeat-Intervall                  |
| `RMM_CLIENT_ID`  | Hostname                  | Optionale manuelle Client-ID         |

## API-Endpunkte

Alle Endpunkte erfordern den Header: `X-API-Key: <dein-key>`

### `GET /clients`
Liste aller bekannten Clients mit Status.

```bash
curl -H "X-API-Key: geheimkey" http://localhost:8080/clients
```

### `POST /clients/{id}/task`
Aufgabe an einen Client senden. Wird beim nächsten Heartbeat abgeholt.

```bash
# Befehl ausführen
curl -X POST -H "X-API-Key: geheimkey" \
  -H "Content-Type: application/json" \
  -d '{"type":"exec","payload":{"command":"hostname"}}' \
  http://localhost:8080/clients/mein-pc/task

# Resource abfragen
curl -X POST -H "X-API-Key: geheimkey" \
  -H "Content-Type: application/json" \
  -d '{"type":"resource","payload":{"name":"os"}}' \
  http://localhost:8080/clients/mein-pc/task
```

## Aufgaben-Typen

### `exec`
Führt einen Shell-Befehl auf dem Client aus.
- Payload: `{"command": "ipconfig /all"}`
- Unter Linux/macOS: via `sh -c`
- Unter Windows: via `cmd /C`

### `resource`
Fragt eine spezifische Resource ab.
- `{"name": "hostname"}` → Hostname
- `{"name": "os"}` → OS/Architektur
- `{"name": "pid"}` → Prozess-ID
- `{"name": "env", "key": "PATH"}` → Umgebungsvariable

## Erweiterungsideen
- [ ] HTTPS / TLS-Unterstützung
- [ ] Web-UI zur Client-Übersicht
- [ ] Weitere Resource-Typen (CPU, RAM, Disk)
- [ ] Client als Windows-Dienst / systemd-Unit installieren
- [ ] Task-Ergebnisse in der JSON-Datei persistieren
