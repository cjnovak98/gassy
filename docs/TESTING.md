# Testing Gassy

## Quick Test

```bash
cd gassy
make test       # Run Go unit tests
make validate   # Run end-to-end PoC demo test
```

## Test Targets

| Target | What it does |
|--------|-------------|
| `make test` | Runs Go unit tests for all internal packages |
| `make validate` | Runs the PoC demo end-to-end, verifying A2A communication |

## What `make validate` checks

The PoC demo exercises the full A2A stack:

1. **Agent Card Discovery** - Fetches `/.well-known/agent.json` and verifies the agent exposes correct capabilities
2. **A2A Messaging** - Sends a JSON-RPC 2.0 message via `sendMessage` and receives a task
3. **Task Retrieval** - Calls `getTask` to verify the task exists on the server
4. **SSE Streaming** - Sends a streaming message and consumes SSE events (status updates, text deltas)

## Understanding the PoC Demo

The demo at `examples/poc/demo/main.go` is a self-contained A2A server + client:

- Starts an A2A server with two handlers:
  - `messageHandler` - synchronous task completion
  - `streamingMessageHandler` - SSE-based streaming responses
- Uses `httptest.NewServer` so no port conflicts or containers needed
- Runs in-process, so cleanup is automatic

## Writing Tests

### Unit Tests (Go)

Unit tests live alongside the code they test:

```
internal/a2a/
  client.go
  client_test.go    <- tests for client
  server.go
  server_test.go    <- tests for server
```

Use `httptest.NewServer` to test HTTP interactions without real servers.

### End-to-End Tests

For integration tests that span multiple containers, extend `examples/poc/demo/main.go`.

## CI/CD

Run before committing:

```bash
make test
make validate
```

Both targets must pass for changes to be considered valid.