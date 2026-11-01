BIN := yak

.PHONY: build test test-docs update fmt vet check install clean

build:
	go build -o $(BIN) ./cmd/yak

test:
	go test ./...

test-docs:
	go test ./internal/engine -run '^TestDocExamples$$' -count=1

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
	go clean -testcache
