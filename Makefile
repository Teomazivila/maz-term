.PHONY: all help build run test test-race cover lint vet fmt fmt-check vuln \
        tidy clean install sample-plugin build-with-plugins release check

APP_NAME    := maz-term
MAIN_PATH   := ./cmd/maz-term
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_FLAGS := -trimpath -ldflags "-s -w -X main.Version=$(VERSION)"

# The pure-Go SQLite driver means no target needs cgo, so every platform below
# produces a self-contained static binary.
export CGO_ENABLED ?= 0

DIST := dist

# Release targets as GOOS/GOARCH pairs.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

all: check build

help:
	@echo "Targets:"
	@echo "  build               build $(APP_NAME) for the host"
	@echo "  run                 build and run"
	@echo "  test                run tests"
	@echo "  test-race           run tests with the race detector"
	@echo "  cover               run tests and report coverage per package"
	@echo "  lint                run golangci-lint (installs nothing; skipped if absent)"
	@echo "  vet fmt fmt-check   go vet / gofmt"
	@echo "  vuln                run govulncheck (skipped if absent)"
	@echo "  check               fmt-check + vet + lint + test-race"
	@echo "  release             cross-compile every supported platform into $(DIST)/"
	@echo "  build-with-plugins  host build with native plugin loading (requires cgo)"
	@echo "  sample-plugin       build the reference plugin as a shared object"

build:
	go build $(BUILD_FLAGS) -o $(APP_NAME) $(MAIN_PATH)

run: build
	./$(APP_NAME)

test:
	go test ./...

test-race:
	go test -race -timeout 300s ./...

cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt:
	gofmt -w $(shell git ls-files '*.go')

# Fails when any tracked Go file is unformatted.
fmt-check:
	@unformatted="$$(gofmt -l $(shell git ls-files '*.go'))"; \
	if [ -n "$$unformatted" ]; then \
		echo "unformatted files:"; echo "$$unformatted"; exit 1; \
	fi

# The command substitution is escaped as $$( ... ) so the shell expands it.
# Written as $( ... ) it was expanded by make as an undefined variable, the test
# always compared against an empty string, and the linter never ran.
lint:
	@if [ -n "$$(command -v golangci-lint)" ]; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping"; \
		echo "install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

vuln:
	@if [ -n "$$(command -v govulncheck)" ]; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed; skipping"; \
		echo "install: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

check: fmt-check vet lint test-race

tidy:
	go mod tidy

# Native plugin loading needs cgo and does not exist on Windows, so it is a
# separate, explicitly opted-in build rather than the default.
build-with-plugins:
	CGO_ENABLED=1 go build $(BUILD_FLAGS) -tags plugins -o $(APP_NAME) $(MAIN_PATH)

sample-plugin:
	CGO_ENABLED=1 go build -tags plugin -buildmode=plugin \
		-o plugins/sample/sample.so ./pkg/plugins/sample
	@echo "record this digest under plugins.allow in your configuration:"
	@shasum -a 256 plugins/sample/sample.so 2>/dev/null || sha256sum plugins/sample/sample.so

release: $(DIST)
	@for platform in $(PLATFORMS); do \
		os="$${platform%%/*}"; arch="$${platform##*/}"; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="$(DIST)/$(APP_NAME)_$${os}_$${arch}$$ext"; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch go build $(BUILD_FLAGS) -o "$$out" $(MAIN_PATH) || exit 1; \
	done
	@cd $(DIST) && shasum -a 256 * > SHA256SUMS 2>/dev/null || sha256sum * > SHA256SUMS
	@echo "release artifacts in $(DIST)/"

$(DIST):
	@mkdir -p $(DIST)

install: build
	go install $(BUILD_FLAGS) $(MAIN_PATH)

clean:
	rm -f $(APP_NAME) coverage.out
	rm -rf $(DIST)
	rm -f plugins/sample/sample.so
