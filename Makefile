BIN := awesome_labs
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

# No cgo: the binary doesn't depend on the build machine's C library.
export CGO_ENABLED := 0

.PHONY: build lint install release clean
build:
	go build -o $(BIN) .

lint: build
	./$(BIN) lint

install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)

# Build a binary for every platform into dist/, with checksums.
release:
	rm -rf dist && mkdir -p dist
	for p in $(PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} go build -trimpath -ldflags="-s -w" \
			-o dist/$(BIN)-$${p%/*}-$${p#*/} . || exit 1; \
	done
	cd dist && if command -v sha256sum >/dev/null; then sha256sum $(BIN)-*; else shasum -a 256 $(BIN)-*; fi > SHA256SUMS

clean:
	rm -f $(BIN)
	rm -rf dist
