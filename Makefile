BIN := yak

.PHONY: build test update fmt vet check install clean docs docs-serve

build:
	go build -o $(BIN) ./cmd/yak

test:
	go test ./...

# Rewrite the golden files under testdata/ from current behavior. Only the
# engine, schemaimport and yakfmt tests read them, and only their test
# binaries take -update.
update:
	go test ./internal/engine ./internal/schemaimport ./internal/yakfmt -update

fmt:
	gofmt -l -w .

vet:
	go vet ./...

check: vet test

install:
	go install ./cmd/yak

clean:
	rm -f $(BIN)
	rm -rf site
	go clean -testcache

# The docs site is built with Zensical, run through uvx so nothing is installed.
docs:
	uvx zensical build --clean

docs-serve:
	uvx zensical serve
