.PHONY: ci build check test web-install web-build web-check image run demo maps

ci: check build image

build: web-build
	go build -o bin/palworld-live-map ./cmd/palworld-live-map

check: web-check
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...

test: web-build
	go test ./...

web-install:
	npm --prefix web ci

web-build: web-install
	npm --prefix web run build

web-check: web-install
	npm --prefix web run check
	npm --prefix web test
	npm --prefix web run build

image:
	docker build -t palworld-live-map:dev .

run: web-build
	set -a; . ./.env; set +a; go run ./cmd/palworld-live-map

demo: web-build
	DEMO_MODE=true go run ./cmd/palworld-live-map

maps:
	./tools/map-exporter/export.sh
