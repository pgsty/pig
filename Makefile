#==============================================================#
# File      :   Makefile
# Mtime     :   2025-02-14
# Copyright (C) 2018-2025 Ruohang Feng
#==============================================================#
VERSION=v0.6.1

###############################################################
#                     Build & Release                         #
###############################################################

# Release Dir
LINUX_AMD_DIR:=dist/$(VERSION)/pig-$(VERSION).linux-amd64
LINUX_ARM_DIR:=dist/$(VERSION)/pig-$(VERSION).linux-arm64
DARWIN_AMD_DIR:=dist/$(VERSION)/pig-$(VERSION).darwin-amd64
DARWIN_ARM_DIR:=dist/$(VERSION)/pig-$(VERSION).darwin-arm64

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o pig
	upx pig
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o pig
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
r: run
run:
	go run main.go
b: build
build:
	go build -o pig
c: clean
clean:
	rm -rf pig
d:
	hugo serve
b:
	hugo --minify


.PHONY: run build clean build-linux-amd64 build-linux-arm64 release release-linux linux-amd64 linux-arm64 \
 goreleaser-install goreleaser-snapshot goreleaser-build goreleaser-release goreleaser-test-release \
 goreleaser-check release-new goreleaser-local
