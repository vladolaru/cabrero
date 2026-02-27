VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o cabrero .

install: build
	mkdir -p $(HOME)/.cabrero/bin
	cp cabrero $(HOME)/.cabrero/bin/cabrero
	@codesign -s - $(HOME)/.cabrero/bin/cabrero 2>/dev/null || true
	@echo "Installed to ~/.cabrero/bin/cabrero"
	@ln -sf $(HOME)/.cabrero/bin/cabrero /usr/local/bin/cabrero 2>/dev/null \
		&& echo "Symlinked to /usr/local/bin/cabrero" \
		|| echo "Could not symlink to /usr/local/bin/cabrero (try: sudo ln -sf ~/.cabrero/bin/cabrero /usr/local/bin/cabrero)"
	@echo "Run 'cabrero setup' to complete configuration."

test:
	go test ./...

test-v:
	go test -v ./...

SNAPSHOT_VIEWS := dashboard dashboard-narrow dashboard-empty \
	proposal-detail proposal-detail-chat fitness-report \
	source-manager pipeline-monitor help-overlay help-overlay-vim \
	log-viewer operations

# freeze v0.2.2: --config flag breaks --language ansi, so use CLI flags.
FREEZE_FLAGS := --language ansi --window --padding 20,40,20,40 \
	--border.radius 8 --font.family "JetBrains Mono" --font.size 14 \
	--theme catppuccin-mocha

MAX_PNG_WIDTH := 1200

snapshots:
	@command -v freeze >/dev/null 2>&1 || { echo "freeze not found. Install: brew install charmbracelet/tap/freeze"; exit 1; }
	@mkdir -p snapshots
	@for name in $(SNAPSHOT_VIEWS); do \
		echo "  $$name ..."; \
		CLICOLOR_FORCE=1 go run ./cmd/snapshot $$name | freeze $(FREEZE_FLAGS) -o snapshots/$$name.svg; \
		CLICOLOR_FORCE=1 go run ./cmd/snapshot $$name | freeze $(FREEZE_FLAGS) -o snapshots/$$name.png; \
		sips --resampleWidth $(MAX_PNG_WIDTH) snapshots/$$name.png >/dev/null 2>&1; \
	done
	@echo "Done. Snapshots in snapshots/"

snapshot:
	@test -n "$(VIEW)" || { echo "Usage: make snapshot VIEW=dashboard"; exit 1; }
	@command -v freeze >/dev/null 2>&1 || { echo "freeze not found. Install: brew install charmbracelet/tap/freeze"; exit 1; }
	@mkdir -p snapshots
	CLICOLOR_FORCE=1 go run ./cmd/snapshot $(VIEW) | freeze $(FREEZE_FLAGS) -o snapshots/$(VIEW).svg
	CLICOLOR_FORCE=1 go run ./cmd/snapshot $(VIEW) | freeze $(FREEZE_FLAGS) -o snapshots/$(VIEW).png
	@sips --resampleWidth $(MAX_PNG_WIDTH) snapshots/$(VIEW).png >/dev/null 2>&1
	@echo "snapshots/$(VIEW).{svg,png}"

.PHONY: build install test test-v snapshots snapshot
