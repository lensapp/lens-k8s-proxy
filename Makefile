BUILD_ARCHS ?= $(shell go env GOOS)/$(shell go env GOARCH)
OUTPUT_NAME ?= $(or ${BINARY_NAME},lens-k8s-proxy)
OUTPUT_SUFFIX ?= $(shell go env GOOS)-$(shell go env GOARCH)
OUTPUT_EXT ?= $(or ${BINARY_EXT},)
OUTPUT = ${OUTPUT_NAME}-${OUTPUT_SUFFIX}${OUTPUT_EXT}
COMMIT = $(shell git rev-parse HEAD)

${OUTPUT}:
	@: $(if ${VERSION},,$(error VERSION is not set))
	go build -ldflags="-w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" -o ${OUTPUT} main.go

${OUTPUT}.sha256: ${OUTPUT}
ifeq ($(OS),Windows_NT)
	certutil.exe -hashfile "${OUTPUT}" SHA256 | findstr.exe /VRC:"[a-f 0-9]" > "${OUTPUT}.sha256"
else
	shasum -a 256 "${OUTPUT}" | awk '{print $$1}' > "${OUTPUT}.sha256"
endif

build: ${OUTPUT} ${OUTPUT}.sha256

.PHONY: clean
clean:
	rm -f lens-k8s-proxy-*
