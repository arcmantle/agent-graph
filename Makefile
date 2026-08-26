.PHONY: build test acceptance lint check

build:
	CGO_ENABLED=1 go build ./cmd/agent-graph

test:
	CGO_ENABLED=1 go test ./...

acceptance:
	CGO_ENABLED=1 go test ./...

lint:
	CGO_ENABLED=1 go vet ./...

check: build acceptance lint