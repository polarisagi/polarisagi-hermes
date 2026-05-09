.PHONY: all build build-mac build-linux build-windows build-all clean

BINARY_NAME=polaris-gateway
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags="-s -w -X 'polaris-gateway/internal/webapi.Version=${VERSION}'"

all: clean build

build:
	go build ${LDFLAGS} -o bin/${BINARY_NAME} ./cmd/polaris

build-mac:
	GOOS=darwin GOARCH=arm64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-darwin-arm64 ./cmd/polaris
	GOOS=darwin GOARCH=amd64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-darwin-amd64 ./cmd/polaris

build-linux:
	GOOS=linux GOARCH=amd64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-linux-amd64 ./cmd/polaris
	GOOS=linux GOARCH=arm64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-linux-arm64 ./cmd/polaris

build-windows:
	GOOS=windows GOARCH=amd64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-windows-amd64.exe ./cmd/polaris
	GOOS=windows GOARCH=arm64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-windows-arm64.exe ./cmd/polaris

build-all: build-mac build-linux build-windows

clean:
	rm -rf bin/
