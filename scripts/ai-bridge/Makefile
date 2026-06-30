BINARY := ai-bridge
CMD    := ./cmd/ai-bridge

.PHONY: build sign install clean test generate

build:
	go build -o $(BINARY) $(CMD)

generate:
	go generate ./...

sign: build
	codesign --sign - --force $(BINARY)

install: sign
	./$(BINARY) install-launchd

test:
	go test ./...

clean:
	rm -f $(BINARY) coverage.out
