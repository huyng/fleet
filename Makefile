# Build, test and check fleet.
#
# The master embeds the web page, so the page has to be built before the Go
# binary. Every target below does that in the right order.
#
# Common use:
#   make             build ./fleet
#   make test        run the Go and frontend tests
#   make check       everything CI would run
#
# Variables you can override, for example `make BIN=/usr/local/bin/fleet`:
BIN     ?= fleet
GO      ?= go
NPM     ?= npm
GOFLAGS ?=
UI      := ui
DIST    := internal/web/dist

# The stamp records when the page was last built. Everything that feeds the
# page goes in its dependency list, so an edit to any of it triggers a rebuild.
PAGE_STAMP  := $(DIST)/index.html
PAGE_SRC    := $(shell find $(UI)/src $(UI)/public -type f 2>/dev/null) \
               $(UI)/index.html $(UI)/build.mjs $(UI)/package.json
DEPS_STAMP  := $(UI)/node_modules/.package-lock.json
GO_SRC      := $(shell find cmd internal -name '*.go' 2>/dev/null)

# A static binary: nothing in fleet needs cgo, and a static one copies to
# another machine without matching library versions.
export CGO_ENABLED ?= 0

.DEFAULT_GOAL := build
.PHONY: build ui deps test test-go test-ui vet fmt fmt-check check \
        audit screenshots clean clean-ui help

## build: compile ./fleet, building the page first (the default goal)
build: $(BIN)

# The binary depends on the Go source and on the built page. Depending on the
# stamp file rather than on the `ui` alias matters: a target that is not a file
# is always considered out of date, which would relink the binary every run.
$(BIN): $(GO_SRC) go.mod go.sum $(PAGE_STAMP)
	$(GO) build $(GOFLAGS) -o $@ ./cmd/fleet

## ui: build the page into internal/web/dist
ui: $(PAGE_STAMP)

$(PAGE_STAMP): $(PAGE_SRC) $(DEPS_STAMP)
	cd $(UI) && $(NPM) run build
	@test -f $@ || { echo "the page build did not produce $@"; exit 1; }

# Dependencies are only installed when the directory is missing or the lock
# file changed, so repeat builds stay quick.
$(DEPS_STAMP): $(UI)/package-lock.json $(UI)/package.json
	cd $(UI) && $(NPM) ci
	@touch $@

## deps: install Go modules and page dependencies
deps: $(DEPS_STAMP)
	$(GO) mod download

## test: Go tests and frontend tests
test: test-go test-ui

test-go:
	$(GO) test ./...

test-ui: $(DEPS_STAMP)
	cd $(UI) && $(NPM) test

## vet: go vet here, and the darwin build the node has to support
vet:
	$(GO) vet ./...
	GOOS=darwin GOARCH=arm64 $(GO) vet ./...

## fmt: format the Go code
fmt:
	gofmt -l -w .

# For CI: fails when any file is unformatted, or cannot be parsed at all.
# gofmt reports parse errors on stderr and exits non-zero with no file names,
# so the exit status has to be checked too; otherwise broken code would pass.
fmt-check:
	@out="$$(gofmt -l .)"; status=$$?; \
	if [ -n "$$out" ]; then \
		echo "not formatted:"; echo "$$out"; \
		echo "run: make fmt"; exit 1; \
	fi; \
	if [ $$status -ne 0 ]; then \
		echo "gofmt could not read every file, see the errors above"; exit 1; \
	fi

## check: format, vet and test everything
check: fmt-check vet test

## audit: measure the page for layout and contrast problems (needs URL=...)
audit: $(DEPS_STAMP)
	@test -n "$(URL)" || { echo "usage: make audit URL=http://127.0.0.1:8080/"; exit 1; }
	cd $(UI) && node test/audit.mjs $(URL)

## screenshots: capture the page at desktop and mobile widths (needs URL=...)
screenshots: $(DEPS_STAMP)
	@test -n "$(URL)" || { echo "usage: make screenshots URL=http://127.0.0.1:8080/"; exit 1; }
	cd $(UI) && node test/screenshot.mjs $(URL) $(or $(OUT),/tmp/shots)

clean:
	rm -f $(BIN)

# Leaves the committed page bundle alone, so `go build` still works after it.
clean-ui:
	rm -rf $(UI)/node_modules $(UI)/test/.build

## help: list these targets
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## *//; s/^/  /'

# Packaging for a release lives in its own file, because it is a separate job
# from the everyday build. It also works on its own: `make -f release.mk`.
include release.mk
