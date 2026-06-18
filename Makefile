APP := openwrt-fancontrol

VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := \
    -s -w \
    -X main.Version=$(VERSION) \
    -X main.Commit=$(COMMIT) \
    -X main.BuildDate=$(DATE)

.PHONY: build
build:
	go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o bin/$(APP) \
		./cmd/$(APP)

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf bin dist

.PHONY: release
release:
	CGO_ENABLED=0 \
	go build \
		-trimpath \
		-ldflags="$(LDFLAGS)" \
		-o dist/$(APP) \
		./cmd/$(APP)