.PHONY: run run-demo run-client test clean

# Run the demo (starts server + runs client)
run: run-demo

# Run the demo
run-demo:
	cd demo && go run .

# Run the client (expects server running at A2A_SERVER_URL or localhost:8080)
run-client:
	cd client && go run .

# Test demo and internal a2a package
test:
	cd demo && go test -v ./...
	cd ../../gassy && go test -v ./internal/a2a/...

clean:
	rm -f demo.test
