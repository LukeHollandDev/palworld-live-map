.PHONY: build check test image

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
