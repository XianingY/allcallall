# AllCallAll Project Makefile
# Common commands for development

.PHONY: help setup setup-android-onnxruntime build-android build-ios clean test

# Default target
help:
	@echo "AllCallAll Development Commands"
	@echo "================================"
	@echo ""
	@echo "Setup:"
	@echo "  make setup            - Initialize project (submodules, dependencies)"
	@echo "  make setup-models     - Download ML models"
	@echo "  make setup-android-onnxruntime - Prepare ONNX Runtime Android native libs"
	@echo ""
	@echo "Build:"
	@echo "  make build-android    - Build Android debug APK"
	@echo "  make build-ios        - Build iOS (requires macOS)"
	@echo ""
	@echo "Backend:"
	@echo "  make run-backend      - Start backend server"
	@echo ""
	@echo "Clean:"
	@echo "  make clean            - Clean all build artifacts"
	@echo "  make clean-android    - Clean Android build only"
	@echo ""

# ===========================
# Setup Commands
# ===========================

setup:
	@echo "Initializing git submodules..."
	git submodule update --init --recursive
	@echo "Installing mobile dependencies..."
	cd mobile && npm install
	@echo "Setup complete!"

setup-models:
	@echo "Downloading ML models..."
	cd scripts/translation && ./download_models.sh
	@echo "Models downloaded!"

# ===========================
# Build Commands
# ===========================

build-android:
	@echo "Building Android debug APK..."
	$(MAKE) setup-android-onnxruntime
	cd mobile/android && ./gradlew -I gradle-mirrors.init.gradle :app:assembleDebug
	@echo "APK built at: mobile/android/app/build/outputs/apk/debug/"

build-android-release:
	@echo "Building Android release APK..."
	$(MAKE) setup-android-onnxruntime
	cd mobile/android && ./gradlew -I gradle-mirrors.init.gradle :app:assembleRelease
	@echo "APK built at: mobile/android/app/build/outputs/apk/release/"

setup-android-onnxruntime:
	@echo "Preparing ONNX Runtime Android native libs..."
	cd mobile && bash scripts/setup-android-onnxruntime.sh

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

clean-models:
	@echo "Removing downloaded models..."
	rm -rf models/**/*.bin
	rm -rf models/**/*.onnx
	rm -rf mobile/android/app/src/main/assets/models

# ===========================
# Test Commands
# ===========================

test:
	@echo "Running tests..."
	cd mobile && npm test

test-backend:
	@echo "Running backend tests..."
	cd backend && go test ./...

# ===========================
# Development Commands
# ===========================

dev-android:
	@echo "Starting Metro bundler and Android emulator..."
	cd mobile && npx expo start --android

dev-ios:
	@echo "Starting Metro bundler and iOS simulator..."
	cd mobile && npx expo start --ios
