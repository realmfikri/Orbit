.PHONY: build test run lint

GO_CMD=./backend/cmd/orbitserver
BIN_DIR=bin

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/orbitserver $(GO_CMD)
	cd web && npm ci && npm run build

test:
	go test ./...
	cd web && npm ci && npm test

lint:
	go vet ./...
	cd web && npm ci && npm run lint

run:
	go run $(GO_CMD)
