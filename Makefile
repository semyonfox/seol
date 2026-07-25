.PHONY: build test check cross-build npm-pack docker

build:
	go build -trimpath -o seol ./cmd/seol

test:
	go test -race ./...

check:
	gofmt -w cmd internal
	go vet ./...
	go test -race ./...

cross-build:
	./scripts/build-all.sh

npm-pack:
	npm pack --dry-run

docker:
	docker build -t seol:dev .
