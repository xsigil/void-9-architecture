# Sovereign Web Application Makefile
APP_NAME := {{MODULE_NAME}}
BIN_DIR  := ./bin
TARGET   := $(BIN_DIR)/$(APP_NAME)

GO_BUILD_FLAGS ?= -trimpath -ldflags="-s -w"
export CGO_ENABLED ?= 0

.PHONY: all build run test clean

all: build

build:
	@mkdir -p $(BIN_DIR)
	@printf "[*] Compiling pure static binary: %s...\n" "$(TARGET)"
	go build $(GO_BUILD_FLAGS) -o $(TARGET) ./cmd
	@printf "[+] Built successfully: %s\n" "$(TARGET)"

run:
	go run ./cmd

test:
	go test -v ./...

clean:
	rm -rf $(BIN_DIR) data/