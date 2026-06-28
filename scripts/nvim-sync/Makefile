BINARY := nvim-sync
CMD    := ./cmd/nvim-sync

.PHONY: build test clean

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

clean:
	rm -f $(BINARY) coverage.out
