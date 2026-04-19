# Gassy Agent

Container-based agent for the Gassy multi-agent system.

## Roles

- **engineer**: Handles coding, testing, and build tasks delegated by the Mayor
- **mayor**: Orchestrates tasks, answers questions, delegates to engineer when needed

## Quick Start

The agent runs in a container, managed by `gassy-admin`. Build and start via:

```bash
cd gassy
make build-agent    # Build container image
gassy-admin start   # Start all gassy containers
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_ROLE` | `engineer` | Agent role: `engineer` or `mayor` |
| `PORT` | `8084` | Port for the A2A server (engineer) |
| `SUPERVISOR_URL` | `http://localhost:9091` | Supervisor URL for registration |
| `ANTHROPIC_API_KEY` | (required) | Anthropic API key |

## A2A Protocol

The agent implements the Agent-to-Agent (A2A) protocol:

- **Agent Card**: `GET /.well-known/agent.json`
- **Message Endpoint**: `POST /a2a`
- **Streaming**: `GET /a2a/stream`

### Send a message

```bash
curl -X POST http://localhost:8084/a2a \
  -H "Content-Type: application/json" \
  -d '{
    "type": "message",
    "id": "msg-1",
    "method": "message",
    "params": {
      "message": "Hello, what can you do?"
    }
  }'
```

### Response format

```json
{
  "type": "message",
  "id": "msg-1",
  "result": {
    "message": "I am the Engineer agent...",
    "sessionId": "session-msg-1"
  }
}
```

## Architecture

```
                    +------------------+
                    |   Supervisor     |
                    +--------+---------+
                             |
              +--------------+---------------+
              |                              |
              v                              v
    +---------+---------+          +---------+---------+
    |   Engineer Agent   |          |    Mayor Agent   |
    |   (PORT 8084)      |          |   (PORT 8081)     |
    +---------+---------+          +---------+---------+
              ^                              ^
              |                              |
              +--------------+---------------+
                             |
                    A2A (JSON-RPC)
```