BUILD_ARCHS ?= $(shell go env GOOS)/$(shell go env GOARCH)
VERSION ?= $(or ${GIT_TAG},latest)
OUTPUT_NAME ?= $(or ${BINARY_NAME},lens-k8s-proxy)
OUTPUT_SUFFIX ?= $(shell go env GOOS)-$(shell go env GOARCH)
OUTPUT_EXT ?= $(or ${BINARY_EXT},)
OUTPUT = ${OUTPUT_NAME}-${OUTPUT_SUFFIX}${OUTPUT_EXT}

${OUTPUT}:
	go build -ldflags="-w -X main.Version=${VERSION}" -o ${OUTPUT} main.go

build: ${OUTPUT}

.PHONY: clean
clean:
	rm -f lens-k8s-proxy-*