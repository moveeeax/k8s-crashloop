.PHONY: build test lint fmt install clean
build:
	go build -o k8s-crashloop .
test:
	go test -race ./...
fmt:
	gofmt -w .
lint:
	gofmt -l . && go vet ./...
install:
	go install .
clean:
	rm -f k8s-crashloop
