GO ?= $(ROOT).builder/local/go-wrapper
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
BINARY := $(ROOT)bin/openrouter
TARGET ?= ./cmd/openrouter
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
INSTALL_PATH := $(BINDIR)/openrouter
ALIAS_PATH := $(BINDIR)/omt
DATA_DIR := $(ROOT)
OUTPUT := $(ROOT)docs/openrouter-model-comparison.md
EVIDENCE_DIR := $(ROOT).release
GO_FILES := $(addprefix $(ROOT),$(shell git -C $(ROOT) ls-files -co --exclude-standard '*.go' | while IFS= read -r file; do test -f "$(ROOT)$$file" && printf '%s\n' "$$file"; done))

DESCRIBE_VERSION := $(shell git -C $(ROOT) describe --tags --always --dirty)
TAG_VERSION := $(shell git -C $(ROOT) describe --tags --exact-match 2>/dev/null)
TAG_IS_CLEAN := $(shell test -z "$(shell git -C $(ROOT) status --porcelain)" && printf 'yes')
VERSION ?= $(if $(TAG_VERSION),$(patsubst v%,%,$(TAG_VERSION)),0.0.0-dev)
VERSIONCHECK := $(GO) run ./cmd/versioncheck
PUBLISHED_EVIDENCE ?= $(EVIDENCE_DIR)/published-evidence.json
FORMULA_TAG ?=
HOMEBREW_VERSION := $(patsubst v%,%,$(if $(FORMULA_TAG),$(FORMULA_TAG),$(TAG_VERSION)))
LOCAL_RELEASE_DIR ?= $(ROOT)dist/local-release
LOCAL_RELEASE_PLATFORMS ?= darwin/arm64 darwin/amd64 linux/amd64 linux/arm64
LOCAL_RELEASE_BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Signed release provenance (static cosign key-pair, no Fulcio/Rekor — see
# ~/projects/tools/guide-tools/.task/go-guide-compliance/signing-design.md
# §5.5). Evidence file paths are relative-to-root literals (not built from
# EVIDENCE_DIR, which is an absolute path used only for `mkdir -p`) so they
# match what gets written into release-manifest.json / published-evidence.json
# and stay resolvable after `cd $(ROOT)`.
COSIGN_PUBLIC_KEY := cosign.pub
COSIGN_PRIVATE_KEY_REF ?= env://COSIGN_PRIVATE_KEY

# Release-signing key material lives in the macOS login Keychain, not in CI.
# Retrieve by hand with:
#   security find-generic-password -s cosign.openrouter-model-tracker.private-key -a mickle.grok -w | openssl base64 -d -A
#   security find-generic-password -s cosign.openrouter-model-tracker.key-password  -a mickle.grok -w
COSIGN_KEYCHAIN_KEY_SERVICE      ?= cosign.openrouter-model-tracker.private-key
COSIGN_KEYCHAIN_PASSWORD_SERVICE ?= cosign.openrouter-model-tracker.key-password
COSIGN_KEYCHAIN_ACCOUNT          ?= mickle.grok

cosign-key-check:
	@set -eu; \
	key="$$(security find-generic-password -s '$(COSIGN_KEYCHAIN_KEY_SERVICE)' -a '$(COSIGN_KEYCHAIN_ACCOUNT)' -w 2>/dev/null | openssl base64 -d -A)" || { \
	  printf '%s\n' 'FAIL: cosign private key not found in the login Keychain' >&2; \
	  printf '%s\n' 'HINT: security find-generic-password -s $(COSIGN_KEYCHAIN_KEY_SERVICE) -a $(COSIGN_KEYCHAIN_ACCOUNT) -w | openssl base64 -d -A' >&2; exit 1; }; \
	pw="$$(security find-generic-password -s '$(COSIGN_KEYCHAIN_PASSWORD_SERVICE)' -a '$(COSIGN_KEYCHAIN_ACCOUNT)' -w 2>/dev/null)" || { \
	  printf '%s\n' 'FAIL: cosign key password not found in the login Keychain' >&2; exit 1; }; \
	tmp="$$(mktemp -d)"; trap 'rm -rf "$$tmp"' EXIT HUP INT TERM; \
	printf 'cosign-key-check\n' > "$$tmp/probe.txt"; \
	COSIGN_PASSWORD="$$pw" COSIGN_PRIVATE_KEY="$$key" cosign sign-blob --key env://COSIGN_PRIVATE_KEY \
	  --yes --tlog-upload=false --use-signing-config=false --bundle "$$tmp/probe.sig.json" "$$tmp/probe.txt" >/dev/null 2>&1 \
	  || { printf '%s\n' 'FAIL: the stored key could not sign (wrong password, or corrupted PEM)' >&2; exit 1; }; \
	cosign verify-blob --key '$(COSIGN_PUBLIC_KEY)' --bundle "$$tmp/probe.sig.json" \
	  --insecure-ignore-tlog=true "$$tmp/probe.txt" >/dev/null 2>&1 \
	  || { printf '%s\n' 'FAIL: the stored key is NOT the private half of $(COSIGN_PUBLIC_KEY)' >&2; exit 1; }
	@printf '%s\n' 'PASS: cosign-key-check (Keychain key is the private half of $(COSIGN_PUBLIC_KEY))'

cosign-sign-release:
	@set -eu; \
	key="$$(security find-generic-password -s '$(COSIGN_KEYCHAIN_KEY_SERVICE)' -a '$(COSIGN_KEYCHAIN_ACCOUNT)' -w | openssl base64 -d -A)"; \
	pw="$$(security find-generic-password -s '$(COSIGN_KEYCHAIN_PASSWORD_SERVICE)' -a '$(COSIGN_KEYCHAIN_ACCOUNT)' -w)"; \
	COSIGN_PRIVATE_KEY="$$key" COSIGN_PASSWORD="$$pw" $(MAKE) --no-print-directory sign attest

RELEASE_MANIFEST := .release/release-manifest.json
RELEASE_MANIFEST_SIG := .release/release-manifest.json.sig.bundle.json
RELEASE_MANIFEST_ATT := .release/release-manifest.json.att.bundle.json
PROVENANCE_PREDICATE := .release/provenance-predicate.json
SBOM_FILE := .release/sbom.spdx.json
# local/candidate: applicability no-op. external/published: real cosign verification.
PROVENANCE_PROFILE ?= local
GITHUB_REPOSITORY ?= MikcleGrok/openrouter-model-tracker
GITHUB_RUN_ID ?= local
RELEASE_SOURCE_DIR ?= $(ROOT)
RELEASE_ARTIFACT_DIR ?= $(RELEASE_SOURCE_DIR)/dist/local-release/$(VERSION)

VALID_PROVENANCE_PROFILES := local candidate external published
ifneq ($(filter $(PROVENANCE_PROFILE),$(VALID_PROVENANCE_PROFILES)),$(PROVENANCE_PROFILE))
$(error BLOCKED: unknown PROVENANCE_PROFILE '$(PROVENANCE_PROFILE)' (expected local|candidate|external|published))
endif

.DEFAULT_GOAL := help

.PHONY: setup check-env toolchain build test test-unit test-acceptance test-all race coverage lint vet fmt format fmt-check security dependency-check secrets-check install-hooks sign-flags-check provenance-profile-check openrouter-launchd-refresh-check openrouter-launchd-refresh-install openrouter-launchd-refresh-uninstall openrouter-launchd-refresh-status openrouter-launchd-refresh-start sbom release-manifest provenance-predicate cosign-key-check cosign-sign-release sign attest verify-provenance signature checksums artifact manifest check-package check-install-paths install reinstall upgrade uninstall verify-install install-smoke smoke check init refresh history table version check-version check-tag check-homebrew-formula sync-homebrew-formula homebrew-reinstall release-check release-build verify-local-artifact verify-release release-local local-release release-github-check release-github docs check-docs clean help FORCE

build: $(BINARY)

$(BINARY): FORCE $(ROOT)Makefile $(GO_FILES) $(ROOT)go.mod $(ROOT)go.sum
	@mkdir -p $(dir $@)
	cd $(ROOT) && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $@ $(TARGET)

FORCE:

setup check-env toolchain:
	@printf '%s\n' 'NO-OP: setup and toolchain provisioning are managed by the host environment.'

test:
	$(MAKE) -C $(ROOT) test-all

test-unit:
	cd $(ROOT) && $(GO) test -count=1 ./internal/... ./cmd/...

test-acceptance: build
	cd $(ROOT) && OPENROUTER_EXPECTED_VERSION="$(VERSION)" $(GO) test -count=1 ./tests/...

test-all: test-unit test-acceptance sign-flags-check provenance-profile-check

race:
	cd $(ROOT) && OPENROUTER_EXPECTED_VERSION="$(VERSION)" $(GO) test -race -count=1 ./...

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
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && rm -f .release/govulncheck.txt .release/osv-scanner.txt .release/govuln-version.txt .release/osv-version.txt .release/dependency-evidence.json; : > .release/govulncheck.txt; : > .release/osv-scanner.txt; set +e; toolchain="$$($(GO) --print-toolchain)"; printf '%s\n' "Go toolchain: $$toolchain"; $(GO) mod verify > .release/go-mod-verify.txt 2>&1; mod_exit=$$?; mod_status=passed; test $$mod_exit -eq 0 || mod_status=error; govuln_status=blocked; osv_status=blocked; govuln_version=; osv_version=; if command -v govulncheck >/dev/null 2>&1; then GOTOOLCHAIN="$$toolchain" govulncheck -version > .release/govuln-version.txt 2>&1; govuln_version="$$(tr '\n' ' ' < .release/govuln-version.txt)"; GOTOOLCHAIN="$$toolchain" govulncheck ./... > .release/govulncheck.txt 2>&1; test $$? -eq 0 && govuln_status=passed || govuln_status=error; fi; if command -v osv-scanner >/dev/null 2>&1; then osv-scanner --version > .release/osv-version.txt 2>&1; osv_version="$$(tr '\n' ' ' < .release/osv-version.txt)"; GOTOOLCHAIN="$$toolchain" osv-scanner scan source --lockfile go.mod > .release/osv-scanner.txt 2>&1; test $$? -eq 0 && osv_status=passed || osv_status=error; fi; shasum -a 256 go.mod go.sum > .release/module-checksums.txt || exit $$?; input_digest="$$(shasum -a 256 .release/module-checksums.txt)" || exit $$?; input_digest="$${input_digest%% *}"; rm -f .release/module-checksums.txt; $(GO) run ./cmd/dependencyevidence --output .release/dependency-evidence.json --commit "$$(git rev-parse HEAD)" --input-digest "$$input_digest" --mod-status "$$mod_status" --govuln-status "$$govuln_status" --govuln-version "$$govuln_version" --osv-status "$$osv_status" --osv-version "$$osv_version" --database "scanner-reported databases; see native output" --govuln-output .release/govulncheck.txt --osv-output .release/osv-scanner.txt; evidence_status=$$?; test $$evidence_status -eq 0
	@printf '%s\n' 'Dependency evidence written to .release/dependency-evidence.json; non-passed scans are explicit blockers/errors.'

secrets-check:
	@cd $(ROOT) && if git grep -n -E -- '-----BEGIN (RSA |EC |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY-----|AKIA[0-9A-Z]{16}|(ghp|github_pat)_[A-Za-z0-9_]+' -- ':!go.sum'; then printf '%s\n' 'Potential secret detected.' >&2; exit 1; fi
	@printf '%s\n' 'Secrets check passed for tracked source.'

install-hooks:
	@cd $(ROOT) && git config core.hooksPath .githooks
	@printf '%s\n' 'Installed: git hooks now run from .githooks (pre-commit secrets-check enabled).'

sign-flags-check:
	@$(ROOT)scripts/sign_flags_test.sh

provenance-profile-check:
	@$(ROOT)scripts/provenance_profile_test.sh

openrouter-launchd-refresh-check:
	@$(ROOT)scripts/launchd-refresh_test.sh

openrouter-launchd-refresh-install:
	@$(ROOT)scripts/launchd-refresh.sh install

openrouter-launchd-refresh-uninstall:
	@$(ROOT)scripts/launchd-refresh.sh uninstall

openrouter-launchd-refresh-status:
	@$(ROOT)scripts/launchd-refresh.sh status

openrouter-launchd-refresh-start:
	@$(ROOT)scripts/launchd-refresh.sh start

sbom:
	@command -v syft >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: syft is required to generate the release SBOM.' >&2; exit 1; }
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && syft dir:. -o spdx-json=.release/sbom.spdx.json

release-manifest: check-tag artifact sbom
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && sbom_digest="$$(openssl dgst -sha256 -r $(SBOM_FILE))" || exit $$?; sbom_digest="$${sbom_digest%% *}"; artifact_digest="$$(cat .release/openrouter.sha256)" || exit $$?; artifact_digest="$${artifact_digest%% *}"; printf '{"schema":"guide-tools/release-manifest-v1","project":"openrouter-model-tracker","version":"%s","tag":"%s","commit":"%s","built_at":"%s","artifacts":[{"name":"openrouter","path":"bin/openrouter","sha256":"%s"}],"sbom":{"path":"%s","sha256":"%s"},"builder":{"platform":"github-actions","repository":"%s","workflow":"release.yml","run_id":"%s"}}\n' \
		'$(VERSION)' '$(TAG_VERSION)' "$$(git rev-parse HEAD)" "$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		"$$artifact_digest" \
		'$(SBOM_FILE)' "$$sbom_digest" \
		'$(GITHUB_REPOSITORY)' '$(GITHUB_RUN_ID)' \
		> $(RELEASE_MANIFEST)
	@test -s $(RELEASE_MANIFEST)

provenance-predicate: check-tag
	@mkdir -p $(EVIDENCE_DIR)
	@cd $(ROOT) && printf '{"buildDefinition":{"buildType":"https://guide-tools.local/cosign-static-key/v1","externalParameters":{"tag":"%s","commit":"%s"}},"runDetails":{"builder":{"id":"self-reported:github-actions:%s:release.yml:%s"},"metadata":{"invocationId":"%s"}}}\n' \
		'$(TAG_VERSION)' "$$(git rev-parse HEAD)" '$(GITHUB_REPOSITORY)' '$(GITHUB_RUN_ID)' '$(GITHUB_RUN_ID)' \
		> $(PROVENANCE_PREDICATE)
	@test -s $(PROVENANCE_PREDICATE)

ifeq ($(filter local candidate,$(PROVENANCE_PROFILE)),)
sign: release-manifest
	@command -v cosign >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: cosign is required to sign the release manifest'; exit 1; }
	@test -n "$(COSIGN_PRIVATE_KEY)" || { printf '%s\n' 'BLOCKED: COSIGN_PRIVATE_KEY is required for the external signing profile'; exit 1; }
	@test -s $(COSIGN_PUBLIC_KEY) || { printf '%s\n' 'BLOCKED: $(COSIGN_PUBLIC_KEY) is missing; cannot sign without the committed key pair'; exit 1; }
	cd $(ROOT) && cosign sign-blob --key '$(COSIGN_PRIVATE_KEY_REF)' --yes --tlog-upload=false --use-signing-config=false --bundle $(RELEASE_MANIFEST_SIG) $(RELEASE_MANIFEST)
	@test -s $(RELEASE_MANIFEST_SIG)
	@grep -q '"tlogEntries"' $(RELEASE_MANIFEST_SIG) && { printf '%s\n' 'BLOCKED: signature bundle contains a transparency log entry; refusing to publish'; exit 1; } || true
else
sign:
	@printf '%s\n' 'NOT APPLICABLE: cosign signing is disabled for PROVENANCE_PROFILE=$(PROVENANCE_PROFILE); no signed evidence created.'
endif

ifeq ($(filter local candidate,$(PROVENANCE_PROFILE)),)
attest: provenance-predicate
	@command -v cosign >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: cosign is required to attest the release manifest'; exit 1; }
	@test -n "$(COSIGN_PRIVATE_KEY)" || { printf '%s\n' 'BLOCKED: COSIGN_PRIVATE_KEY is required for the external signing profile'; exit 1; }
	@test -s $(RELEASE_MANIFEST) || { printf '%s\n' 'BLOCKED: $(RELEASE_MANIFEST) is missing; run make release-manifest (or make sign) first -- attest MUST NOT regenerate it, or it would attest different content than sign signed'; exit 1; }
	cd $(ROOT) && cosign attest-blob --predicate $(PROVENANCE_PREDICATE) --type slsaprovenance1 --key '$(COSIGN_PRIVATE_KEY_REF)' --yes --tlog-upload=false --use-signing-config=false --bundle $(RELEASE_MANIFEST_ATT) $(RELEASE_MANIFEST)
	@test -s $(RELEASE_MANIFEST_ATT)
	@grep -q '"tlogEntries"' $(RELEASE_MANIFEST_ATT) && { printf '%s\n' 'BLOCKED: attestation bundle contains a transparency log entry; refusing to publish'; exit 1; } || true
else
attest:
	@printf '%s\n' 'NOT APPLICABLE: cosign attestation is disabled for PROVENANCE_PROFILE=$(PROVENANCE_PROFILE); no provenance evidence created.'
endif

signature:
	@cd $(ROOT) && PROVENANCE_PROFILE='$(PROVENANCE_PROFILE)' TAG_VERSION='$(TAG_VERSION)' VERSION='$(VERSION)' COSIGN_PRIVATE_KEY='$(COSIGN_PRIVATE_KEY)' COSIGN_PUBLIC_KEY='$(COSIGN_PUBLIC_KEY)' RELEASE_MANIFEST='$(RELEASE_MANIFEST)' RELEASE_MANIFEST_SIG='$(RELEASE_MANIFEST_SIG)' ./scripts/verify-provenance.sh signature

verify-provenance: signature
	@cd $(ROOT) && PROVENANCE_PROFILE='$(PROVENANCE_PROFILE)' TAG_VERSION='$(TAG_VERSION)' VERSION='$(VERSION)' COSIGN_PRIVATE_KEY='$(COSIGN_PRIVATE_KEY)' COSIGN_PUBLIC_KEY='$(COSIGN_PUBLIC_KEY)' RELEASE_MANIFEST='$(RELEASE_MANIFEST)' RELEASE_MANIFEST_ATT='$(RELEASE_MANIFEST_ATT)' BIN='bin/openrouter' SBOM_FILE='$(SBOM_FILE)' GITHUB_REPOSITORY='$(GITHUB_REPOSITORY)' PUBLISHED_EVIDENCE='$(PUBLISHED_EVIDENCE)' ./scripts/verify-provenance.sh full
ifneq ($(filter external published,$(PROVENANCE_PROFILE)),)
	@cd $(ROOT) && $(GO) run ./cmd/evidencecheck --published-evidence '$(PUBLISHED_EVIDENCE)' --tag '$(TAG_VERSION)' --commit "$$(git rev-parse HEAD)" --version '$(VERSION)'
endif

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

install reinstall upgrade: check-install-paths check-version
	@set -eu; temp_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/openrouter-install-build.XXXXXX")"; trap 'rm -rf "$$temp_dir"' EXIT HUP INT TERM; temp_binary="$$temp_dir/openrouter"; cd "$(ROOT)" && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o "$$temp_binary" "$(TARGET)"; "$(ROOT)scripts/install.sh" install "$$temp_binary" "$(INSTALL_PATH)" "$(VERSION)" "$(ALIAS_PATH)"

check-install-paths:
	@test -n "$(PREFIX)" || { printf '%s\n' 'PREFIX must not be empty'; exit 2; }
	@case "$(PREFIX)" in /*) ;; *) printf '%s\n' 'PREFIX must be an absolute path: $(PREFIX)' >&2; exit 2 ;; esac
	@test -n "$(BINDIR)" || { printf '%s\n' 'BINDIR must not be empty'; exit 2; }
	@case "$(BINDIR)" in /*) ;; *) printf '%s\n' 'BINDIR must be an absolute path: $(BINDIR)' >&2; exit 2 ;; esac

uninstall: check-install-paths
	@$(ROOT)scripts/install.sh uninstall "$(INSTALL_PATH)" "$(ALIAS_PATH)"

verify-install: install
	@set -eu; test -L "$(ALIAS_PATH)"; test "$$(readlink "$(ALIAS_PATH)")" = "$(INSTALL_PATH)"; for installed in "$(INSTALL_PATH)" "$(ALIAS_PATH)"; do test -x "$$installed"; actual="$$("$$installed" --version)"; test "$$actual" = "openrouter version $(VERSION)"; actual="$$("$$installed" version)"; test "$$actual" = "openrouter $(VERSION)"; "$$installed" --help >/dev/null; done; printf '%s\n' 'Verified installed CLI pair: $(INSTALL_PATH), $(ALIAS_PATH) (VERSION=$(VERSION))'

install-smoke:
	@GO="$(GO)" $(ROOT)scripts/install_test.sh

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
	@if test -n "$(TAG_VERSION)" && test "$(TAG_IS_CLEAN)" != yes && test "$(VERSION)" = "$(patsubst v%,%,$(TAG_VERSION))"; then printf '%s\n' 'exact-tag VERSION is forbidden on a dirty checkout'; git -C $(ROOT) status --short; exit 1; fi
	@if test -n "$(TAG_VERSION)" && test -z "$$(git -C $(ROOT) status --porcelain)"; then normalized="$$(cd $(ROOT) && $(VERSIONCHECK) "$(TAG_VERSION)")" || exit $$?; test "$$normalized" = "$(VERSION)" || { printf '%s\n' 'VERSION does not match the exact tag'; exit 1; }; fi

check-tag: check-version
	@test -n "$(TAG_VERSION)" || { printf '%s\n' 'an exact release tag is required'; exit 1; }
	@cd $(ROOT) && $(VERSIONCHECK) "$(TAG_VERSION)" >/dev/null
	@test -z "$$(git -C $(ROOT) status --porcelain)" || { printf '%s\n' 'release tag checkout must be clean'; git -C $(ROOT) status --short; exit 1; }
	@test "$$(git -C $(ROOT) rev-parse HEAD)" = "$$(git -C $(ROOT) rev-parse "$(TAG_VERSION)^{commit}")" || { printf '%s\n' 'HEAD must point at the exact release tag'; exit 1; }

check-homebrew-formula:
	@cd $(ROOT) && ./scripts/verify-distribution.sh --tag "$(FORMULA_TAG)"

sync-homebrew-formula:
	@cd $(ROOT) && ./scripts/sync-homebrew-formula.sh $(FORMULA_TAG)

homebrew-reinstall: sync-homebrew-formula
	@test -f "$(shell brew --repository)/Library/Taps/local/homebrew-tap/Formula/openrouter.rb" || { printf '%s\n' 'Local Homebrew formula is missing'; exit 1; }
	brew reinstall --formula --build-from-source "$(shell brew --repository)/Library/Taps/local/homebrew-tap/Formula/openrouter.rb"
	@cd $(ROOT) && ./scripts/sync-homebrew-formula.sh --check
	@test -n "$(HOMEBREW_VERSION)" || { printf '%s\n' 'An exact formula tag is required for installed-version verification'; exit 1; }
	@test "$$(brew list --versions openrouter)" = "openrouter $(HOMEBREW_VERSION)" || { printf '%s\n' 'Installed Homebrew version does not match the formula tag'; exit 1; }
	@test "$$(openrouter --version)" = "openrouter version $(HOMEBREW_VERSION)" || { printf '%s\n' 'Installed CLI version does not match the formula tag'; exit 1; }
	brew test local/tap/openrouter

release-check: check-version build
	@test -f $(ROOT)CHANGELOG.md && awk '/^## \[Unreleased\]$$/{found=1; next} /^## /{if(found) exit} found && /^- /{bullet=1} END{exit !(found && bullet)}' $(ROOT)CHANGELOG.md || { printf '%s\n' 'CHANGELOG.md must contain a non-empty Unreleased section with bullet notes'; exit 1; }
	@test -z "$$(git -C $(ROOT) status --porcelain)" || { printf '%s\n' 'release candidate checkout must be clean'; git -C $(ROOT) status --short; exit 1; }
	@commit="$$(git -C $(ROOT) rev-parse --verify HEAD)" || exit $$?; printf '%s\n' "release candidate: version=$(VERSION) commit=$$commit planned-tag=v$(VERSION)"
	@cd $(ROOT) && git diff --check
	$(MAKE) -C $(ROOT) fmt-check test vet security dependency-check secrets-check sbom verify-provenance signature check-docs PROVENANCE_PROFILE=candidate
	@test "$$(cd $(ROOT) && ./bin/openrouter --version)" = "openrouter version $(VERSION)" || { printf '%s\n' 'candidate binary version does not match VERSION'; exit 1; }

release-build: check-tag check-homebrew-formula
	@mkdir -p $(dir $(BINARY))
	cd $(ROOT) && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/openrouter

verify-local-artifact: check-tag check-homebrew-formula
	@test -x "$(BINARY)" || { printf '%s\n' 'local release artifact is missing or not executable'; exit 1; }
	@cd $(ROOT) && $(GO) run ./cmd/evidencecheck --manifest .release/manifest.json --checksum .release/openrouter.sha256 --artifact bin/openrouter --tag "$(TAG_VERSION)" --commit "$$(git rev-parse HEAD)" --version "$(VERSION)"
	@cd $(ROOT) && test "$$(./bin/openrouter --version)" = "openrouter version $(VERSION)" && test "$$(./bin/openrouter version)" = "openrouter $(VERSION)" && ./bin/openrouter --help >/dev/null
	@cd $(ROOT) && artifact_digest="$$(shasum -a 256 bin/openrouter)" || exit $$?; artifact_digest="$${artifact_digest%% *}"; evidence_digest="$$(cat .release/openrouter.sha256)" || exit $$?; evidence_digest="$${evidence_digest%% *}"; test "$$artifact_digest" = "$$evidence_digest" || { printf '%s\n' 'local artifact digest does not match evidence'; exit 1; }
	@printf '%s\n' 'Verified local exact-tag artifact only.'

verify-release: check-tag
	@cd $(ROOT) && ./scripts/verify-distribution.sh --tag "$(TAG_VERSION)" --version "$(VERSION)" --installed-package openrouter --installed-version "$(VERSION)" --brew-test local/tap/openrouter
	@cd $(ROOT) && test "$$(openrouter --version)" = "openrouter version $(VERSION)" || { printf '%s\n' 'Installed CLI --version does not match VERSION'; exit 1; }
	@cd $(ROOT) && test "$$(openrouter version)" = "openrouter $(VERSION)" || { printf '%s\n' 'Installed CLI version does not match VERSION'; exit 1; }
	@printf '%s\n' 'Verified local stable Homebrew channel only; no GitHub publication or provenance claim made.'

whats-new:
	@test -f $(ROOT)CHANGELOG.md || { printf '%s\n' 'CHANGELOG.md is missing'; exit 1; }
	@awk -v version="$(VERSION)" 'BEGIN { found=0; notes=0 } /^## / { if (found) exit; if ($$0 == "## [" version "]") found=1 } found { print; if ($$0 ~ /^- /) notes=1 } END { exit !(found && notes) }' $(ROOT)CHANGELOG.md || { printf '%s\n' "CHANGELOG.md has no non-empty exact section for $(VERSION)"; exit 1; }

release-local local-release: check-tag fmt-check test-all vet security secrets-check check-docs
	@set -eu; \
		version='$(VERSION)'; tag='$(TAG_VERSION)'; commit="$$(git -C '$(ROOT)' rev-parse HEAD)"; out='$(LOCAL_RELEASE_DIR)/'"$$version"; \
		rm -rf "$$out"; mkdir -p "$$out/artifacts"; artifacts_json=''; first=1; \
		for platform in $(LOCAL_RELEASE_PLATFORMS); do \
		os="$${platform%/*}"; arch="$${platform#*/}"; name="openrouter-$$version-$$os-$$arch"; \
		GOOS="$$os" GOARCH="$$arch" CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-X main.version=$$version" -o "$$out/$$name" '$(ROOT)cmd/openrouter'; \
		chmod 0755 "$$out/$$name"; \
		touch -t 197001010000 "$$out/$$name"; \
		package="$$out/package-$$name"; mkdir -p "$$package/scripts"; \
		mv "$$out/$$name" "$$package/$$name"; \
		cp '$(ROOT)scripts/cron-refresh.sh' '$(ROOT)scripts/launchd-refresh.sh' "$$package/scripts/"; \
		chmod 0755 "$$package/$$name" "$$package/scripts/cron-refresh.sh" "$$package/scripts/launchd-refresh.sh"; \
		tar -C "$$package" --format=ustar --mtime='1970-01-01 00:00:00 UTC' --owner=0 --group=0 --numeric-owner -czf "$$out/artifacts/$$name.tar.gz" "$$name" scripts/cron-refresh.sh scripts/launchd-refresh.sh; \
		tar -tzf "$$out/artifacts/$$name.tar.gz" | grep -Fqx "$$name"; \
		tar -tzf "$$out/artifacts/$$name.tar.gz" | grep -Fqx 'scripts/cron-refresh.sh'; \
		tar -tzf "$$out/artifacts/$$name.tar.gz" | grep -Fqx 'scripts/launchd-refresh.sh'; \
		! tar -tzf "$$out/artifacts/$$name.tar.gz" | grep -Fq 'launchd-refresh_test.sh'; \
		test "$$(tar -tvzf "$$out/artifacts/$$name.tar.gz" | awk '$$NF == "scripts/launchd-refresh.sh" {print $$1}')" = '-rwxr-xr-x'; \
		digest="$$(shasum -a 256 "$$out/artifacts/$$name.tar.gz")" || exit $$?; digest="$${digest%% *}"; \
		printf '%s  %s\n' "$$digest" "artifacts/$$name.tar.gz" >> "$$out/SHA256SUMS"; \
		if test "$$first" -eq 0; then artifacts_json="$$artifacts_json,"; fi; first=0; \
		artifacts_json="$$artifacts_json{\"artifact\":\"artifacts/$$name.tar.gz\",\"sha256\":\"$$digest\"}"; \
		rm -rf "$$package"; \
	done; \
		awk -v version="$$version" 'BEGIN { found=0; notes=0 } /^## / { if (found) exit; if ($$0 == "## [" version "]") found=1 } found { print; if ($$0 ~ /^- /) notes=1 } END { exit !(found && notes) }' '$(ROOT)CHANGELOG.md' > "$$out/RELEASE_NOTES.md"; \
	printf '{"schema":"openrouter-model-tracker/local-release-v1","version":"%s","tag":"%s","commit":"%s","built_at":"%s","artifacts":[%s]}\n' "$$version" "$$tag" "$$commit" '$(LOCAL_RELEASE_BUILT_AT)' "$$artifacts_json" > "$$out/manifest.json"; \
	printf '%s\n' "Local release written to $$out"

release-github-check:
	@set -eu; \
		command -v jq >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: jq is required for GitHub release evidence validation' >&2; exit 1; }; \
		source_dir="$$(cd '$(RELEASE_SOURCE_DIR)' 2>/dev/null && pwd -P)" || { printf '%s\n' 'BLOCKED: RELEASE_SOURCE_DIR is not an accessible directory: $(RELEASE_SOURCE_DIR)' >&2; exit 1; }; \
		artifact_dir="$$(cd '$(RELEASE_ARTIFACT_DIR)' 2>/dev/null && pwd -P)" || { printf '%s\n' 'BLOCKED: RELEASE_ARTIFACT_DIR is not an accessible directory: $(RELEASE_ARTIFACT_DIR)' >&2; exit 1; }; \
		tag='$(TAG_VERSION)'; version='$(VERSION)'; test "$$tag" = "v$$version" && printf '%s\n' "$$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { printf '%s\n' 'BLOCKED: TAG_VERSION must be vX.Y.Z and match VERSION' >&2; exit 1; }; \
		test -z "$$(git -C "$$source_dir" status --porcelain)" || { printf '%s\n' 'BLOCKED: RELEASE_SOURCE_DIR must be clean' >&2; git -C "$$source_dir" status --short >&2; exit 1; }; \
		commit="$$(git -C "$$source_dir" rev-parse --verify HEAD)" || { printf '%s\n' 'BLOCKED: cannot resolve source checkout HEAD' >&2; exit 1; }; local_tag="$$(git -C "$$source_dir" rev-parse --verify "$$tag^{commit}")" || { printf '%s\n' "BLOCKED: exact tag $$tag cannot be resolved locally" >&2; exit 1; }; test "$$commit" = "$$local_tag" || { printf '%s\n' "BLOCKED: source checkout is not at exact tag $$tag" >&2; exit 1; }; \
		remote_tmp="$$(mktemp -d)"; trap 'rm -rf "$$remote_tmp"' EXIT; set +e; git -C "$$source_dir" ls-remote origin "refs/tags/$$tag" "refs/tags/$$tag^{}" >"$$remote_tmp/record" 2>"$$remote_tmp/error"; remote_status=$$?; set -e; test $$remote_status -eq 0 -o $$remote_status -eq 2 || { printf '%s\n' "BLOCKED: origin tag lookup failed (network/API error, exit $$remote_status)" >&2; cat "$$remote_tmp/error" >&2; exit 1; }; \
		direct="$$(awk -v ref="refs/tags/$$tag" '$$2 == ref { print $$1 }' "$$remote_tmp/record")"; peeled="$$(awk -v ref="refs/tags/$$tag^{}" '$$2 == ref { print $$1 }' "$$remote_tmp/record")"; test -n "$$direct" -o -n "$$peeled" || { printf '%s\n' "BLOCKED: exact tag $$tag is missing on origin" >&2; exit 1; }; remote_commit="$$peeled"; test -n "$$remote_commit" || remote_commit="$$direct"; test "$$remote_commit" = "$$commit" || { printf '%s\n' "BLOCKED: origin tag $$tag does not match source commit $$commit" >&2; exit 1; }; \
		manifest="$$source_dir/.release/release-manifest.json"; signature="$$source_dir/.release/release-manifest.json.sig.bundle.json"; attestation="$$source_dir/.release/release-manifest.json.att.bundle.json"; sbom="$$source_dir/.release/sbom.spdx.json"; dependency_evidence="$$source_dir/.release/dependency-evidence.json"; published_evidence="$$source_dir/.release/published-evidence.json"; \
		test -s "$$manifest" || { printf '%s\n' "BLOCKED: release manifest is missing or empty: $$manifest" >&2; exit 1; }; test -s "$$signature" || { printf '%s\n' "BLOCKED: GitHub release requires signed provenance; signature bundle is missing: $$signature" >&2; printf '%s\n' "DETAIL: set COSIGN_PRIVATE_KEY securely, then run PROVENANCE_PROFILE=published TAG_VERSION=$$tag VERSION=$$version make -C $$source_dir sign attest verify-provenance" >&2; exit 1; }; test -s "$$attestation" || { printf '%s\n' "BLOCKED: GitHub release requires signed provenance; attestation bundle is missing: $$attestation" >&2; printf '%s\n' "DETAIL: set COSIGN_PRIVATE_KEY securely, then run PROVENANCE_PROFILE=published TAG_VERSION=$$tag VERSION=$$version make -C $$source_dir attest verify-provenance" >&2; exit 1; }; for evidence in "$$sbom" "$$dependency_evidence"; do test -s "$$evidence" || { printf '%s\n' "BLOCKED: required release evidence is missing or empty: $$evidence" >&2; exit 1; }; done; \
		jq -e --arg version "$$version" --arg tag "$$tag" --arg commit "$$commit" '(.version == $$version) and (.tag == $$tag) and (.commit == $$commit) and (.artifacts | length == 1) and (.artifacts[0].path == "bin/openrouter")' "$$manifest" >/dev/null || { printf '%s\n' 'BLOCKED: signed release manifest identity does not match exact tag checkout' >&2; exit 1; }; \
		cd "$$artifact_dir"; test -s RELEASE_NOTES.md && test -s manifest.json && test -s SHA256SUMS || { printf '%s\n' 'BLOCKED: local release notes, manifest, or checksums are missing' >&2; exit 1; }; \
		jq -e --arg version "$$version" --arg tag "$$tag" --arg commit "$$commit" '(.version == $$version) and (.tag == $$tag) and (.commit == $$commit) and (.artifacts | length > 0) and ([.artifacts[].artifact] | all(test("^artifacts/[A-Za-z0-9._-]+\\.tar\\.gz$$"))) and ([.artifacts[].artifact] | length == (unique | length)) and ([.artifacts[].sha256] | length == (.artifacts | length)) and ([.artifacts[].sha256] | all(test("^[0-9a-f]{64}$$")))' manifest.json >/dev/null || { printf '%s\n' 'BLOCKED: local release manifest has invalid identity, archive paths, or digests' >&2; exit 1; }; \
		tmp_dir="$$(mktemp -d)"; trap 'rm -rf "$$remote_tmp" "$$tmp_dir"' EXIT; jq -r '.artifacts[] | "\(.sha256)  \(.artifact)"' manifest.json | sort > "$$tmp_dir/manifest"; sort SHA256SUMS > "$$tmp_dir/checksums"; cmp -s "$$tmp_dir/manifest" "$$tmp_dir/checksums" || { printf '%s\n' 'BLOCKED: SHA256SUMS differs from release manifest' >&2; exit 1; }; : > "$$tmp_dir/actual"; set -- artifacts/*.tar.gz; test -f "$$1" || { printf '%s\n' 'BLOCKED: no local release archives found' >&2; exit 1; }; for archive; do test -s "$$archive" || { printf '%s\n' "BLOCKED: release archive is missing or empty: $$archive" >&2; exit 1; }; shasum -a 256 "$$archive" >> "$$tmp_dir/actual"; done; sort "$$tmp_dir/actual" > "$$tmp_dir/actual.sorted"; cmp -s "$$tmp_dir/manifest" "$$tmp_dir/actual.sorted" || { printf '%s\n' 'BLOCKED: local archive set or digest differs from release manifest' >&2; exit 1; }; shasum -a 256 -c SHA256SUMS >/dev/null || { printf '%s\n' 'BLOCKED: local release archive checksum verification failed' >&2; exit 1; }; \
		cd "$$source_dir"; PROVENANCE_PROFILE=published TAG_VERSION="$$tag" VERSION="$$version" COSIGN_PUBLIC_KEY="$$source_dir/cosign.pub" RELEASE_MANIFEST="$$manifest" RELEASE_MANIFEST_SIG="$$signature" RELEASE_MANIFEST_ATT="$$attestation" SBOM_FILE="$$sbom" PUBLISHED_EVIDENCE="$$published_evidence" GITHUB_REPOSITORY="$(GITHUB_REPOSITORY)" ./scripts/verify-provenance.sh full >/dev/null; $(GO) run ./cmd/evidencecheck --manifest .release/manifest.json --checksum .release/openrouter.sha256 --artifact bin/openrouter --tag "$$tag" --commit "$$commit" --version "$$version"; $(GO) run ./cmd/evidencecheck --published-evidence "$$published_evidence" --tag "$$tag" --commit "$$commit" --version "$$version"; \
		printf '%s\n' "GitHub release evidence verified for $$tag at $$commit"

release-github: release-github-check
	@set -eu; \
		source_dir="$$(cd '$(RELEASE_SOURCE_DIR)' && pwd -P)"; artifact_dir="$$(cd '$(RELEASE_ARTIFACT_DIR)' && pwd -P)"; tag='$(TAG_VERSION)'; repository='$(GITHUB_REPOSITORY)'; notes="$$artifact_dir/RELEASE_NOTES.md"; tmp_dir="$$(mktemp -d)"; trap 'rm -rf "$$tmp_dir"' EXIT; \
		if gh release view "$$tag" --repo "$$repository" >"$$tmp_dir/release-view.out" 2>"$$tmp_dir/release-view.err"; then printf '%s\n' "BLOCKED: GitHub Release $$tag already exists; refusing duplicate publication" >&2; exit 1; else view_status=$$?; if test $$view_status -eq 1 && grep -Eiq '(^|[^0-9])404([^0-9]|$$)|not found' "$$tmp_dir/release-view.err"; then :; else printf '%s\n' "BLOCKED: GitHub release preflight failed; only confirmed not-found permits create (exit $$view_status)" >&2; cat "$$tmp_dir/release-view.err" >&2; exit 1; fi; fi; \
		set -- gh release create "$$tag" --repo "$$repository" --verify-tag --title "$$tag" --notes-file "$$notes" --draft=false --prerelease=false "$$source_dir/.release/release-manifest.json" "$$source_dir/.release/release-manifest.json.sig.bundle.json" "$$source_dir/.release/release-manifest.json.att.bundle.json" "$$source_dir/.release/sbom.spdx.json" "$$source_dir/.release/dependency-evidence.json" "$$source_dir/.release/published-evidence.json" "$$source_dir/.release/openrouter.sha256"; \
		jq -r '.artifacts[].artifact' "$$artifact_dir/manifest.json" > "$$tmp_dir/artifacts"; \
		while IFS= read -r artifact; do artifact_path="$$artifact_dir/$$artifact"; case "$$artifact" in artifacts/[A-Za-z0-9._-]*.tar.gz) ;; *) printf '%s\n' "BLOCKED: unsafe archive path in local release manifest: $$artifact" >&2; exit 1 ;; esac; case "$$artifact_path" in "$$artifact_dir"/artifacts/*) ;; *) printf '%s\n' "BLOCKED: archive path escapes local release directory: $$artifact" >&2; exit 1 ;; esac; set -- "$$@" "$$artifact_path"; done < "$$tmp_dir/artifacts"; \
		if test '$(RELEASE_DRY_RUN)' = 1; then printf 'DRY RUN:'; printf ' %s' "$$@"; printf '\n'; else command -v gh >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is required to publish a GitHub Release' >&2; exit 1; }; gh auth status >/dev/null 2>&1 || { printf '%s\n' 'BLOCKED: gh is not authenticated; run gh auth login' >&2; exit 1; }; if "$$@"; then :; else if gh release view "$$tag" --repo "$$repository" >/dev/null 2>&1; then printf '%s\n' "BLOCKED: GitHub Release $$tag appeared during publication; refusing duplicate publication" >&2; else printf '%s\n' "BLOCKED: GitHub Release $$tag publication failed" >&2; fi; exit 1; fi; fi

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
		'install-hooks  Point core.hooksPath at .githooks (enables the pre-commit secrets-check)' \
		'sbom           Generate SPDX SBOM with Syft (required)' \
		'release-manifest Write and checksum-bind the signed release-manifest.json' \
		'provenance-predicate Write the SLSA v1 provenance predicate' \
		'cosign-key-check Verify the Keychain-stored cosign key is the private half of COSIGN_PUBLIC_KEY' \
		'cosign-sign-release Sign and attest using the cosign key/password from the login Keychain' \
		'sign           Sign release-manifest.json with the static cosign key (no tlog upload)' \
		'attest         Attest release-manifest.json with the SLSA predicate (no tlog upload)' \
		'signature      Verify cosign signature (PROFILE=local|candidate|external|published)' \
		'verify-provenance Verify cosign evidence (PROFILE=local|candidate|external|published)' \
		'checksums      Write SHA-256 checksum for the local artifact' \
		'artifact       Build local artifact and checksum' \
		'manifest       Write local artifact manifest' \
		'check-package  NO-OP: package template is external' \
		'openrouter-launchd-refresh-check Check LaunchAgent plist generation without launchctl mutations' \
		'openrouter-launchd-refresh-install Install the user LaunchAgent' \
		'openrouter-launchd-refresh-uninstall Remove the user LaunchAgent' \
		'openrouter-launchd-refresh-status Show the user LaunchAgent status' \
		'openrouter-launchd-refresh-start Start the user LaunchAgent now' \
		'install        Build and atomically install openrouter (PREFIX/BINDIR/VERSION/TARGET)' \
		'reinstall      Rebuild and install through the canonical local installer' \
		'upgrade        Rebuild and install through the canonical local installer' \
		'uninstall      Remove only the managed openrouter executable' \
		'verify-install Install and verify --version, version, and --help' \
		'install-smoke  Install into a disposable PREFIX and verify the CLI' \
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
		'check-homebrew-formula Validate the local formula against the exact tag and revision' \
		'sync-homebrew-formula Update the local formula from the exact checked-out tag' \
		'homebrew-reinstall Sync, reinstall, and verify the local Homebrew formula' \
		'release-check  Run the non-publishing pre-tag gate (VERSION=...)' \
		'release-build  Build with the normalized version from the exact checked-out tag' \
		'release-local   Run checks and build deterministic local platform archives' \
		'local-release   Alias for release-local' \
		'release-github-check Verify exact-tag GitHub release evidence without publishing' \
		'release-github  Publish an exact-tag GitHub Release (RELEASE_DRY_RUN=1 for command preview)' \
		'verify-local-artifact Verify strict local exact-tag artifact evidence' \
		'verify-release Verify the local stable Homebrew channel read-only' \
		'whats-new      Print exact-version release notes from CHANGELOG.md' \
		'docs           Validate required project documentation' \
		'clean           Remove only bin/openrouter' \
		'help            Show this list of targets'
