.PHONY: help dev-backend dev-frontend build-backend build-frontend build-all docker-up docker-down docker-build test clean build-linux build-windows build-mac

# Default target
help:
	@echo "MemoDrive Makefile Commands:"
	@echo ""
	@echo "Development:"
	@echo "  make dev-chroma      - Start Chroma vector DB (Docker, port 8000)"
	@echo "  make dev-backend     - Run backend in development mode"
	@echo "  make dev-frontend    - Run frontend in development mode"
	@echo ""
	@echo "Docker Deployment:"
	@echo "  make docker-up       - Start all services with Docker Compose"
	@echo "  make docker-down     - Stop all Docker Compose services"
	@echo "  make docker-build    - Rebuild and start all Docker Compose services"
	@echo ""
	@echo "Local Build:"
	@echo "  make build-all       - Build both frontend and backend for current OS"
	@echo "  make build-backend   - Build backend binary for current OS"
	@echo "  make build-frontend  - Build frontend production assets"
	@echo ""
	@echo "Cross-Platform Build (Backend):"
	@echo "  make build-linux     - Build backend for Linux (amd64)"
	@echo "  make build-windows   - Build backend for Windows (amd64)"
	@echo "  make build-mac       - Build backend for macOS (arm64/Apple Silicon)"
	@echo ""
	@echo "Testing & Cleanup:"
	@echo "  make test            - Run backend unit tests"
	@echo "  make clean           - Clean build artifacts"

# --- Development ---
dev-backend:
	@export $$(grep -v '^#' .env | xargs) && \
	cd backend && \
	CHROMA_BASE_URL=http://localhost:8000 \
	OLLAMA_BASE_URL=http://localhost:11434 \
	go run ./cmd/server

dev-chroma:
	docker compose up -d chroma
	@echo "Chroma running at http://localhost:8000"

stop-chroma:
	docker compose down chroma

dev-frontend:
	cd frontend && npm run dev

# --- Docker Deployment ---
docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-build:
	docker compose up -d --build

# --- Local Build ---
build-backend:
	cd backend && go build -o bin/server ./cmd/server

build-frontend:
	cd frontend && npm run build

build-all: build-frontend build-backend

# --- Cross-Platform Build ---
build-linux:
	cd backend && GOOS=linux GOARCH=amd64 go build -o bin/server-linux-amd64 ./cmd/server

build-windows:
	cd backend && GOOS=windows GOARCH=amd64 go build -o bin/server-windows-amd64.exe ./cmd/server

build-mac:
	cd backend && GOOS=darwin GOARCH=arm64 go build -o bin/server-darwin-arm64 ./cmd/server

# --- Testing & Cleanup ---
test:
	cd backend && go test -v ./...

clean:
	rm -rf backend/bin
	rm -rf frontend/dist
