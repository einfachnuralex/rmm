.PHONY: build build-server build-client clean run-server run-client

build: build-server build-client

build-server:
	GOOS=linux GOARCH=amd64 go build -o bin/rmm-server ./cmd/server

build-client:
	GOOS=linux GOARCH=amd64 go build -o bin/rmm-client ./cmd/client

clean:
	rm -rf bin/

run-server:
	RMM_API_KEY=secretkey RMM_ADDR=:8080 go run ./cmd/server

run-client:
	RMM_API_KEY=secretkey RMM_SERVER=http://localhost:8080 RMM_HEARTBEAT_INTERVAL=10s go run ./cmd/client

test:
	go test -v ./...
