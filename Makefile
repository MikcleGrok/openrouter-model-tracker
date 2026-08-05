GO := go
BINARY := bin/openrouter
DATA_DIR := $(CURDIR)
OUTPUT := $(CURDIR)/docs/openrouter-model-comparison.md
GO_FILES := $(shell git ls-files '*.go')

VERSION := $(shell git describe --tags --always --dirty)
RELEASE_VERSION := $(shell git describe --tags --exact-match 2>/dev/null)

.DEFAULT_GOAL := help

.PHONY: build test race vet fmt fmt-check check init refresh history table release-build clean help FORCE

build: $(BINARY)

$(BINARY): FORCE Makefile $(GO_FILES) go.mod go.sum
	@mkdir -p $(dir $@)
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o $@ ./cmd/openrouter

FORCE:

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@test -z "$$(gofmt -l $(GO_FILES))" || { printf '%s\n' 'Go files need gofmt:'; gofmt -l $(GO_FILES); exit 1; }

check: build
	$(BINARY) check --data-dir $(DATA_DIR) --output /dev/null

init:
	./scripts/init.sh

refresh: build
	$(BINARY) refresh --data-dir $(DATA_DIR) --output $(OUTPUT)

history: build
	$(BINARY) history --data-dir $(DATA_DIR)

table: build
	$(BINARY) table --data-dir $(DATA_DIR)

release-build:
	@test -n "$(RELEASE_VERSION)" || { printf '%s\n' 'release-build requires checkout at an exact git tag'; exit 1; }
	@test "$(VERSION)" = "$(RELEASE_VERSION)" || { printf '%s\n' 'release-build requires a clean checkout at an exact git tag'; exit 1; }
	@mkdir -p $(dir $(BINARY))
	$(GO) build -ldflags "-X main.version=$(RELEASE_VERSION)" -o $(BINARY) ./cmd/openrouter

clean:
	rm -f $(BINARY)

help:
	@printf '%s\n' \
		'build          Build bin/openrouter with the current version' \
		'test           Run all Go tests' \
		'race           Run all Go tests with the race detector' \
		'vet            Run go vet' \
		'fmt            Format tracked Go files' \
		'fmt-check      Check tracked Go files without changing them' \
		'check          Run the read-only CLI check against this checkout' \
		'init           Build, initialize, refresh, and open the report on macOS' \
		'refresh        Refresh data and the generated comparison document' \
		'history        Show price history for this checkout' \
		'table          Show local model data as a plain-text table' \
		'release-build  Build with the exact checked-out git tag' \
		'clean           Remove only bin/openrouter' \
		'help            Show this list of targets'
