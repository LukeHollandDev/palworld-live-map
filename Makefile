.PHONY: build check test image run demo maps

build:
	go build -o bin/palworld-live-map ./cmd/palworld-live-map

check:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...

test:
	go test ./...

image:
	docker build -t palworld-live-map:dev .

run:
	set -a; . ./.env; set +a; go run ./cmd/palworld-live-map

demo:
	DEMO_MODE=true go run ./cmd/palworld-live-map

maps:
	./tools/maps/export
