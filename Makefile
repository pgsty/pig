#==============================================================#
# File      :   Makefile
# Mtime     :   2024-12-8
# Copyright (C) 2018-2024 Ruohang Feng
#==============================================================#

# Get Current Version
VERSION=v3.1.0
# VERSION=`cat cmd/root.go | grep -E 'const PigstyVersion' | grep -Eo '[a-zA-Z-0-9.]+'`

# Release Dir
LINUX_AMD_DIR:=dist/$(VERSION)/pig-$(VERSION).linux-amd64
LINUX_ARM_DIR:=dist/$(VERSION)/pig-$(VERSION).linux-arm64
DARWIN_AMD_DIR:=dist/$(VERSION)/pig-$(VERSION).darwin-amd64
DARWIN_ARM_DIR:=dist/$(VERSION)/pig-$(VERSION).darwin-arm64



###############################################################
#                        Shortcuts                            #
###############################################################
run:
	go run main.go
build:
	go build -o pig
clean:
	rm -rf

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o pig
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -a -ldflags '-extldflags "-static"' -o pig

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

.PHONY: run build clean build-linux-amd64 build-linux-arm64 release release-linux linux-amd64 linux-arm64
