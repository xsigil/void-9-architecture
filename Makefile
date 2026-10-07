# Void Architecture™ Sovereign CLI Engine (void)
# Author: parorafia (@xsigil)
# License: MIT

BIN_DIR ?= ./bin
TARGET  ?= $(BIN_DIR)/void
PREFIX  ?= /usr/local
SRC     := main.go

GO_BUILD_FLAGS ?= -ldflags="-s -w"
export CGO_ENABLED ?= 0

.PHONY: all build install uninstall clean fmt vet test dev-scaffold help

all: build

## build: Build static pure Go 'void' CLI binary
build:
	@mkdir -p $(BIN_DIR)
	@printf "[*] Compiling pure static void binary...\n"
	go build $(GO_BUILD_FLAGS) -o $(TARGET) .
	@printf "[+] Built successfully: %s (%s)\n" "$(TARGET)" "$$(du -h $(TARGET) | cut -f1)"

## install: Install binary to $(PREFIX)/bin/void (requires sudo if default prefix)
install: build
	@printf "[*] Installing to %s/bin/void...\n" "$(PREFIX)"
	@install -d -m 0755 "$(PREFIX)/bin"
	@install -m 0755 $(TARGET) "$(PREFIX)/bin/void"
	@printf "[+] 'void' is now live at %s/bin/void\n" "$(PREFIX)"

## uninstall: Remove binary from $(PREFIX)/bin/void
uninstall:
	@printf "[*] Removing %s/bin/void...\n" "$(PREFIX)"
	@rm -f "$(PREFIX)/bin/void"
	@printf "[+] Uninstalled successfully.\n"

## fmt: Format Go sources
fmt:
	@printf "[*] Formatting sources...\n"
	go fmt ./...

## vet: Run go vet inspection
vet:
	go vet ./...

## test: Run unit tests
test:
	go test -v ./...

## dev-scaffold: Test scaffolding an ephemeral target in /tmp
dev-scaffold: build
	@TMP_APP=$$(mktemp -d /tmp/void-demo-XXXXXX); \
	printf "[*] Testing scaffolding in $$TMP_APP/demo...\n"; \
	$(TARGET) "$$TMP_APP/demo"; \
	ls -la "$$TMP_APP/demo"; \
	rm -rf "$$TMP_APP"; \
	printf "[+] Scaffolding self-test passed.\n"

## clean: Remove build artifacts and temporary binaries
clean:
	@printf "[*] Cleansing artifacts...\n"
	@rm -rf $(BIN_DIR)
	@printf "[+] Repository clean.\n"

## help: Display this target index
help:
	@printf "Void Sovereign CLI Engine (void)\n\nUsage:\n  make <target>\n\nTargets:\n"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //g' | awk 'BEGIN {FS = ":"}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
