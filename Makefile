BIN := awesome_labs
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# No cgo: the binary doesn't depend on the build machine's C library.
export CGO_ENABLED := 0

.PHONY: build lint verified docs install release clean
build:
	go build -ldflags="$(LDFLAGS)" -o $(BIN) .

lint: build
	./$(BIN) lint

# The release gate: every exercise must have passed `awesome_labs verify` in its
# current form (see verified.json and docs/AUTHORING.md).
verified: build
	./$(BIN) verified

# Rewrite the exercise counts in README.md and docs/PENDING.md.
docs: build
	./$(BIN) counts

install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)

# Build a binary for every platform into dist/, with checksums. It refuses while
# any exercise is unverified or the docs' counts are stale; FORCE=1 builds anyway.
release: build
	@if ! ./$(BIN) verified -q; then \
		if [ "$(FORCE)" = 1 ]; then \
			echo; echo "!!! FORCE=1: releasing with UNVERIFIED exercises (listed by: ./$(BIN) verified) !!!"; echo; \
		else \
			echo "release refused: verify them (awesome_labs verify -record verified.json ...), or make release FORCE=1"; exit 1; \
		fi; \
	fi
	@./$(BIN) counts -check >/dev/null || { echo "release refused: the docs' exercise counts are stale; run: make docs"; exit 1; }
	rm -rf dist && mkdir -p dist
	for p in $(PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} go build -trimpath -ldflags="-s -w $(LDFLAGS)" \
			-o dist/$(BIN)-$${p%/*}-$${p#*/} . || exit 1; \
	done
	cd dist && if command -v sha256sum >/dev/null; then sha256sum $(BIN)-*; else shasum -a 256 $(BIN)-*; fi > SHA256SUMS

clean:
	rm -f $(BIN)
	rm -rf dist
