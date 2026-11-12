VERSION ?= 0.1.0
GO ?= go
PNPM ?= pnpm
GOFMT ?= gofmt
GO_TEST_PARALLEL ?= 4
# Extra printf wrappers beyond vet's automatic *f-name detection.
GO_VET_FLAGS ?= -printf.funcs=problemf,invalidf,usagef,checkf
GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.15.0
BASELINE_SAMPLES ?= 30
BASELINE_OUTPUT ?= baseline/results/latest.json
E2E_CACHE ?= .cache/webmux-e2e
E2E_BINARY ?= $(abspath $(E2E_CACHE)/webmux)
# E2E finds the fake agent (internal/acp/testdata/fakeagent) next to the
# backend, named after the agents whose profiles it stands in for.
E2E_AGENTS = $(dir $(abspath $(E2E_BINARY)))agents
E2E_FAKE_AGENTS = $(shell cat web/test/e2e/fake-agent-names.txt)
LDFLAGS = -s -w -X main.version=$(VERSION)
UPDATE_PKG = github.com/rainchan/webmux/internal/update
# Only release builds take part in updates (internal/update.IsRelease).
RELEASE_LDFLAGS = $(LDFLAGS) -X $(UPDATE_PKG).release=1
RELEASESIGN = $(GO) run ./cmd/internal/releasesign
TRAY_PKG = github.com/rainchan/webmux/internal/tray
# Set by release: the Linux server digests the tray may install.
TRAY_LDFLAGS ?=
MAKENSIS ?= makensis
# Pinned nfpm, run through the Go toolchain so no separate install is needed.
NFPM ?= $(GO) run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.48.0
# A development version (X.Y.Z-dev.N) names its binaries, trays and
# installers webmux-<version>-<release name without "webmux-">, so a
# publish to the rolling `dev` Release uploads beside the outgoing build
# instead of overwriting it before the manifest switches (internal/update
# AssetName). Package file names keep the version as is; nfpm writes the
# package version as X.Y.Z~dev.N, which sorts before X.Y.Z.
DEV_VERSION = $(findstring -dev.,$(VERSION))
ASSET_PREFIX = webmux-$(if $(DEV_VERSION),$(VERSION)-)
SIGNING_KEY_ENV = $(if $(DEV_VERSION),WEBMUX_DEV_SIGNING_KEY,WEBMUX_RELEASE_SIGNING_KEY)
LINUX_PACKAGES = webmux_$(VERSION)-1_amd64.deb webmux_$(VERSION)-1_arm64.deb \
	webmux-$(VERSION)-1.x86_64.rpm webmux-$(VERSION)-1.aarch64.rpm
RELEASE_FILES = $(addprefix $(ASSET_PREFIX),linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 \
	windows-amd64.exe windows-arm64.exe tray-windows-amd64.exe tray-windows-arm64.exe \
	setup-windows-amd64.exe setup-windows-arm64.exe) $(LINUX_PACKAGES)

.PHONY: web build tray installer linux-packages release release-build release-sign check docs-check persistence-check go-lint test test-fast test-go test-frontend test-unit test-browser test-storybook test-e2e test-e2e-built test-performance test-browsers-install cover cover-go cover-web frontend e2e-runtime format format-check install baseline test-install
web:
	$(PNPM) --dir web install --frozen-lockfile
	$(PNPM) --dir web build

build: web
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/webmux ./cmd/webmux

# The Windows tray needs no frontend build; its icon and version resources
# come from the committed cmd/webmux-tray/rsrc_windows_*.syso files. A plain
# `make tray` carries no server digests and therefore cannot install webmux.
tray:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS) $(TRAY_LDFLAGS) -H=windowsgui" -o bin/$(ASSET_PREFIX)tray-windows-amd64.exe ./cmd/webmux-tray
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS) $(TRAY_LDFLAGS) -H=windowsgui" -o bin/$(ASSET_PREFIX)tray-windows-arm64.exe ./cmd/webmux-tray

# Packages bin/<prefix>tray-windows-<arch>.exe and bin/<prefix>linux-<arch>
# as they are, so each installer holds the release artifacts byte for byte.
# Needs NSIS 3 (`makensis`); see deploy/windows/webmux.nsi.
installer:
	for arch in amd64 arm64; do \
		$(MAKENSIS) -V2 -INPUTCHARSET UTF8 -DVERSION=$(VERSION) -DARCH=$$arch \
			"-DTRAY=$(abspath bin)/$(ASSET_PREFIX)tray-windows-$$arch.exe" \
			"-DSERVER=$(abspath bin)/$(ASSET_PREFIX)linux-$$arch" \
			"-DICON=$(abspath internal/tray/winui/icons/running.ico)" \
			"-DOUTFILE=$(abspath bin)/$(ASSET_PREFIX)setup-windows-$$arch.exe" \
			deploy/windows/webmux.nsi || exit 1; \
	done

# Packages bin/<prefix>linux-<arch> as /usr/bin/webmux plus the systemd
# units into $(LINUX_PACKAGES); see deploy/linux/nfpm.yaml. The file names
# are given explicitly: nfpm's own names would carry the "~" of a
# development package version, which GitHub rewrites on upload.
linux-packages:
	for arch in amd64 arm64; do \
		case $$arch in amd64) rpm_arch=x86_64 ;; arm64) rpm_arch=aarch64 ;; esac; \
		VERSION=$(VERSION) ARCH=$$arch SERVER=bin/$(ASSET_PREFIX)linux-$$arch $(NFPM) package --config deploy/linux/nfpm.yaml \
			--packager deb --target bin/webmux_$(VERSION)-1_$$arch.deb || exit 1; \
		VERSION=$(VERSION) ARCH=$$arch SERVER=bin/$(ASSET_PREFIX)linux-$$arch $(NFPM) package --config deploy/linux/nfpm.yaml \
			--packager rpm --target bin/webmux-$(VERSION)-1.$$rpm_arch.rpm || exit 1; \
	done
	cd bin && ls $(LINUX_PACKAGES) >/dev/null

# Linux servers first: the tray embeds their digests, and SHA256SUMS lists
# every artifact plus install.sh, which keeps its name in every Release.
# `release` checks the signing key before building anything so a missing
# secret fails fast; a development version signs with
# $WEBMUX_DEV_SIGNING_KEY, a release with $WEBMUX_RELEASE_SIGNING_KEY.
# Sub-makes inherit both the environment and command-line variables
# (through MAKEFLAGS), so release-build runs without either key variable and
# with empty overrides; only release-sign sees the key. CI runs
# release-build without the key and signs in a separate job.
release:
	@test -n "$$$(SIGNING_KEY_ENV)" || { echo 'release: $(SIGNING_KEY_ENV) is not set; cannot sign SHA256SUMS' >&2; exit 1; }
	env -u WEBMUX_RELEASE_SIGNING_KEY -u WEBMUX_DEV_SIGNING_KEY $(MAKE) release-build WEBMUX_RELEASE_SIGNING_KEY= WEBMUX_DEV_SIGNING_KEY=
	$(MAKE) release-sign

release-build:
	$(MAKE) web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o bin/$(ASSET_PREFIX)linux-amd64 ./cmd/webmux
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o bin/$(ASSET_PREFIX)linux-arm64 ./cmd/webmux
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o bin/$(ASSET_PREFIX)darwin-amd64 ./cmd/webmux
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o bin/$(ASSET_PREFIX)darwin-arm64 ./cmd/webmux
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o bin/$(ASSET_PREFIX)windows-amd64.exe ./cmd/webmux-windows
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o bin/$(ASSET_PREFIX)windows-arm64.exe ./cmd/webmux-windows
	amd64=$$(sha256sum bin/$(ASSET_PREFIX)linux-amd64 | cut -d' ' -f1) && \
	arm64=$$(sha256sum bin/$(ASSET_PREFIX)linux-arm64 | cut -d' ' -f1) && \
	$(MAKE) tray TRAY_LDFLAGS="-X $(UPDATE_PKG).release=1 -X $(TRAY_PKG).linuxAMD64SHA256=$$amd64 -X $(TRAY_PKG).linuxARM64SHA256=$$arm64" && \
	for exe in bin/$(ASSET_PREFIX)tray-windows-amd64.exe bin/$(ASSET_PREFIX)tray-windows-arm64.exe; do \
		grep -qaF "$$amd64" $$exe && grep -qaF "$$arm64" $$exe || { echo "$$exe lacks the server digests" >&2; exit 1; }; \
	done
	$(MAKE) installer
	$(MAKE) linux-packages
	cp install.sh bin/install.sh
	cd bin && sha256sum $(RELEASE_FILES) install.sh > SHA256SUMS

# Signs bin/SHA256SUMS into SHA256SUMS.minisig with
# $WEBMUX_RELEASE_SIGNING_KEY, or $WEBMUX_DEV_SIGNING_KEY for a development
# version, then proves the built-in public keys of that group accept it.
release-sign:
	$(RELEASESIGN) sign -version $(VERSION) bin/SHA256SUMS
	$(RELEASESIGN) verify -version $(VERSION) bin/SHA256SUMS

check: web format-check docs-check persistence-check
	$(PNPM) --dir web check:test
	$(PNPM) --dir web lint
	$(PNPM) --dir web check:arch
	$(GO) vet $(GO_VET_FLAGS) ./...
	$(MAKE) go-lint

# Keep local checks on the same linter release as CI.
go-lint:
	@$(GOLANGCI_LINT) version --short | grep -qx '$(GOLANGCI_LINT_VERSION)' || { \
		echo 'Install golangci-lint v$(GOLANGCI_LINT_VERSION); see docs/go-style.md' >&2; exit 1; }
	$(GOLANGCI_LINT) run ./...

test: e2e-runtime
	$(MAKE) --output-sync=target -j test-go test-unit test-browser
	$(MAKE) --output-sync=target -j test-storybook test-e2e-built
	WEBMUX_E2E_BINARY=$(abspath $(E2E_BINARY)) $(PNPM) --dir web test:performance

# Fast iteration: Go race plus Vitest unit/browser, without Storybook, E2E or
# performance. Builds the embedded frontend once; `test-go` can be run alone
# against an existing `web/dist` for Go-only changes.
test-fast: frontend
	$(MAKE) --output-sync=target -j test-go test-unit test-browser

test-go:
	$(GO) test -race -parallel=$(GO_TEST_PARALLEL) ./...

test-frontend:
	$(MAKE) --output-sync=target -j test-unit test-browser

cover: cover-go cover-web

cover-go: frontend
	mkdir -p .cache/coverage
	$(GO) test -race -parallel=$(GO_TEST_PARALLEL) -covermode=atomic -coverpkg=./cmd/...,./internal/...,./web,./deploy -coverprofile=.cache/coverage/raw.coverprofile ./...
	sh .github/scripts/go-coverage-summary.sh .cache/coverage/raw.coverprofile .cache/coverage

cover-web:
	$(PNPM) --dir web test:coverage

# Production frontend for Go's embed and the prepared E2E runtime.
frontend:
	$(PNPM) --dir web exec vite build

# One immutable executable for E2E and performance; each worker still owns its
# backend, ports, HOME, state directory and PTYs. Override E2E_BINARY to change
# the cache location (the path is resolved to an absolute path). Hybrid-session
# E2E puts $(E2E_AGENTS) first on each backend's PATH.
e2e-runtime: frontend
	mkdir -p $(dir $(abspath $(E2E_BINARY))) $(E2E_AGENTS)
	CGO_ENABLED=0 $(GO) build -trimpath -o $(abspath $(E2E_BINARY)) ./cmd/webmux
	CGO_ENABLED=0 $(GO) build -trimpath -o $(E2E_AGENTS)/omp ./internal/acp/testdata/fakeagent
	for name in $(filter-out omp,$(E2E_FAKE_AGENTS)); do cp $(E2E_AGENTS)/omp $(E2E_AGENTS)/$$name; done

baseline: web
	$(GO) run ./cmd/baseline -samples $(BASELINE_SAMPLES) -output $(BASELINE_OUTPUT)

test-unit:
	$(PNPM) --dir web test:unit

test-browser:
	$(PNPM) --dir web test:browser

test-storybook:
	$(PNPM) --dir web test:storybook

test-e2e: e2e-runtime
	$(MAKE) test-e2e-built

test-e2e-built:
	WEBMUX_E2E_BINARY=$(abspath $(E2E_BINARY)) $(PNPM) --dir web test:e2e

test-performance: e2e-runtime
	WEBMUX_E2E_BINARY=$(abspath $(E2E_BINARY)) $(PNPM) --dir web test:performance

test-browsers-install:
	$(PNPM) --dir web exec playwright install --with-deps chromium

persistence-check:
	$(GO) test -count=1 ./internal/archtest -run '^TestPersistence'

docs-check:
	$(GO) test -count=1 ./internal/docguard

test-install:
	sh -n install.sh
	sh .github/scripts/test-install.sh

format:
	$(GOFMT) -w cmd internal web/embed.go deploy/embed.go
	web/node_modules/.bin/oxfmt README.md CONTRIBUTING.md SECURITY.md docs .oxfmtrc.json web

format-check:
	@files="$$($(GOFMT) -l cmd internal web/embed.go deploy/embed.go)"; \
		if [ -n "$$files" ]; then printf 'Unformatted Go files:\n%s\n' "$$files"; exit 1; fi
	web/node_modules/.bin/oxfmt --check README.md CONTRIBUTING.md SECURITY.md docs .oxfmtrc.json web

install: build
	install -d $(HOME)/.local/bin
	install -m 755 bin/webmux $(HOME)/.local/bin/webmux
