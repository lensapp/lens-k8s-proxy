BUILD_ARCHS ?= $(shell go env GOOS)/$(shell go env GOARCH)
VERSION ?= ${GITHUB_REF}
OUTPUT_NAME ?= $(or ${BINARY_NAME},lens-k8s-proxy)
OUTPUT_SUFFIX ?= $(shell go env GOOS)-$(shell go env GOARCH)
OUTPUT_EXT ?= $(or ${BINARY_EXT},)
OUTPUT = ${OUTPUT_NAME}-${OUTPUT_SUFFIX}${OUTPUT_EXT}
COMMIT = $(shell git rev-parse HEAD)

ifeq (${VERSION},)
$(error VERSION is not set)
endif

${OUTPUT}:
	go build -ldflags="-w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" -o ${OUTPUT} main.go

build: ${OUTPUT}

.PHONY: clean
clean:
	rm -f lens-k8s-proxy-*
