# Gassy: A2A-Native Orchestration Platform

## TL;DR

**Gassy** combines Gas City's orchestration model (city.toml, supervisor/reconcile loop, work tracking, budgets, governance) with the **A2A protocol** (Agent Cards, JSON-RPC over HTTP, SSE streaming, task lifecycle) to create an orchestration platform where agents are swappable, communication is debuggable, and everything interops via an open standard.

## Why Gassy Exists

| | Gas City | Paperclip | **Gassy** |
|---|---|---|---|
| Agent communication | Proprietary exec script | Proprietary | **Standard A2A JSON-RPC** |
| Agent discovery | Static city.toml | Static config | **Dynamic via Agent Cards** |
| Cross-framework agents | No | No | **Yes — any A2A-compliant agent** |
| Streaming updates | No | No | **Yes — SSE** |
| Task history | Beads ticket log | Proprietary | **Full A2A task.history** |
| Push notifications | No | No | **Yes — webhooks** |
| Interop | Proprietary lock-in | Proprietary lock-in | **Open standard (150+ A2A partners)** |
| Governance/cost control | Yes | Yes | **Yes + A2A task lifecycle** |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Gassy City                             │
│                                                             │
│  ┌─────────────┐                    ┌─────────────────┐   │
│  │   Mayor     │◄─────── hires ────►│  Supervisor     │   │
│  │ (CEO-tier   │         /          │  (Registry +    │   │
│  │  orchestr.) │        fire        │   Resource      │   │
│  └──────┬──────┘                    │   Manager)      │   │
│         │                           └────────┬────────┘   │
│         │ delegate                          │              │
│         │ A2A                               │ registry     │
│         ▼                                   ▼              │
│  ┌─────────────┐   A2A    ┌─────────────┐  ┌──────────┐  │
│  │  Director   │◄────────►│  Engineer   │  │ Engineer │  │
│  │ (reports    │          │  (A2A       │  │(dynamic) │  │
│  │  to Mayor)  │          │   server)   │  └──────────┘  │
│  └─────────────┘          └──────┬──────┘                │
│                                   │                        │
│                                   │ A2A Agent Card         │
│                                   │ /.well-known/          │
│                                   │  agent.json             │
│                                   ▼                        │
│  ┌──────────────────────────────────────────────┐          │
│  │  Beads Store (work tracking, budgets, audit)  │          │
│  └──────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

**Key insight**: `city.toml` is a bootstrap hint, not a enforced desired state.
The Mayor and Directors exercise agency — they hire, fire, and direct agents
based on work needs and budget constraints. The Supervisor provides the
registry and resource management infrastructure that makes this possible.

## Core Abstractions

### 1. City Config (`city.toml`)

**Bootstrap hint, not source of truth.** The city.toml seeds the initial agent
topology — typically Mayor + 1 Engineer, or Mayor + 3 Directors + 6 Engineers.
After bootstrap, the Mayor and Directors exercise agency to hire/fire agents
based on work demands and budget.

```toml
[city]
name = "bright-lights"
version = "1"

[[agents]]
id = "mayor"           # orchestrator (A2A client)
role = "ceo"
budget = { monthly = 60.0 }

[[agents]]
id = "engineer"
role = "cto"
skills = ["code", "test", "review"]
budget = { monthly = 50.0 }

[[agents]]
id = "designer"
role = "cmo"
skills = ["design", "ui", "assets"]
budget = { monthly = 40.0 }

[runtime]
port_range = { min = 8080, max = 9000 }
agent_image = "localhost:5000/gassy/agent:latest"
heartbeat_interval = "4h"
startup_timeout = "30s"
```

**No ports in config.** Agents don't specify ports — the Supervisor dynamically
allocates from `port_range` when hiring. The `agent_image` is the single container
image used for all agents.

### 2. A2A Agent Card

Every agent exposes a card at `/.well-known/agent.json`:

```json
{
  "name": "engineer",
  "version": "1.0",
  "url": "http://localhost:8002",
  "capabilities": {
    "streaming": true,
    "pushNotifications": false,
    "extendedAgentCard": false
  },
  "skills": [
    { "id": "code", "name": "Write Code", "description": "Writes clean, tested code" },
    { "id": "test", "name": "Test Code", "description": "Writes unit and integration tests" },
    { "id": "review", "name": "Review Code", "description": "Reviews PRs and provides feedback" }
  ],
  "securitySchemes": {
    "Bearer": { "type": "http", "scheme": "bearer" }
  },
  "defaultStream": true
}
```

### 3. A2A Server Middleware

Each agent runs as an A2A server via lightweight middleware:

```python
# gassy/server.py — wraps any agent with A2A endpoints
from a2a_server import A2AServer, AgentCardMiddleware
from starlette.applications import Starlette
from starlette.routing import Route

server = A2AServer(
    agent_id="engineer",
    agent_url="http://localhost:8002",
    skills=[...],
    handle_message=agent_dispatch,  # your agent's message handler
)

app = Starlette(routes=[
    Route("/.well-known/agent.json", endpoint=server.agent_card_endpoint),
    Route("/a2a", endpoint=server.a2a_endpoint, methods=["POST"]),
])
```

### 4. Supervisor (Registry + Resource Manager + Reconcile Loop)

The Supervisor is the infrastructure backbone of a Gassy city. It combines three concerns:

**Registry**: All agents register with the Supervisor at startup. Agents
discover each other by querying the Supervisor's registry (via `GET /list`).
This replaces static URL configuration — agents don't hardcode peer URLs.

**Resource Manager**: The Supervisor manages compute resources and budgets.
It tracks which agents are running, their resource usage, and enforces
monthly budget caps. It provides the `hire` and `fire` operations that
authorized agents (Mayor, Directors) use to scale the city.

**Reconcile Loop**: For bootstrap and disaster recovery, the Supervisor
can ensure a minimum baseline of agents is running (e.g., if Mayor dies,
restart it). But this is NOT enforcing city.toml as desired state — it's
infrastructure-level housekeeping.

**Port Allocation**: The Supervisor manages a port range (`runtime.port_range`).
When hiring an agent, it scans for the first available port and allocates it.
Agents register back with their actual `a2a_url` so peers can communicate directly.
No port configuration needed in city.toml — the Supervisor handles all allocation.

```go
// Supervisor types (cmd/supervisor/main.go)
type Supervisor struct {
    city        CityConfig        // bootstrap seed, not enforced
    beads       BeadsStore        // work tracking, budgets
    registry    map[string]Agent  // agent_id → Agent registration
    portRange   PortRange         // min/max ports for allocation
    usedPorts   map[int]bool      // currently allocated ports
    a2a_clients map[string]A2AClient
}

type PortRange struct {
    Min int
    Max int
}

// Registry operations
func (s *Supervisor) Register(agent Agent) error      // agent registers a2a_url
func (s *Supervisor) Unregister(agentID string) error
func (s *Supervisor) ListAgents() []AgentCard
func (s *Supervisor) Discover(skill string) (Agent, error)  // find by skill

// Port allocation (internal)
func (s *Supervisor) allocatePort() (int, error)  // first free port in range
func (s *Supervisor) releasePort(port int)

// Resource management (called by Mayor/Directors)
func (s *Supervisor) Hire(config AgentConfig) error  // allocate port, start container
func (s *Supervisor) Fire(agentID string) error      // stop container, release port

// Reconcile (infrastructure housekeeping)
async func (s *Supervisor) reconcile()
```

**Registry API (HTTP endpoints on :9091):**

```http
# Agent self-registration (called by agent at startup)
POST /registry/register
Content-Type: application/json
{
  "agent_id": "engineer-1",
  "role": "engineer",
  "skills": ["code", "test"],
  "a2a_url": "http://localhost:8085"   # actual port, allocated by supervisor
}

# Discover agents by skill (called by Mayor/Directors)
GET /registry/discover?skill=code
→ { "agent_id": "engineer-1", "a2a_url": "http://localhost:8085" }

# List all registered agents
GET /registry/list
→ [{ "agent_id": "mayor", "role": "ceo", "a2a_url": "http://localhost:8081" }, ...]

# Unregister (called by agent on shutdown)
DELETE /registry/unregister/{agent_id}
```

### 5. Mayor (CEO-tier authority + A2A client)

The Mayor is the top-level decision maker. It has authority to:

- **Hire agents**: Call `Supervisor.Hire()` to bring new agents online
- **Fire agents**: Call `Supervisor.Fire()` to remove agents
- **Delegate work**: Send tasks via A2A to any registered agent
- **Discover agents**: Query Supervisor registry to find agents by skill

**Note on Directors**: In larger cities, the Mayor may appoint Director
agents (COO, CTO, CMO tiers) who have delegated hire/fire authority for
their domain. Directors report to the Mayor and operate within budget
allocations set by the Mayor.

```python
class MayorClient:
    def __init__(self, supervisor_url: str):
        self.supervisor = SupervisorClient(supervisor_url)

    async def hire_engineer(self, skills: list[str], budget: float) -> str:
        """Hire a new engineer agent. Returns agent_id."""
        config = AgentConfig(
            role="engineer",
            skills=skills,
            budget=Budget(monthly=budget),
        )
        agent_id = await self.supervisor.hire(config)
        return agent_id

    async def fire(self, agent_id: str) -> None:
        """Terminate an agent."""
        await self.supervisor.fire(agent_id)

    async def delegate(self, skill: str, prompt: str) -> Task:
        """Find best agent for skill and delegate via A2A."""
        # Discover from registry, not static config
        agent_card = await self.supervisor.discover(skill)
        client = A2AClient(agent_card.url)
        return await client.send_message(Message(parts=[TextPart(text=prompt)]))
```
```

## Directory Structure

```
gassy/
├── cmd/
│   ├── gassy/              # CLI (gc-equivalent)
│   │   ├── main.go
│   │   ├── city.go         # city.toml parsing
│   │   ├── delegate.go     # A2A task delegation
│   │   ├── agent.go        # Agent list/discover
│   │   └── supervisor.go   # Supervisor client (list/hire/fire)
│   └── gassy-admin/        # Container lifecycle CLI
│       ├── main.go
│       ├── start.go        # Container start
│       ├── stop.go         # Container stop
│       ├── status.go       # Container status
│       ├── logs.go          # Container logs
│       ├── ps.go           # List containers
│       └── restart.go      # Container restart
├── internal/
│   ├── a2a/
│   │   ├── client.go       # A2A client
│   │   ├── server.go       # A2A server middleware
│   │   ├── card.go         # Agent Card types
│   │   └── types.go        # A2A protocol types
│   ├── supervisor/        # Supervisor (runs in container)
│   │   └── main.go        # Registry + reconcile loop
│   ├── beads/
│   │   └── store.go        # Beads RPC client
│   ├── city/
│   │   └── city.go         # Shared city.toml parsing
├── agent/                  # Agent container image
│   ├── Dockerfile
│   └── src/                # TypeScript agent implementation
├── gassy.go                # Main library
├── go.mod
├── city.toml.example
├── PLAN.md
└── README.md
```

## CLI Commands

```bash
# Start the city (supervisor container + agents from city.toml)
gassy-admin start

# List all agents (from Supervisor registry)
gassy agent list

# Delegate work to an agent
gassy delegate engineer "Write a WebSocket handler"

# Supervisor management (client commands to containerized supervisor)
gassy supervisor list
gassy supervisor hire <name> <role> [skills...]
gassy supervisor fire <name>

# Stop the city
gassy-admin stop
```

**Key distinction**: `gassy rig add` (Phase 2) registers an agent with
the Supervisor at startup. `gassy supervisor hire` (organizational) is when a Mayor
or Director intentionally adds an agent to the workforce. Both use the
Supervisor's underlying hire mechanism, but they serve different purposes
(bootstrap vs. organizational action).

## Current Status

Phase 0 (Foundation), Phase 1 (Orchestration Core), and Phase 2 (Agent Integration) complete. The following are implemented:

### Phase 0 — Foundation ✅
- Project scaffold (Go module, cmd/, internal/)
- `city.toml` parser (`cmd/gassy/city.go`)
- A2A types (`internal/a2a/types.go`: AgentCard, Task, Message, Part)
- A2A client (`internal/a2a/client.go`: SendMessage, GetTask, streaming)
- Agent Card endpoint + server stub (`internal/a2a/server.go`)

### Phase 1 — Orchestration Core ✅
- Supervisor reconcile loop (`cmd/supervisor/main.go`)
- Agent lifecycle: hire, fire, health check, restart (`cmd/supervisor/main.go`)
- Supervisor runs in container (`localhost:5000/gassy/supervisor:latest`)
- `gassy-admin start` launches supervisor + reconciles agents from city.toml
- `gassy supervisor` CLI (list, hire, fire)
- `gassy delegate` for A2A task delegation

### Phase 2 — Agent Integration ✅
- A2A server middleware per agent (`examples/poc/engineer/main.go`, `examples/poc/mayor/main.go`)
- Agent self-registration via supervisor HTTP API (`:9091`)
- Mayor discovers agents from supervisor at startup
- Mayor discovers agents from supervisor at startup

### What's Running
- **Supervisor**: port 9091 (registry + resource manager + reconcile)
- **Agents**: dynamic ports allocated from `port_range` (default 8080-9000)
- `gassy-admin start` to launch supervisor container and reconcile agents from city.toml
- `gassy-admin stop` to stop all containers
- `gassy agent list` to list registered agents from Supervisor registry
- `gassy delegate <agent> "<message>"` to delegate work via A2A
- Ports are allocated automatically — no manual port management needed

### Authority Model
- **Mayor** (CEO-tier): Can hire/fire any agent, delegate work, query registry
- **Director** (COO/CTO/CMO-tier): Delegated authority within domain, reports to Mayor
- Authorization is currently prompt-driven; role-based checks come later

## What Makes Gassy Superior

1. **No proprietary lock-in**: Any A2A-compliant agent (LangChain agent, LlamaIndex agent, custom agent) can join a Gassy city. Gas City agents only work with Gas City. Paperclip agents only work with Paperclip.

2. **Debuggable communication**: A2A is JSON-RPC over HTTP. You can `curl` an agent, inspect the Agent Card, log the JSON-RPC traffic. Gas City's exec protocol is opaque shell scripts talking to tmux sessions.

3. **Dynamic discovery**: Agents register with the Supervisor's registry at startup. Mayor discovers agents via `GET /registry/discover?skill=code`. No hardcoded ports or URLs — the Supervisor handles all allocation.

4. **Standard task lifecycle**: Tasks have `working → input-required → completed/failed` states, streaming updates, artifact accumulation, and full history. This is built into A2A. Gas City has tickets (Beads) and sessions (tmux) separately with no unified view.

5. **Enterprise-ready**: A2A has 150+ partners including Salesforce, SAP, ServiceNow. Gassy inherits that ecosystem. Gas City and Paperclip are small open-source projects with no ecosystem leverage.

## Technical Debt Note

Gassy is designed to be a Go project. Gas City is the reference implementation in Go. The A2A SDK is primarily Python, but the protocol is language-agnostic (JSON-RPC + HTTP). The Go implementation will wrap the A2A spec directly (no SDK dependency needed — it's just HTTP + JSON).
