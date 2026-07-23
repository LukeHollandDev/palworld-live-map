override PROJECT_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
WEB_NPM := npm --prefix "$(PROJECT_ROOT)/web"
BINARY := $(PROJECT_ROOT)/bin/palworld-live-map

.PHONY: ci build check test web-install web-lint web-typecheck web-test web-assets web-build web-check exporter-check image run demo game-assets maps clean distclean

ci: check exporter-check

build: web-build
	mkdir -p "$(dir $(BINARY))"
	go build -o "$(BINARY)" ./cmd/palworld-live-map

check: web-check web-assets
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...

test: web-test web-assets
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

demo: web-build
	DEMO_MODE=true go run ./cmd/palworld-live-map

game-assets:
	"$(PROJECT_ROOT)/exporter/export.sh"

maps: game-assets

clean:
	@test -n "$(PROJECT_ROOT)"
	@test "$(PROJECT_ROOT)" != "/"
	@test -f "$(PROJECT_ROOT)/go.mod"
	rm -rf -- "$(PROJECT_ROOT)/bin" "$(PROJECT_ROOT)/coverage.out" "$(PROJECT_ROOT)/web/coverage" "$(PROJECT_ROOT)/web/dist" "$(PROJECT_ROOT)/exporter/src/bin" "$(PROJECT_ROOT)/exporter/src/obj" "$(PROJECT_ROOT)/exporter/tests/bin" "$(PROJECT_ROOT)/exporter/tests/obj"

distclean: clean
	rm -rf -- "$(PROJECT_ROOT)/build" "$(PROJECT_ROOT)/web/node_modules"
