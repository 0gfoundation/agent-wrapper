.PHONY: all build test clean docker demo docker-dev compose-up compose-down

# Variables
BINARY_NAME=wrapper
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVENDOR=$(GOCMD) mod
GOMOD=$(GOVENDOR) tidy
GOGET=$(GOVENDOR) get

# Docker parameters
DOCKER_IMAGE=0g-citizen-claw/agent-wrapper
DOCKER_TAG=$(VERSION)

all: clean build test

## Build
build: clean
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/wrapper

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/wrapper

## Test
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

test-unit:
	$(GOTEST) -v -short ./...

test-integration:
	$(GOTEST) -v -run Integration ./...

## Dependencies
deps:
	$(GOVENDOR) download
	$(GOVENDOR) tidy

deps-update:
	$(GOGET) -u ./...
	$(GOVENDOR) tidy

## Lint
lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

## Clean
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html

## Docker
docker:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest

docker-push:
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

## Run
run: build
	./bin/$(BINARY_NAME)

## Demo mode - run locally with demo agent
demo: build
	@echo "Starting demo mode..."
	@echo "Wrapper: http://localhost:8080"
	@echo "Initialize with: ./examples/init-demo.sh"
	@DEMO_MODE=true ./bin/$(BINARY_NAME)

## Docker development build
docker-dev:
	docker build -t $(DOCKER_IMAGE):dev -f Dockerfile.dev .

## Docker development run
docker-dev-run:
	docker run --rm -p 8080:8080 -p 9000:9000 \
		-e DEMO_MODE=true \
		$(DOCKER_IMAGE):dev

## Docker Compose
compose-up:
	docker-compose up -d

compose-down:
	docker-compose down

compose-logs:
	docker-compose logs -f
