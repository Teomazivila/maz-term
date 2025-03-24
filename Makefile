.PHONY: build test clean install run lint

# Application name
APP_NAME=maz-term
# Version
VERSION=0.1.0
# Main package path
MAIN_PATH=./cmd/maz-term

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOGET=$(GOCMD) get
GOLINT=golangci-lint

# Build flags
BUILD_FLAGS=-ldflags "-X main.Version=$(VERSION)"

# Binary output
BINARY_NAME=$(APP_NAME)
BINARY_UNIX=$(BINARY_NAME)_unix

all: lint test build

build:
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PATH)

test:
	$(GOTEST) -v ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run:
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	./$(BINARY_NAME)

lint:
	$(GOVET) ./...
	@if [ -x "$(command -v $(GOLINT))" ]; then \
		$(GOLINT) run; \
	else \
		echo "golangci-lint is not installed. Skipping lint."; \
	fi

install:
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME) $(MAIN_PATH)
	mv $(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

# Cross compilation for different platforms
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_UNIX) $(MAIN_PATH)

build-macos:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME)_macos $(MAIN_PATH)

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_NAME).exe $(MAIN_PATH)

build-all: build-linux build-macos build-windows 