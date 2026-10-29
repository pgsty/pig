#==============================================================#
# File      :   Makefile
# Mtime     :   2026-08-14
# Copyright (C) 2018-2026 Ruohang Feng
#==============================================================#
VERSION=v1.10.3
PIG_VERSION:=$(patsubst v%,%,$(VERSION))
PIGSTY_VERSION:=4.5.0
GORELEASER_VERSION:=v2.18.3
GORELEASER:=go run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)
DOCS_DIR?=../pig.pgsty.com

# Build Variables
BRANCH=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
REVISION=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
EXACT_TAG=$(shell git describe --tags --exact-match 2>/dev/null || true)
WORKTREE_STATE=$(shell test -z "$$(git status --porcelain 2>/dev/null)" && echo clean || echo dirty)
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GO_VERSION=$(shell go version | awk '{print $$3}')
DEV_DIRTY_SUFFIX=$(if $(filter dirty,$(WORKTREE_STATE)),.dirty)
BUILD_PIG_VERSION=$(if $(and $(filter $(VERSION),$(EXACT_TAG)),$(filter clean,$(WORKTREE_STATE))),$(PIG_VERSION),$(PIG_VERSION)-dev.$(REVISION)$(DEV_DIRTY_SUFFIX))

# LD Flags for injecting build-time variables
LD_FLAGS=-X 'pig/internal/config.PigVersion=$(BUILD_PIG_VERSION)' \
        -X 'pig/internal/config.PigstyVersion=$(PIGSTY_VERSION)' \
        -X 'pig/internal/config.Branch=$(BRANCH)' \
        -X 'pig/internal/config.Revision=$(REVISION)' \
        -X 'pig/internal/config.BuildDate=$(BUILD_DATE)' \
        -X 'pig/internal/config.GoVersion=$(GO_VERSION)'

###############################################################
#                     Build & Release                         #
###############################################################

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig

rel: release
release: gr-snapshot
# Compatibility alias; GoReleaser owns the full cross-platform artifact matrix.
release-linux: gr-snapshot
linux-amd64: clean build-linux-amd64
linux-arm64: clean build-linux-arm64


###############################################################
#                      GoReleaser                            #
###############################################################
# Resolve the release-pinned GoReleaser into the Go build cache
gr-install:
	@$(GORELEASER) --version

# Build snapshot release (without publishing)
gr-snapshot:
	$(GORELEASER) release --snapshot --clean --skip=publish

# Build release locally (without git tag)
gr-build:
	$(GORELEASER) build --snapshot --clean

# Build release locally without snapshot suffix (requires clean git)
gr-local:
	$(GORELEASER) release --clean --skip=publish

# Release with goreleaser (requires git tag)
release-guard:
	@test "$(EXACT_TAG)" = "$(VERSION)" || { echo "release requires HEAD at exact tag $(VERSION), got $(EXACT_TAG)" >&2; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "release requires a clean worktree" >&2; exit 1; }
	go mod verify
	go test ./...
	go vet ./...

gr-release: release-guard
	$(GORELEASER) release --clean

# Production release (set prerelease to false in config first)
gr-prod-release: release-guard
	@echo "Creating production release (will notify subscribers if announce.skip is false)..."
	$(GORELEASER) release --clean

# Check goreleaser configuration
gr-check: gr-install
	$(GORELEASER) check

# New main release task using goreleaser
release-new: gr-release


###############################################################
#                       Development                           #
###############################################################
u: upload
upload: release-guard
	bin/upload.sh $(VERSION)
tag:
	@printf '%s\n' "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "invalid release version: $(VERSION)" >&2; exit 2; }
	@test -z "$$(git status --porcelain)" || { echo "tag requires a clean worktree" >&2; exit 1; }
	@if git rev-parse -q --verify "refs/tags/$(VERSION)" >/dev/null; then echo "refusing to move existing tag $(VERSION)" >&2; exit 1; fi
	git tag -a $(VERSION) -m "Release $(VERSION)"
run:
	go run .
r: run
b: build
build:
	go build -trimpath -ldflags "$(LD_FLAGS)" -o pig
c: clean
clean:
	rm -rf pig

docs-guard:
	@test -f "$(DOCS_DIR)/hugo.yaml" -a -f "$(DOCS_DIR)/Makefile" || { \
		echo "PIG documentation checkout not found at $(DOCS_DIR)" >&2; \
		echo "set DOCS_DIR=/path/to/pig.pgsty.com or check out the companion repository" >&2; \
		exit 1; \
	}
docs-serve: docs-guard
	$(MAKE) -C "$(DOCS_DIR)" serve
docs-build: docs-guard
	$(MAKE) -C "$(DOCS_DIR)" build
docs-check: docs-guard
	$(MAKE) -C "$(DOCS_DIR)" check
d: docs-serve
db: docs-build
dc: docs-check

###############################################################
#                         Testing                            #
###############################################################
arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
amd:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
2m:
	scp pig meta:/tmp/pig; ssh meta sudo mv /tmp/pig /usr/bin/pig
2c:
	docker cp pig d13a:/usr/bin/pig
2a:
	scp pig ai:/tmp/pig; ssh ai sudo mv /tmp/pig /usr/bin/pig
2j: amd
	scp pig jp:/tmp/pig; ssh jp sudo mv /tmp/pig /usr/bin/pig

.PHONY: run build clean rel release release-linux build-linux-amd64 build-linux-arm64 linux-amd64 linux-arm64 \
 gr-install gr-snapshot gr-build gr-local gr-release gr-prod-release gr-check release-new \
 upload tag docs-guard docs-serve docs-build docs-check d db dc r b c arm amd 2m 2c 2a 2j
