BIN := bin/kvcache
PKG := ./...

.PHONY: build test race lint run clean

build:
	go build -o $(BIN) ./cmd/kvcache

test:
	go test $(PKG)

race:
	go test -race $(PKG)

lint:
	go vet $(PKG)

run: build
	./$(BIN)

clean:
	rm -rf bin
