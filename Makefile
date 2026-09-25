BIN := yak

.PHONY: build test update fmt vet check install clean

build:
	go build -o $(BIN) ./cmd/yak

test:
	go test ./...

# Rewrite the golden files under testdata/ from current behavior.
update:
	go test ./... -update

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
