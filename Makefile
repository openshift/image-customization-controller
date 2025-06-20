ifeq (/,${HOME})
GOLANGCI_LINT_CACHE=/tmp/golangci-lint-cache/
else
GOLANGCI_LINT_CACHE=${HOME}/.cache/golangci-lint
endif
GOLANGCI_LINT ?= GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) go run vendor/github.com/golangci/golangci-lint/v2/cmd/golangci-lint/main.go

# See pkg/version.go for details
SOURCE_GIT_COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_VERSION ?= $(shell git describe --always --abbrev=40 --dirty)
VERSION_URI = "github.com/openshift/image-customization-controller/pkg/version"
export LDFLAGS="-X $(VERSION_URI).Raw=${BUILD_VERSION} \
                -X $(VERSION_URI).Commit=${SOURCE_GIT_COMMIT} \
                -X $(VERSION_URI).BuildTime=$(shell date +%Y-%m-%dT%H:%M:%S%z)"

IMG ?= image-customization-controller:latest

# Set VERBOSE to -v to make tests produce more output
VERBOSE ?=

all: image-customization-controller image-customization-server

test: generate lint unit

unit:
	go test $(VERBOSE) ./... -coverprofile cover.out

image-customization-controller: generate
	go build -ldflags $(LDFLAGS) -o bin/image-customization-controller cmd/controller/main.go

image-customization-server: generate
	go build -ldflags $(LDFLAGS) -o bin/image-customization-server cmd/static-server/main.go

run:
	go run ./main.go

.PHONY: lint
lint:
	$(GOLANGCI_LINT) run

.PHONY: generate
generate:
	# go generate -x ./...
	$(GOLANGCI_LINT) run --fix

.PHONY: docker
docker: generate
	docker build . -t ${IMG}

.PHONY: docker-push
docker-push:
	docker push ${IMG}
