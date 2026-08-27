.PHONY: build test check format cross-build npm-pack docker

build:
	go build -trimpath -o seol ./cmd/seol

test:
	go test -race ./...

check:
	@unformatted="$$(gofmt -l cmd internal)"; \
	test -z "$$unformatted" || { echo "Run 'make format' to format:" >&2; echo "$$unformatted" >&2; exit 1; }
	go vet ./...
	go test -race ./...
	npm test
	npm pack --dry-run

format:
	gofmt -w cmd internal

cross-build:
	./scripts/build-all.sh

npm-pack:
	npm pack --dry-run

docker:
	docker build -t seol:dev .
