# Package the binary into the tarballs a release is made of.
#
# The output of this file is what install.sh downloads, so `make release` on a
# developer machine produces exactly the same thing a tagged release does. That
# is what makes the installer testable without GitHub.
#
# The page bundle in internal/web/dist is committed, so packaging needs only Go:
# no Node, no frontend build.

# Defaults, so this file works on its own as well as included from the main
# Makefile.
GO  ?= go
NPM ?= npm

# Read the version from the single place it is defined.
VERSION := $(shell sed -n 's/^const Version = "\(.*\)"/\1/p' internal/version/version.go)

# The platforms a release covers. The node's status gathering only knows Linux
# and macOS, so there is nothing to ship for Windows.
PLATFORMS := linux_amd64 linux_arm64 darwin_amd64 darwin_arm64

# Where the tarballs are written, and the file that lists their hashes.
RELEASE_DIR := dist
CHECKSUMS   := $(RELEASE_DIR)/checksums.txt

.PHONY: release checksums clean-release list-assets

## release: build the tarballs and checksums for every platform
release: $(CHECKSUMS)

# One tarball per platform, each holding a directory with the binary, the
# README and the example configs. The directory keeps extraction from
# clobbering anything already in the target directory.
$(RELEASE_DIR)/fleet_$(VERSION)_%.tar.gz: | $(RELEASE_DIR)
	@test -n "$(VERSION)" || { echo "could not read the version from internal/version/version.go"; exit 1; }
	CGO_ENABLED=0 GOOS=$(word 1,$(subst _, ,$*)) GOARCH=$(word 2,$(subst _, ,$*)) \
		$(GO) build -trimpath -ldflags "-s -w" -o $(RELEASE_DIR)/fleet ./cmd/fleet
	@dir="$(RELEASE_DIR)/fleet_$(VERSION)_$*"; \
	mkdir -p "$$dir"; \
	cp $(RELEASE_DIR)/fleet "$$dir/fleet"; \
	cp README.md "$$dir/README.md"; \
	mkdir -p "$$dir/examples"; \
	cp examples/*.toml "$$dir/examples/"; \
	tar -czf "$@" -C $(RELEASE_DIR) "fleet_$(VERSION)_$*"; \
	rm -rf "$$dir"; \
	rm -f $(RELEASE_DIR)/fleet; \
	echo "made $@"

$(RELEASE_DIR):
	@mkdir -p $@

# Gather every tarball and hash them. install.sh verifies against this file.
$(CHECKSUMS): $(addprefix $(RELEASE_DIR)/fleet_$(VERSION)_,$(addsuffix .tar.gz,$(PLATFORMS))) | $(RELEASE_DIR)
	@cd $(RELEASE_DIR) && \
	if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum fleet_$(VERSION)_*.tar.gz > checksums.txt; \
	else \
		shasum -a 256 fleet_$(VERSION)_*.tar.gz > checksums.txt; \
	fi
	@echo "made $(CHECKSUMS):"
	@cat $(CHECKSUMS)

## checksums: rebuild the checksum file from existing tarballs
checksums: $(CHECKSUMS)

## list-assets: show what a release would upload
list-assets: release
	@ls -lh $(RELEASE_DIR)/fleet_$(VERSION)_*.tar.gz $(CHECKSUMS)

clean-release:
	rm -rf $(RELEASE_DIR)
