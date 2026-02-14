# Makefile for nanobot

# Binary name
BINARY_NAME=nanobot
CMD_PATH=./cmd/nanobot/main.go

# Build directory
BUILD_DIR=build

# Versions
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date +%FT%T%z)

# LDFLAGS for versioning (optional, if you want to add version info to binary)
LDFLAGS=-ldflags "-s -w"

.PHONY: all clean linux android mac mac-arm64

all: clean linux android mac mac-arm64

# Linux (amd64)
linux:
	@echo "Building for Linux (amd64)..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64/$(BINARY_NAME) $(CMD_PATH)

# Android (arm64)
android:
	@echo "Building for Android (arm64)..."
	GOOS=android GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/android-arm64/$(BINARY_NAME) $(CMD_PATH)

# MacOS (amd64 - Intel)
mac:
	@echo "Building for macOS (amd64)..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_NAME) $(CMD_PATH)

# MacOS (arm64 - Apple Silicon)
mac-arm64:
	@echo "Building for macOS (arm64)..."
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_NAME) $(CMD_PATH)

# Clean build directory
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	mkdir -p $(BUILD_DIR)

# Help
help:
	@echo "Makefile for $(BINARY_NAME)"
	@echo ""
	@echo "Usage:"
	@echo "  make          Build all targets"
	@echo "  make linux    Build for Linux (amd64)"
	@echo "  make android  Build for Android (arm64)"
	@echo "  make mac      Build for macOS (Intel)"
	@echo "  make mac-arm64 Build for macOS (Apple Silicon)"
	@echo "  make clean    Remove build artifacts"
