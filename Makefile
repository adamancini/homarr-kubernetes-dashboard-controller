BINARY    := homarr-dashboard-controller
IMAGE     := ghcr.io/adamancini/homarr-kubernetes-dashboard-controller
VERSION   ?= dev
GOFLAGS   := -trimpath
LDFLAGS   := -s -w -X main.version=$(VERSION)

.PHONY: build test lint docker-build run

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/controller

test:
	go test -race -coverprofile=coverage.txt ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -t $(IMAGE):$(VERSION) .

run: build
	./bin/$(BINARY) $(ARGS)
