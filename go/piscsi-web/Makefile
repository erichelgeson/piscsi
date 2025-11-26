.DEFAULT_GOAL := build

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# Binary names
BINARY_NAME=piscsi-web
BINARY_UNIX=$(BINARY_NAME)_unix
MOCK_BINARY=mock-piscsi

# Build flags
LDFLAGS=-ldflags "-s -w"

all: test build

build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) -v ./cmd/piscsi-web

mock:
	@echo "Building mock PiSCSI daemon..."
	$(GOBUILD) $(LDFLAGS) -o $(MOCK_BINARY) -v ./cmd/mock-piscsi
	@echo "✅ Mock daemon created: $(MOCK_BINARY)"
	@echo "Run with: ./$(MOCK_BINARY)"

build-linux-arm64:
	@echo "Building for ARM64 (Raspberry Pi 4/5)..."
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
	  CC=aarch64-unknown-linux-gnu-gcc \
	  CGO_LDFLAGS="-L$(ARM64_PAM_LIB)" \
	  $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-arm64 ./cmd/piscsi-web
	@echo "✅ ARM64 binary created: $(BINARY_NAME)-arm64"
	@ls -lh $(BINARY_NAME)-arm64

build-linux-armv7:
	@echo "Building for ARMv7 (Raspberry Pi 2/3)..."
	CGO_ENABLED=1 GOOS=linux GOARCH=arm GOARM=7 \
	  CC=armv7l-unknown-linux-gnueabihf-gcc \
	  CGO_LDFLAGS="-L$(ARMV7_PAM_LIB)" \
	  $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-armv7 ./cmd/piscsi-web
	@echo "✅ ARMv7 binary created: $(BINARY_NAME)-armv7"
	@ls -lh $(BINARY_NAME)-armv7

test:
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-short:
	$(GOTEST) -v -short ./...

test-server:
	$(GOTEST) -v ./internal/server

test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "✅ Coverage report generated: coverage.html"

test-verbose:
	$(GOTEST) -v -race ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f $(MOCK_BINARY)
	rm -f coverage.txt coverage.html

run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./cmd/piscsi-web
	./$(BINARY_NAME)

deps:
	$(GOMOD) download
	$(GOMOD) verify

tidy:
	$(GOMOD) tidy

proto:
	cd proto && ./generate.sh

install:
	@echo "Installing PiSCSI Web Interface..."
	@./scripts/install.sh

uninstall:
	@echo "Uninstalling PiSCSI Web Interface..."
	@./scripts/uninstall.sh

.PHONY: all build mock build-linux-arm64 build-linux-armv7 test test-short test-server test-coverage test-verbose clean run deps tidy proto install uninstall
