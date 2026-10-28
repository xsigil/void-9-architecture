# Void Architecture™ Sovereign CLI Engine (void)
# Author: parorafia (@xsigil)
# License: MIT

BIN_DIR   ?= ./bin
DIST_DIR  ?= ./dist
TARGET    ?= $(BIN_DIR)/void
PREFIX    ?= /usr/local
SRC       := .

# Version and Packaging
VERSION   ?= 1.0.0
ARCH      ?= amd64
DEB_DIR   ?= $(DIST_DIR)/deb/void-9_$(VERSION)_$(ARCH)
DEB_FILE  ?= $(DIST_DIR)/void-9_$(VERSION)_$(ARCH).deb

GO_BUILD_FLAGS ?= -ldflags="-s -w"
export CGO_ENABLED ?= 0

.PHONY: all build deb install uninstall clean fmt vet test dev-scaffold help

all: build

## build: Build static pure Go 'void' CLI binary
build:
	@mkdir -p $(BIN_DIR)
	@printf "[*] Compiling pure static void binary...\n"
	go build $(GO_BUILD_FLAGS) -o $(TARGET) $(SRC)
	@printf "[+] Built successfully: %s (%s)\n" "$(TARGET)" "$$(du -h $(TARGET) | cut -f1)"

## deb: Build Debian package (.deb) for Debian / Ubuntu hosts
deb: build
	@printf "[*] Staging Debian package layout in %s...\n" "$(DEB_DIR)"
	@rm -rf "$(DEB_DIR)"
	@mkdir -p "$(DEB_DIR)/DEBIAN"
	@mkdir -p "$(DEB_DIR)$(PREFIX)/bin"
	@mkdir -p "$(DEB_DIR)$(PREFIX)/share/void-9"
	@install -m 0755 $(TARGET) "$(DEB_DIR)$(PREFIX)/bin/void"
	@if [ -d "templates" ]; then \
		cp -r templates "$(DEB_DIR)$(PREFIX)/share/void-9/"; \
	fi
	@printf "[*] Writing package control file...\n"
	@cat <<-'EOF' > "$(DEB_DIR)/DEBIAN/control"
	Package: void-9
	Version: $(VERSION)
	Section: utils
	Priority: optional
	Architecture: $(ARCH)
	Maintainer: Section 9 Architecture <support@section-9.sh>
	Description: Minimalist containerless isolation & packaging primitive
	 Linux namespace, pivot_root, and bind-mount ephemeral jail execution tool.
	EOF
	@find "$(DEB_DIR)" -type d -exec chmod 0755 {} +
	@mkdir -p "$(DIST_DIR)"
	@dpkg-deb --build --root-owner-group "$(DEB_DIR)" "$(DEB_FILE)"
	@printf "[+] Debian package generated: %s (%s)\n" "$(DEB_FILE)" "$$(du -h $(DEB_FILE) | cut -f1)"

## install: Install binary to $(PREFIX)/bin/void (requires sudo if default prefix)
install: build
	@printf "[*] Installing to %s/bin/void...\n" "$(PREFIX)"
	@install -d -m 0755 "$(PREFIX)/bin"
	@install -m 0755 $(TARGET) "$(PREFIX)/bin/void"
	@if [ -d "templates" ]; then \
		install -d -m 0755 "$(PREFIX)/share/void-9"; \
		cp -r templates "$(PREFIX)/share/void-9/"; \
	fi
	@printf "[+] 'void' is now live at %s/bin/void\n" "$(PREFIX)"

## uninstall: Remove binary and templates from $(PREFIX)
uninstall:
	@printf "[*] Removing %s/bin/void...\n" "$(PREFIX)"
	@rm -f "$(PREFIX)/bin/void"
	@rm -rf "$(PREFIX)/share/void-9"
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

## clean: Remove build artifacts, distribution packages, and temporary binaries
clean:
	@printf "[*] Cleansing artifacts...\n"
	@rm -rf $(BIN_DIR) $(DIST_DIR)
	@printf "[+] Repository clean.\n"

## help: Display this target index
help:
	@printf "Void Sovereign CLI Engine (void)\n\nUsage:\n  make <target>\n\nTargets:\n"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //g' | awk 'BEGIN {FS = ":"}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'