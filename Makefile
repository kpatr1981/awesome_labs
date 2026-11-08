BIN := awesome_labs

.PHONY: build lint install clean
build:
	go build -o $(BIN) .

lint: build
	./$(BIN) lint

install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)

clean:
	rm -f $(BIN)
