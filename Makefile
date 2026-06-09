# AllCallAll Project Makefile
# Common commands for development

.PHONY: help setup build-android build-ios clean test interview-bench realtime-replay-bench chat-ws-replay-bench

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
