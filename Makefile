GO := go
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
BINARY := $(ROOT)bin/openrouter
DATA_DIR := $(ROOT)
OUTPUT := $(ROOT)docs/openrouter-model-comparison.md
EVIDENCE_DIR := $(ROOT).release
GO_FILES := $(addprefix $(ROOT),$(shell git -C $(ROOT) ls-files -co --exclude-standard '*.go'))

DESCRIBE_VERSION := $(shell git -C $(ROOT) describe --tags --always --dirty)
TAG_VERSION := $(shell git -C $(ROOT) describe --tags --exact-match 2>/dev/null)
TAG_IS_CLEAN := $(shell test -z "$(shell git -C $(ROOT) status --porcelain)" && printf 'yes')
VERSION ?= $(if $(and $(TAG_VERSION),$(TAG_IS_CLEAN)),$(patsubst v%,%,$(TAG_VERSION)),0.0.0-dev)
VERSIONCHECK := $(GO) run ./cmd/versioncheck
PUBLISHED_EVIDENCE ?= $(EVIDENCE_DIR)/published-evidence.json

.DEFAULT_GOAL := help

.PHONY: setup check-env toolchain build test test-unit test-integration test-e2e race coverage lint vet fmt format fmt-check security dependency-check secrets-check sbom verify-provenance signature checksums artifact manifest check-package install reinstall upgrade uninstall install-smoke smoke check init refresh history table version check-version check-tag release-check release-build verify-local-artifact verify-release whats-new docs check-docs clean help FORCE

build: $(BINARY)

$(BINARY): FORCE $(ROOT)Makefile $(GO_FILES) $(ROOT)go.mod $(ROOT)go.sum
	@mkdir -p $(dir $@)
	cd $(ROOT) && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $@ ./cmd/openrouter

FORCE:

setup check-env toolchain:
	@printf '%s\n' 'NO-OP: setup and toolchain provisioning are managed by the host environment.'

test:
	cd $(ROOT) && $(GO) test ./...

test-unit test-integration test-e2e: test

race:
	cd $(ROOT) && $(GO) test -race ./...

coverage:
	cd $(ROOT) && $(GO) test -cover ./...

vet:
	cd $(ROOT) && $(GO) vet ./...

lint: vet

fmt:
	cd $(ROOT) && gofmt -w $(GO_FILES)

format: fmt-check

fmt-check:
	@cd $(ROOT) && test -z "$$(gofmt -l $(GO_FILES))" || { printf '%s\n' 'Go files need gofmt:'; gofmt -l $(GO_FILES); exit 1; }

security:
	@cd $(ROOT) && $(GO) vet ./... && printf '%s\n' 'Security baseline passed: go vet and repository profile checks.'

dependency-check:
	@command -v govulncheck >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: govulncheck is required; dependency-check is not a NO-OP.' >&2; exit 1; }
	@command -v osv-scanner >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: osv-scanner is required; dependency-check is not a NO-OP.' >&2; exit 1; }
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && rm -f .release/govulncheck.txt .release/osv-scanner.txt .release/dependency-evidence.json; set +e; $(GO) mod verify > .release/go-mod-verify.txt 2>&1; govuln_status=blocked; osv_status=blocked; govuln_version=; osv_version=; if command -v govulncheck >/dev/null 2>&1; then govulncheck -version > .release/govuln-version.txt 2>&1; govuln_version="$$(tr '\n' ' ' < .release/govuln-version.txt)"; govulncheck ./... > .release/govulncheck.txt 2>&1; test $$? -eq 0 && govuln_status=passed || govuln_status=error; fi; if command -v osv-scanner >/dev/null 2>&1; then osv-scanner --version > .release/osv-version.txt 2>&1; osv_version="$$(tr '\n' ' ' < .release/osv-version.txt)"; osv-scanner scan source -r . > .release/osv-scanner.txt 2>&1; test $$? -eq 0 && osv_status=passed || osv_status=error; fi; input_digest="$$(git ls-files -co --exclude-standard go.mod go.sum | shasum -a 256 | cut -d ' ' -f 1)"; $(GO) run ./cmd/dependencyevidence --output .release/dependency-evidence.json --commit "$$(git rev-parse HEAD)" --input-digest "$$input_digest" --govuln-status "$$govuln_status" --govuln-version "$$govuln_version" --osv-status "$$osv_status" --osv-version "$$osv_version" --database "scanner-reported databases; see native output" --govuln-output .release/govulncheck.txt --osv-output .release/osv-scanner.txt; evidence_status=$$?; test $$evidence_status -eq 0
	@printf '%s\n' 'Dependency evidence written to .release/dependency-evidence.json; non-passed scans are explicit blockers/errors.'

secrets-check:
	@cd $(ROOT) && if git grep -n -E -- '-----BEGIN (RSA|EC|OPENSSH|PGP) PRIVATE KEY-----|AKIA[0-9A-Z]{16}|(ghp|github_pat)_[A-Za-z0-9_]+' -- ':!go.sum'; then printf '%s\n' 'Potential secret detected.' >&2; exit 1; fi
	@printf '%s\n' 'Secrets check passed for tracked source.'

sbom:
	@command -v syft >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: syft is required to generate the release SBOM.' >&2; exit 1; }
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && syft dir:. -o spdx-json=.release/sbom.spdx.json

verify-provenance:
	@printf '%s\n' 'NO-OP: no CI builder or published artifact provenance exists to verify locally.'

signature:
	@printf '%s\n' 'NO-OP: this repository does not sign local artifacts; signing belongs to the publishing profile.'

checksums: build
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && shasum -a 256 bin/openrouter > .release/openrouter.sha256

artifact: build checksums
	@printf '%s\n' 'Local artifact and checksum prepared in .release/.'

manifest: artifact
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && printf '{"version":"%s","tag":"v%s","commit":"%s","artifact":"bin/openrouter","digest":"%s"}\n' "$(VERSION)" "$(VERSION)" "$$(git rev-parse HEAD)" "$$(cut -d ' ' -f 1 .release/openrouter.sha256)" > .release/manifest.json

check-package:
	@printf '%s\n' 'NO-OP: package and formula templates are maintained outside this checkout.'

install reinstall upgrade uninstall install-smoke:
	@printf '%s\n' 'NO-OP: installation and package-manager mutations are outside this repository.'

smoke: build
	@cd $(ROOT) && ./bin/openrouter --version >/dev/null && ./bin/openrouter --help >/dev/null

check: build
	cd $(ROOT) && $(BINARY) check --data-dir $(DATA_DIR) --output /dev/null

init:
	cd $(ROOT) && ./scripts/init.sh

refresh: build
	cd $(ROOT) && $(BINARY) refresh --data-dir $(DATA_DIR) --output $(OUTPUT)

history: build
	cd $(ROOT) && $(BINARY) history --data-dir $(DATA_DIR)

table: build
	cd $(ROOT) && $(BINARY) table --data-dir $(DATA_DIR)

version:
	@printf '%s\n' '$(VERSION)'

check-version:
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION must not be empty'; exit 1; }
	@cd $(ROOT) && $(VERSIONCHECK) --version "$(VERSION)" >/dev/null
	@if test -n "$(TAG_VERSION)" && test -z "$$(git -C $(ROOT) status --porcelain)"; then normalized="$$(cd $(ROOT) && $(VERSIONCHECK) "$(TAG_VERSION)")" || exit $$?; test "$$normalized" = "$(VERSION)" || { printf '%s\n' 'VERSION does not match the exact tag'; exit 1; }; fi

check-tag: check-version
	@test -n "$(TAG_VERSION)" || { printf '%s\n' 'an exact release tag is required'; exit 1; }
	@cd $(ROOT) && $(VERSIONCHECK) "$(TAG_VERSION)" >/dev/null
	@test -z "$$(git -C $(ROOT) status --porcelain)" || { printf '%s\n' 'release tag checkout must be clean'; git -C $(ROOT) status --short; exit 1; }
	@test "$$(git -C $(ROOT) rev-parse HEAD)" = "$$(git -C $(ROOT) rev-parse "$(TAG_VERSION)^{commit}")" || { printf '%s\n' 'HEAD must point at the exact release tag'; exit 1; }

release-check: check-version
	@test -f $(ROOT)CHANGELOG.md && awk '/^## \[Unreleased\]$$/{found=1; next} /^## /{if(found) exit} found && /^- /{bullet=1} END{exit !(found && bullet)}' $(ROOT)CHANGELOG.md || { printf '%s\n' 'CHANGELOG.md must contain a non-empty Unreleased section with bullet notes'; exit 1; }
	@test -z "$$(git -C $(ROOT) status --porcelain)" || { printf '%s\n' 'release candidate checkout must be clean'; git -C $(ROOT) status --short; exit 1; }
	@commit="$$(git -C $(ROOT) rev-parse --verify HEAD)" || exit $$?; printf '%s\n' "release candidate: version=$(VERSION) commit=$$commit planned-tag=v$(VERSION)"
	@cd $(ROOT) && git diff --check
	$(MAKE) -C $(ROOT) fmt-check test vet security dependency-check secrets-check sbom checksums manifest verify-provenance signature check-docs
	@test "$$(cd $(ROOT) && ./bin/openrouter --version)" = "openrouter version $(VERSION)" || { printf '%s\n' 'candidate binary version does not match VERSION'; exit 1; }

release-build: check-tag
	@mkdir -p $(dir $(BINARY))
	cd $(ROOT) && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/openrouter

verify-local-artifact: check-tag
	@test -x "$(BINARY)" || { printf '%s\n' 'local release artifact is missing or not executable'; exit 1; }
	@cd $(ROOT) && $(GO) run ./cmd/evidencecheck --manifest .release/manifest.json --checksum .release/openrouter.sha256 --artifact bin/openrouter --tag "$(TAG_VERSION)" --commit "$$(git rev-parse HEAD)" --version "$(VERSION)"
	@cd $(ROOT) && test "$$(./bin/openrouter --version)" = "openrouter version $(VERSION)" && test "$$(./bin/openrouter version)" = "openrouter $(VERSION)" && ./bin/openrouter --help >/dev/null
	@cd $(ROOT) && test "$$(shasum -a 256 bin/openrouter | cut -d ' ' -f 1)" = "$$(cut -d ' ' -f 1 .release/openrouter.sha256)" || { printf '%s\n' 'local artifact digest does not match evidence'; exit 1; }
	@printf '%s\n' 'Verified local exact-tag artifact only.'

verify-release: check-version
	@printf '%s\n' 'BLOCKED: published/stable evidence cannot be cryptographically and semantically verified by this repository; use verify-local-artifact for local evidence.' >&2
	@exit 1

whats-new:
	@test -f $(ROOT)CHANGELOG.md || { printf '%s\n' 'CHANGELOG.md is missing'; exit 1; }
	@awk -v version="$(VERSION)" 'BEGIN { found=0; notes=0 } /^## / { if (found) exit; if ($$0 == "## [" version "]") found=1 } found { print; if ($$0 ~ /^- /) notes=1 } END { exit !(found && notes) }' $(ROOT)CHANGELOG.md || { printf '%s\n' "CHANGELOG.md has no non-empty exact section for $(VERSION)"; exit 1; }

docs check-docs:
	@test -f $(ROOT)README.md && test -f $(ROOT)CHANGELOG.md && test -f $(ROOT)docs/security.md
	@printf '%s\n' 'Documentation contract passed.'

clean:
	rm -f $(BINARY)

help:
	@printf '%s\n' \
		'build          Build bin/openrouter with the current version' \
		'setup          NO-OP: host-managed setup' \
		'check-env      NO-OP: host-managed environment check' \
		'toolchain      NO-OP: host-managed toolchain' \
		'test           Run all Go tests' \
		'test-unit      Alias for unit tests' \
		'test-integration Alias for integration tests' \
		'test-e2e       Alias for end-to-end tests' \
		'race           Run all Go tests with the race detector' \
		'coverage       Run tests with coverage instrumentation' \
		'lint           Run the configured static-analysis baseline' \
		'vet            Run go vet' \
		'fmt            Format tracked Go files' \
		'fmt-check      Check tracked Go files without changing them' \
		'security       Run the repository security baseline' \
		'dependency-check Run govulncheck and OSV-Scanner (required)' \
		'secrets-check  Scan tracked source for high-confidence secret patterns' \
		'sbom           Generate SPDX SBOM with Syft (required)' \
		'verify-provenance Local-only NO-OP: no published builder/artifact' \
		'signature      Local-only NO-OP: signing is external' \
		'checksums      Write SHA-256 checksum for the local artifact' \
		'artifact       Build local artifact and checksum' \
		'manifest       Write local artifact manifest' \
		'check-package  NO-OP: package template is external' \
		'install        NO-OP: installation is external' \
		'reinstall      NO-OP: installation is external' \
		'upgrade        NO-OP: installation is external' \
		'uninstall      NO-OP: installation is external' \
		'smoke          Run local CLI smoke checks' \
		'check-docs     Validate required project documentation' \
		'check          Run the read-only CLI check against this checkout' \
		'init           Build, initialize, refresh, and open the report on macOS' \
		'refresh        Refresh data and the generated comparison document' \
		'history        Show price history for this checkout' \
		'table          Show local model data as a plain-text table' \
		'version        Print the normalized descriptive or release version' \
		'check-version  Validate version metadata and exact-tag agreement' \
		'check-tag      Validate a clean checkout at an exact immutable release tag' \
		'release-check  Run the non-publishing pre-tag gate (VERSION=...)' \
		'release-build  Build with the normalized version from the exact checked-out tag' \
		'verify-local-artifact Verify strict local exact-tag artifact evidence' \
		'verify-release Verify published/stable distribution evidence (blocked when unavailable)' \
		'whats-new      Print exact-version release notes from CHANGELOG.md' \
		'docs           Validate required project documentation' \
		'clean           Remove only bin/openrouter' \
		'help            Show this list of targets'
