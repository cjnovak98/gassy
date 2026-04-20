# Gassy Deployment Documentation

## Overview

Gassy is a multi-agent A2A orchestration system. The supervisor reads `city.toml` to determine desired state and ensures running agents match that configuration.

## Architecture

| Service     | Port | Role                                      |
|-------------|------|-------------------------------------------|
| mayor       | 8081 | Orchestrator agent (A2A client)           |
| engineer    | 8084 | Coding agent (A2A server)                 |
| supervisor  | 9091 | Reconcile loop - reads city.toml          |
| registry    | 5000 | Podman registry for agent images          |

## Prerequisites

- Podman installed and running
- `.env` file in the project root with:
  ```
  ANTHROPIC_AUTH_TOKEN=<token>
  ANTHROPIC_BASE_URL=https://api.minimax.io/anthropic
  ANTHROPIC_MODEL=MiniMax-M2.7
  MINIMAX_API_KEY=<key>
  ```

## Quick Start

```bash
# Build everything
make build-all

# Deploy agent image to local registry
make build-agent
```

## Managing Services

All service management is handled by `gassy-admin` commands:

```bash
gassy-admin start   # Start all services (registry, supervisor, agents)
gassy-admin stop    # Stop all services
gassy-admin status  # Check running containers
gassy-admin logs    # View logs
```

## Port Configuration Note

The engineer runs on port **8084** (not 8082). Port 8082 may have a zombie socket from a previous deployment that persists until kernel cleanup or podman restart.

## Verification

Check agent cards:
```bash
curl http://localhost:8081/.well-known/agent.json
curl http://localhost:8084/.well-known/agent.json
```

Check supervisor registrations:
```bash
curl http://localhost:9091/list
```

Check running containers:
```bash
podman ps -a
```

## Files

- `city.toml` - Agent topology configuration
- `.env` - Environment variables (in project root)

## Common Issues

### Containers not restarting

Ensure `--restart=unless-stopped` flag is used. Check logs:
```bash
podman logs gassy-mayor
podman logs gassy-engineer
```

### Engineer not registering

The supervisor reads `city.toml` for expected agents. If the engineer is missing from supervisor's registration list, verify the supervisor started with the correct `city.toml` path.