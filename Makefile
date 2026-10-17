VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X main.version=$(VERSION)

.PHONY: build test race lint doctest clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o patience ./cmd/patience

test:
	go test ./...

race:
	go test -race -count=3 ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	GOTOOLCHAIN=go$$(go list -m -f '{{.GoVersion}}') go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...

doctest:
	go test -run TestReadme -count=1 ./cmd/patience

clean:
	rm -f patience
