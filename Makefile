#==============================================================#
# File      :   Makefile
# Mtime     :   2025-11-10
# Copyright (C) 2018-2025 Ruohang Feng
#==============================================================#
VERSION=v0.7.5

# Build Variables
BRANCH=$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
REVISION=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GO_VERSION=$(shell go version | awk '{print $$3}')

# LD Flags for injecting build-time variables
LD_FLAGS=-X 'pig/internal/config.PigVersion=$(VERSION)' \
        -X 'pig/internal/config.Branch=$(BRANCH)' \
        -X 'pig/internal/config.Revision=$(REVISION)' \
        -X 'pig/internal/config.BuildDate=$(BUILD_DATE)' \
        -X 'pig/internal/config.GoVersion=$(GO_VERSION)'

###############################################################
#                     Build & Release                         #
###############################################################

# Release Dir
LINUX_AMD_DIR:=dist/$(VERSION)/pig-$(VERSION).linux-amd64
LINUX_ARM_DIR:=dist/$(VERSION)/pig-$(VERSION).linux-arm64
DARWIN_AMD_DIR:=dist/$(VERSION)/pig-$(VERSION).darwin-amd64
DARWIN_ARM_DIR:=dist/$(VERSION)/pig-$(VERSION).darwin-arm64

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -a -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
	upx pig
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -a -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
	upx pig

r: release
release: release-linux
release-linux: linux-amd64 linux-arm64
linux-amd64: clean build-linux-amd64
	rm -rf $(LINUX_AMD_DIR) && mkdir -p $(LINUX_AMD_DIR)
	nfpm package --packager rpm --config package/nfpm-amd64.yaml --target dist/$(VERSION)
	nfpm package --packager deb --config package/nfpm-amd64.yaml --target dist/$(VERSION)
	cp -r pig $(LINUX_AMD_DIR)/pig
	tar -czf dist/$(VERSION)/pig-$(VERSION).linux-amd64.tar.gz -C dist/$(VERSION) pig-$(VERSION).linux-amd64
	rm -rf $(LINUX_AMD_DIR)

linux-arm64: clean build-linux-arm64
	rm -rf $(LINUX_ARM_DIR) && mkdir -p $(LINUX_ARM_DIR)
	nfpm package --packager rpm --config package/nfpm-arm64.yaml --target dist/$(VERSION)
	nfpm package --packager deb --config package/nfpm-arm64.yaml --target dist/$(VERSION)
	cp -r pig $(LINUX_ARM_DIR)/pig
	tar -czf dist/$(VERSION)/pig-$(VERSION).linux-arm64.tar.gz -C dist/$(VERSION) pig-$(VERSION).linux-arm64
	rm -rf $(LINUX_ARM_DIR)


###############################################################
#                      GoReleaser                            #
###############################################################
# Install goreleaser if not present
gr-install:
	@which goreleaser > /dev/null || (echo "Installing goreleaser..." && go install github.com/goreleaser/goreleaser/v2@latest)

# Build snapshot release (without publishing)
gr-snapshot:
	goreleaser release --snapshot --clean --skip=publish

# Build release locally (without git tag)
gr-build:
	goreleaser build --snapshot --clean

# Build release locally without snapshot suffix (requires clean git)
gr-local:
	goreleaser release --clean --skip=publish

# Release with goreleaser (requires git tag)
gr-release:
	goreleaser release --clean

# Production release (set prerelease to false in config first)
gr-prod-release:
	@echo "Creating production release (will notify subscribers if announce.skip is false)..."
	goreleaser release --clean

# Check goreleaser configuration
gr-check: goreleaser-install
	goreleaser check

# New main release task using goreleaser
release-new: goreleaser-release


###############################################################
#                       Development                           #
###############################################################
u: upload
upload:
	bin/upload.sh
r: run
run:
	go run main.go
b: build
build:
	go build -ldflags "$(LD_FLAGS)" -o pig
c: clean
clean:
	rm -rf pig

d:
	hugo serve
b:
	hugo --minify

###############################################################
#                         Testing                            #
###############################################################
arm:
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -a -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
amd:
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -a -ldflags "$(LD_FLAGS) -extldflags '-static'" -o pig
2m:
	scp pig meta:/tmp/pig; ssh meta sudo mv /tmp/pig /usr/bin/pig
2c:
	docker cp pig d13a:/usr/bin/pig
2a:
	scp pig ai:/tmp/pig; ssh ai sudo mv /tmp/pig /usr/bin/pig

.PHONY: run build clean build-linux-amd64 build-linux-arm64 release release-linux linux-amd64 linux-arm64 \
 goreleaser-install goreleaser-snapshot goreleaser-build goreleaser-release goreleaser-test-release \
 goreleaser-check release-new goreleaser-local
