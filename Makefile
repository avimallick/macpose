VERSION ?= dev
MODULE := github.com/avimallick/macpose
BINARY := bin/macpose
LDFLAGS := -X $(MODULE)/internal/version.Version=$(VERSION)

.PHONY: build test lint fmt install clean snapshot completions

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/macpose

test:
	go test ./...

lint: fmt
	go vet ./...

fmt:
	@gofmt -w $$(go list -f '{{.Dir}}' ./...)

install: build
	install -d "$$(go env GOPATH)/bin"
	install -m 0755 $(BINARY) "$$(go env GOPATH)/bin/macpose"

clean:
	rm -rf bin dist completions/*.bash completions/*.zsh completions/*.fish

snapshot:
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/macpose-darwin-arm64 ./cmd/macpose

completions: build
	@mkdir -p completions
	./$(BINARY) completion bash > completions/macpose.bash
	./$(BINARY) completion zsh > completions/macpose.zsh
	./$(BINARY) completion fish > completions/macpose.fish
