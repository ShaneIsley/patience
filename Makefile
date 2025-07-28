# Makefile for the patience project

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOINSTALL=$(GOCMD) install
GOTOOL=$(GOCMD) tool

# Binaries
BINARY_NAME_PATIENCE=patience
BINARY_NAME_PATIENCED=patienced

all: build

## Build
build:
	@echo "Building binaries..."
	$(GOBUILD) -o $(BINARY_NAME_PATIENCE) ./cmd/patience
	$(GOBUILD) -o $(BINARY_NAME_PATIENCED) ./cmd/patienced

## Test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

## Benchmark
benchmark:
	@echo "Running benchmarks..."
	cd benchmarks && $(GOTEST) -bench=.

## Install
install:
	@echo "Installing binaries..."
	./scripts/install.sh

## Clean
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME_PATIENCE) $(BINARY_NAME_PATIENCED)
	$(GOCLEAN)

.PHONY: all build test benchmark install clean
