override PROJECT_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
WEB_NPM := npm --prefix "$(PROJECT_ROOT)/web"
BINARY := $(PROJECT_ROOT)/bin/palworld-live-map
PYTHON ?= python3
MAP_OUTPUT_DIR ?= $(PROJECT_ROOT)/build/maps
LANDMARK_OUTPUT_DIR ?= $(PROJECT_ROOT)/build/landmarks
MAP_ASSET_DIR := $(PROJECT_ROOT)/assets/palworld/maps
MAP_TILE_VENV := $(PROJECT_ROOT)/build/map-tiles-venv
MAP_TILE_PYTHON := $(MAP_TILE_VENV)/bin/python
MAP_TILE_REQUIREMENTS := $(PROJECT_ROOT)/tools/map-tiles-requirements.txt
MAP_TILE_DEPS := $(MAP_TILE_VENV)/.deps

.PHONY: ci build save-reader check test web-install web-lint web-typecheck web-test web-assets web-build web-check exporter-check image run demo demo-media map-tiles game-assets game-map-tiles game-assets-diff maps clean distclean

ci: check exporter-check

build: map-tiles web-build
	mkdir -p "$(dir $(BINARY))"
	go build -o "$(BINARY)" ./cmd/palworld-live-map

save-reader:
	sh "$(PROJECT_ROOT)/tools/build-save-reader.sh"

check: map-tiles web-check web-assets
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...

test: map-tiles web-test web-assets
	go test ./...

web-install:
	$(WEB_NPM) ci

web-lint: web-install
	$(WEB_NPM) run lint

web-typecheck: web-install
	$(WEB_NPM) run typecheck

web-test: web-install
	$(WEB_NPM) test

web-assets: web-install
	$(WEB_NPM) run build:assets

web-build: web-typecheck web-assets

web-check: web-lint web-typecheck web-test

exporter-check:
	docker build -t palworld-live-map/asset-exporter:check "$(PROJECT_ROOT)/exporter"

image:
	docker build -t palworld-live-map:dev "$(PROJECT_ROOT)"

run: build
	set -a; . ./.env; set +a; "$(BINARY)"

demo: map-tiles web-build
	DEMO_MODE=true go run ./cmd/palworld-live-map

demo-media: map-tiles web-build
	$(WEB_NPM) run demo:media

game-assets:
	MAP_OUTPUT_DIR="$(MAP_OUTPUT_DIR)" LANDMARK_OUTPUT_DIR="$(LANDMARK_OUTPUT_DIR)" "$(PROJECT_ROOT)/exporter/export.sh"
	$(MAKE) MAP_OUTPUT_DIR="$(MAP_OUTPUT_DIR)" LANDMARK_OUTPUT_DIR="$(LANDMARK_OUTPUT_DIR)" game-assets-diff

$(MAP_TILE_DEPS): $(MAP_TILE_REQUIREMENTS)
	mkdir -p "$(dir $(MAP_TILE_VENV))"
	"$(PYTHON)" -m venv "$(MAP_TILE_VENV)"
	"$(MAP_TILE_PYTHON)" -m pip install --disable-pip-version-check --only-binary=Pillow --requirement "$(MAP_TILE_REQUIREMENTS)"
	touch "$@"

map-tiles: $(MAP_TILE_DEPS)
	"$(MAP_TILE_PYTHON)" "$(PROJECT_ROOT)/tools/generate-map-tiles.py" --if-needed "$(MAP_ASSET_DIR)"

game-map-tiles: $(MAP_TILE_DEPS)
	"$(MAP_TILE_PYTHON)" "$(PROJECT_ROOT)/tools/generate-map-tiles.py" --if-needed "$(MAP_OUTPUT_DIR)"

game-assets-diff: map-tiles game-map-tiles
	@status=0; git -C "$(PROJECT_ROOT)" diff --no-index -- assets/palworld/maps "$(MAP_OUTPUT_DIR)" || status=$$?; if [ "$$status" -eq 0 ]; then printf 'No map asset changes.\n'; fi; test "$$status" -le 1
	@status=0; git -C "$(PROJECT_ROOT)" diff --no-index -- assets/palworld/landmarks "$(LANDMARK_OUTPUT_DIR)" || status=$$?; if [ "$$status" -eq 0 ]; then printf 'No landmark asset changes.\n'; fi; test "$$status" -le 1

maps: game-assets

clean:
	@test -n "$(PROJECT_ROOT)"
	@test "$(PROJECT_ROOT)" != "/"
	@test -f "$(PROJECT_ROOT)/go.mod"
	rm -rf -- "$(PROJECT_ROOT)/bin" "$(PROJECT_ROOT)/coverage.out" "$(PROJECT_ROOT)/web/coverage" "$(PROJECT_ROOT)/web/dist" "$(PROJECT_ROOT)/exporter/src/bin" "$(PROJECT_ROOT)/exporter/src/obj" "$(PROJECT_ROOT)/exporter/tests/bin" "$(PROJECT_ROOT)/exporter/tests/obj"

distclean: clean
	rm -rf -- "$(PROJECT_ROOT)/build" "$(PROJECT_ROOT)/web/node_modules"
