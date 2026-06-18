# AllCallAll Project Makefile
# Common commands for development

.PHONY: help setup build-android build-ios clean test run-api run-user-service run-agent-worker run-outbox-worker run-data-worker run-search-worker run-cleanup-worker interview-demo interview-demo-live interview-live-suite interview-load-suite interview-bench interview-microservice-demo agent-eval rag-eval workflow-eval agent-demo-report mcp-tool-server realtime-replay-bench chat-ws-replay-bench

# Default target
help:
	@echo "AllCallAll Development Commands"
	@echo "================================"
	@echo ""
	@echo "Setup:"
	@echo "  make setup            - Install project dependencies"
	@echo ""
	@echo "Build:"
	@echo "  make build-android    - Build Android debug APK"
	@echo "  make build-ios        - Build iOS (requires macOS)"
	@echo ""
	@echo "Backend:"
	@echo "  make run-backend      - Start backend server"
	@echo "  make run-api          - Start API server (EMBEDDED_WORKERS configurable)"
	@echo "  make run-user-service - Start standalone gRPC User Service"
	@echo "  make run-agent-worker - Start standalone Agent worker"
	@echo "  make run-outbox-worker - Start standalone Outbox worker"
	@echo "  make run-data-worker  - Start standalone Kafka settlement Data worker"
	@echo "  make run-search-worker - Start standalone Elasticsearch indexing worker"
	@echo "  make run-cleanup-worker - Start standalone Cleanup worker"
	@echo "  make interview-demo   - Run local interview demo evidence suite"
	@echo "  make interview-demo-live - Start MySQL/Redis and seed live interview demo data"
	@echo "  make interview-live-suite - Run MySQL/Redis live interview smoke suite"
	@echo "  make interview-load-suite - Generate local interview load suite artifacts"
	@echo "  make interview-microservice-demo - Run API + standalone worker demo"
	@echo "  make agent-eval       - Run deterministic Agent eval harness"
	@echo "  make rag-eval         - Run deterministic RAG retrieval eval harness"
	@echo "  make workflow-eval    - Run deterministic Workflow multi-agent eval harness"
	@echo "  make agent-demo-report - Generate combined Agent/RAG/Workflow demo report"
	@echo "  make mcp-tool-server  - Start MCP-compatible read-only tool server"
	@echo "  make interview-bench  - Run local Agent/outbox benchmark"
	@echo "  make realtime-replay-bench - Run local realtime replay benchmark"
	@echo "  make chat-ws-replay-bench - Run authenticated chat WebSocket replay benchmark"
	@echo ""
	@echo "Clean:"
	@echo "  make clean            - Clean all build artifacts"
	@echo "  make clean-android    - Clean Android build only"
	@echo ""

# ===========================
# Setup Commands
# ===========================

setup:
	@echo "Installing mobile dependencies..."
	cd mobile && npm install
	@echo "Setup complete!"

# ===========================
# Build Commands
# ===========================

build-android:
	@echo "Building Android debug APK..."
	cd mobile && bash scripts/run-android-gradle-with-env.sh :app:assembleDebug
	@echo "APK built at: mobile/android/app/build/outputs/apk/debug/"

build-android-release:
	@echo "Building Android release APK..."
	cd mobile && bash scripts/run-android-gradle-with-env.sh :app:assembleRelease
	@echo "APK built at: mobile/android/app/build/outputs/apk/release/"

build-ios:
	@echo "Building iOS..."
	cd mobile/ios && xcodebuild -workspace AllCallAll.xcworkspace -scheme AllCallAll -configuration Debug

# ===========================
# Backend Commands
# ===========================

run-backend:
	@echo "Starting backend server..."
	cd backend && go run cmd/server/main.go

run-api:
	@echo "Starting API server..."
	cd backend && EMBEDDED_WORKERS=$${EMBEDDED_WORKERS:-1} go run ./cmd/server

run-user-service:
	@echo "Starting standalone gRPC User Service..."
	cd backend && go run ./cmd/user-service

run-agent-worker:
	@echo "Starting standalone Agent worker..."
	cd backend && go run ./cmd/agent-worker

run-outbox-worker:
	@echo "Starting standalone Outbox worker..."
	cd backend && go run ./cmd/outbox-worker

run-data-worker:
	@echo "Starting standalone Kafka settlement Data worker..."
	cd backend && go run ./cmd/data-worker

run-search-worker:
	@echo "Starting standalone Elasticsearch search worker..."
	cd backend && go run ./cmd/search-worker

run-cleanup-worker:
	@echo "Starting standalone Cleanup worker..."
	cd backend && go run ./cmd/cleanup-worker

# ===========================
# Clean Commands
# ===========================

clean: clean-android
	@echo "Cleaning all build artifacts..."
	rm -rf mobile/node_modules
	rm -rf mobile/.expo
	@echo "Clean complete!"

clean-android:
	@echo "Cleaning Android build..."
	rm -rf mobile/android/app/.cxx
	rm -rf mobile/android/app/build
	rm -rf mobile/android/.gradle
	cd mobile/android && ./gradlew clean

# ===========================
# Test Commands
# ===========================

test:
	@echo "Running tests..."
	cd mobile && npm test

test-backend:
	@echo "Running backend tests..."
	cd backend && go test ./...

interview-demo:
	@echo "Running local interview demo..."
	bash scripts/interview-demo.sh

interview-demo-live:
	@echo "Running live interview demo seed..."
	MODE=live bash scripts/interview-demo.sh

interview-live-suite:
	@echo "Running MySQL/Redis live interview suite..."
	bash scripts/load/run-interview-live-suite.sh

interview-load-suite:
	@echo "Running local interview load suite..."
	bash scripts/load/run-interview-suite.sh

interview-microservice-demo:
	@echo "Running modular monolith + standalone worker demo..."
	bash scripts/interview-microservice-demo.sh

agent-eval:
	@echo "Running deterministic Agent eval harness..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/agent-eval -provider $${AGENT_PROVIDER:-rules}

rag-eval:
	@echo "Running deterministic RAG eval harness..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/rag-eval

workflow-eval:
	@echo "Running deterministic Workflow eval harness..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/agent-eval -fixture ./internal/agent/testdata/workflow_eval_cases.json

agent-demo-report:
	@echo "Generating combined Agent demo report..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/allcallallctl eval -provider $${AGENT_PROVIDER:-rules} -out ../docs/interview/generated-agent-report

mcp-tool-server:
	@echo "Starting MCP-compatible read-only tool server..."
	cd backend && go run ./cmd/mcp-tool-server

interview-bench:
	@echo "Running local Agent/outbox benchmark..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/interview-bench -conversations 25 -batch-size 50

realtime-replay-bench:
	@echo "Running local realtime replay benchmark..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/realtime-replay-bench -events 2000 -recipients 10 -replay-window 120 -replay-limit 100

chat-ws-replay-bench:
	@echo "Running authenticated chat WebSocket replay benchmark..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/chat-ws-replay-bench -events 2000 -recipients 10 -replay-window 120 -replay-limit 100 -clients 5

# ===========================
# Development Commands
# ===========================

dev-android:
	@echo "Starting Metro bundler and Android emulator..."
	cd mobile && npx expo start --android

dev-ios:
	@echo "Starting Metro bundler and iOS simulator..."
	cd mobile && npx expo start --ios
