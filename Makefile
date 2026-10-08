# Project metadata
NAME := search-tool-ec2logshipper
BINARY := ec2logshipper
# The VERSION file is the single source of truth for both the binary and the RPM.
VERSION := $(shell cat VERSION)
BUILD_DIR := build

.PHONY: all build clean test rpm

# Default target: build the binary for the current platform.
all: build

# Build the Go binary in the current directory.
build:
	go build -o $(BINARY) .

# Run all Go tests.
test:
	go test -v ./...

# Remove build artifacts and the generated build directory.
clean:
	rm -f $(BINARY)
	rm -rf $(BUILD_DIR)

# Build a source tarball and run rpmbuild to create RPM/SRPM packages.
rpm: build
	mkdir -p $(BUILD_DIR)/rpmbuild/SPECS $(BUILD_DIR)/rpmbuild/SOURCES $(BUILD_DIR)/rpmbuild/BUILD $(BUILD_DIR)/rpmbuild/RPMS $(BUILD_DIR)/rpmbuild/SRPMS
	rm -rf $(BUILD_DIR)/src/$(NAME)-$(VERSION)
	mkdir -p $(BUILD_DIR)/src/$(NAME)-$(VERSION)
	cp -r Makefile VERSION *.go go.mod go.sum config.json.example loggen packaging $(BUILD_DIR)/src/$(NAME)-$(VERSION)/
	tar czf $(BUILD_DIR)/rpmbuild/SOURCES/$(NAME)-$(VERSION).tar.gz -C $(BUILD_DIR)/src $(NAME)-$(VERSION)
	rpmbuild --nodeps --define "_topdir $(abspath $(BUILD_DIR))/rpmbuild" --define "_version $(VERSION)" -ba packaging/rpm/$(NAME).spec
