BINARY_NAME=starling-webhook

.PHONY: build test lint clean

build:
	go build -o $(BINARY_NAME) .

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l .

clean:
	rm -f $(BINARY_NAME)
