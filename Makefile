BINARY   := fcm
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags "-s -w -X fcm.dev/fcm-cli/cmd/fcm.Version=$(VERSION)"
DOCKER_IMAGE := fcm-dev

.PHONY: build test lint clean docker-build docker-shell install release

# Build the dev Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE) -f Dockerfile.dev .

# Build inside Docker, output binary to ./bin/
build:
	docker run --rm -v $(PWD):/src -w /src $(DOCKER_IMAGE) \
		go build $(LDFLAGS) -o /src/bin/$(BINARY) ./cmd/fcm

# Run tests inside Docker
test:
	docker run --rm -v $(PWD):/src -w /src $(DOCKER_IMAGE) \
		go test -race -count=1 ./...

# Run linter
lint:
	docker run --rm -v $(PWD):/src -w /src \
		golangci/golangci-lint:v1.62 golangci-lint run

# Interactive shell in dev container
docker-shell:
	docker run --rm -it -v $(PWD):/src -w /src $(DOCKER_IMAGE) bash

# Install binary locally (requires sudo)
install: build
	sudo cp ./bin/$(BINARY) /usr/local/bin/$(BINARY)

# Cross-compile for both architectures
release:
	docker run --rm -v $(PWD):/src -w /src $(DOCKER_IMAGE) sh -c \
		'GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o /src/bin/$(BINARY)-linux-amd64 ./cmd/fcm && \
		 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o /src/bin/$(BINARY)-linux-arm64 ./cmd/fcm'

clean:
	rm -rf bin/
