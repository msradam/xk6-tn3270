.PHONY: build test test-unit test-simbank clean deps fmt

build:
	xk6 build --with github.com/msradam/xk6-tn3270=.

test-unit:
	go test -v -race ./...

test: test-unit

test-simbank: build
	./k6 run examples/simbank-test.js

clean:
	rm -f k6
	rm -rf screenshots/
	go clean

deps:
	go mod tidy

fmt:
	go fmt ./...
	gofmt -s -w .

check-deps:
	@which xk6 > /dev/null || (echo "xk6 not found. Install with: go install go.k6.io/xk6/cmd/xk6@latest" && exit 1)
	@echo "All dependencies available"
