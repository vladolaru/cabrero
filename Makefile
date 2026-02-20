VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o cabrero .

install: build
	mkdir -p $(HOME)/.cabrero/bin
	cp cabrero $(HOME)/.cabrero/bin/cabrero
	@echo "Installed to ~/.cabrero/bin/cabrero"
	@echo "Run 'cabrero setup' to complete configuration."

.PHONY: build install
