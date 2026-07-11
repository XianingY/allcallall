# AllCallAll Project Makefile
# Common commands for development

PYTHON ?= python3
AGENT_RUNTIME_PYTHON ?= $(PYTHON)
RAG_RUNTIME_PYTHON ?= $(PYTHON)

.PHONY: help setup build-android build-ios clean test run-api run-agent-runtime run-rag-runtime run-user-service run-agent-worker run-outbox-worker run-data-worker run-search-worker run-cleanup-worker beta-seed interview-demo interview-demo-live interview-live-suite interview-load-suite interview-bench dashboard-bench interview-microservice-demo agent-runtime-test python-agent-eval python-rag-eval agent-eval rag-eval rerank-eval workflow-eval task-eval agent-demo-report resume-eval ai-portfolio-eval ai-agent-jd-eval mcp-tool-server realtime-replay-bench chat-ws-replay-bench web-contract-check web-performance-check helm-check

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
	@echo "  make run-agent-runtime - Start Python LangGraph Agent Runtime"
	@echo "  make run-rag-runtime - Start Python RAG Runtime"
	@echo "  make run-user-service - Start standalone gRPC User Service"
	@echo "  make run-agent-worker - Start standalone Agent worker"
	@echo "  make run-outbox-worker - Start standalone Outbox worker"
	@echo "  make run-data-worker  - Start standalone Kafka settlement Data worker"
	@echo "  make run-search-worker - Start standalone Elasticsearch indexing worker"
	@echo "  make run-cleanup-worker - Start standalone Cleanup worker"
	@echo "  make beta-seed        - Seed a small-team Beta demo workspace"
	@echo "  make interview-demo   - Run local interview demo evidence suite"
	@echo "  make interview-demo-live - Start MySQL/Redis and seed live interview demo data"
	@echo "  make interview-live-suite - Run MySQL/Redis live interview smoke suite"
	@echo "  make interview-load-suite - Generate local interview load suite artifacts"
	@echo "  make interview-microservice-demo - Run API + standalone worker demo"
	@echo "  make dashboard-bench - Run enterprise dashboard and message-list benchmarks"
	@echo "  make agent-eval       - Run deterministic Agent eval harness"
	@echo "  make agent-runtime-test - Run Python Agent Runtime tests and checks"
	@echo "  make python-agent-eval - Run Python LangGraph task eval fixtures"
	@echo "  make python-rag-eval - Run Python RAG Runtime eval fixtures"
	@echo "  make rag-eval         - Run deterministic RAG retrieval eval harness"
	@echo "  make rerank-eval      - Run RAG rerank eval with baseline comparison"
	@echo "  make workflow-eval    - Run deterministic Workflow multi-agent eval harness"
	@echo "  make task-eval        - Run deterministic black-box task eval harness"
	@echo "  make agent-demo-report - Generate combined Agent/RAG/Workflow demo report"
	@echo "  make resume-eval      - Generate resume-oriented Agent KPI artifacts"
	@echo "  make ai-portfolio-eval - Generate AI Agent portfolio evidence bundle"
	@echo "  make ai-agent-jd-eval - Generate Python Agent + RAG JD evidence bundle"
	@echo "  make mcp-tool-server  - Start MCP-compatible read-only tool server"
	@echo "  make interview-bench  - Run local Agent/outbox benchmark"
	@echo "  make realtime-replay-bench - Run local realtime replay benchmark"
	@echo "  make chat-ws-replay-bench - Run authenticated chat WebSocket replay benchmark"
	@echo "  make web-contract-check - Verify OpenAPI and generated Web types are synchronized"
	@echo "  make web-performance-check - Build Web and enforce bundle budgets"
	@echo "  make helm-check        - Lint and render the Kubernetes Helm chart"
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

run-agent-runtime:
	@echo "Starting Python LangGraph Agent Runtime..."
	cd agent-runtime && uvicorn app.main:app --reload --port $${PY_AGENT_RUNTIME_PORT:-8090}

run-rag-runtime:
	@echo "Starting Python RAG Runtime..."
	cd rag-runtime && uvicorn app.main:app --reload --port $${PY_RAG_RUNTIME_PORT:-8091}

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

beta-seed:
	@echo "Seeding small-team Beta demo workspace..."
	cd backend && go run ./cmd/beta-seed

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

helm-check:
	helm lint infra/helm/allcallall
	helm template allcallall infra/helm/allcallall --namespace allcallall > /tmp/allcallall-helm.yaml
	kubeconform -strict -summary -ignore-missing-schemas /tmp/allcallall-helm.yaml

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

rerank-eval:
	@echo "Running deterministic RAG rerank eval harness..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/allcallallctl rerank-eval -out ../docs/interview/generated-rerank-eval

workflow-eval:
	@echo "Running deterministic Workflow eval harness..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/agent-eval -fixture ./internal/agent/testdata/workflow_eval_cases.json

task-eval:
	@echo "Running deterministic black-box task eval harness..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/allcallallctl task-eval -runtime $${AGENT_RUNTIME:-go} -fixture ./internal/agent/testdata/task_eval_cases.json

agent-runtime-test:
	@echo "Running Python Agent Runtime tests..."
	cd agent-runtime && pytest
	cd agent-runtime && ruff check .
	cd agent-runtime && mypy .

python-agent-eval:
	@echo "Running Python LangGraph Agent Runtime eval..."
	cd agent-runtime && $(AGENT_RUNTIME_PYTHON) -m app.eval_runner --out evals/reports

python-rag-eval:
	@echo "Running Python RAG Runtime eval..."
	cd rag-runtime && $(RAG_RUNTIME_PYTHON) -m app.eval_runner --out evals/reports

agent-demo-report:
	@echo "Generating combined Agent demo report..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/allcallallctl eval -provider $${AGENT_PROVIDER:-rules} -out ../docs/interview/generated-agent-report

resume-eval:
	@echo "Generating resume-oriented Agent KPI report..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/allcallallctl resume-eval -provider $${AGENT_PROVIDER:-rules} -out ../docs/interview/generated-resume-eval

ai-portfolio-eval:
	@echo "Generating AI Agent portfolio eval bundle..."
	@mkdir -p /tmp/allcallall-go-cache
	cd agent-runtime && $(AGENT_RUNTIME_PYTHON) -m app.eval_runner --out evals/reports
	cd rag-runtime && $(RAG_RUNTIME_PYTHON) -m app.eval_runner --out evals/reports
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/allcallallctl ai-portfolio-eval -provider $${AGENT_PROVIDER:-rules} -out ../docs/interview/generated-ai-portfolio-eval

ai-agent-jd-eval: python-agent-eval python-rag-eval
	@echo "Generating AI Agent JD eval bundle..."
	$(PYTHON) scripts/ai_agent_jd_eval.py --out docs/interview/generated-ai-agent-jd-eval

mcp-tool-server:
	@echo "Starting MCP-compatible read-only tool server..."
	cd backend && go run ./cmd/mcp-tool-server

interview-bench:
	@echo "Running local Agent/outbox benchmark..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/interview-bench -conversations 25 -batch-size 50

dashboard-bench:
	@echo "Running enterprise dashboard and message-list benchmarks..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go test -run '^$$' -bench 'Benchmark(GetOrganizationAdminSummary|ListMessages)' ./internal/collaboration

realtime-replay-bench:
	@echo "Running local realtime replay benchmark..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/realtime-replay-bench -events 2000 -recipients 10 -replay-window 120 -replay-limit 100

chat-ws-replay-bench:
	@echo "Running authenticated chat WebSocket replay benchmark..."
	@mkdir -p /tmp/allcallall-go-cache
	cd backend && GOCACHE=$${GOCACHE:-/tmp/allcallall-go-cache} go run ./cmd/chat-ws-replay-bench -events 2000 -recipients 10 -replay-window 120 -replay-limit 100 -clients 5

web-contract-check:
	@echo "Checking Web OpenAPI contract..."
	cd web && npm run contract:check

web-performance-check:
	@echo "Checking Web bundle budget..."
	cd web && npm run build && npm run bundle:budget

verify:
	@echo "Running verification suite..."
	cd backend && go test ./...
	cd web && npm run tsc -- --noEmit
	cd mobile && npx tsc --noEmit
	cd agent-runtime && pytest
	cd rag-runtime && pytest

# ===========================
# Development Commands
# ===========================

dev-android:
	@echo "Starting Metro bundler and Android emulator..."
	cd mobile && npx expo start --android

dev-ios:
	@echo "Starting Metro bundler and iOS simulator..."
	cd mobile && npx expo start --ios
