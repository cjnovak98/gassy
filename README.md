# Gassy

**A2A-native orchestration platform** — combines Gas City's orchestration model with the A2A protocol for standard, interoperable agent communication.

## Concept

Gassy runs a "city" of AI agents with Gas City's orchestration layer (declarative topology, supervisor/reconcile loop, budgets, governance) while agents communicate via the **A2A protocol** — the open standard for agent-to-agent interoperability.

The result: agents are swappable (any A2A-compliant agent works), communication is debuggable (standard JSON-RPC over HTTP), and the platform inherits the A2A ecosystem (150+ partners including LangChain, LlamaIndex, Salesforce, SAP).

## Prerequisites
- Go 1.21+
- Podman (for building agent container)
- Optional: Node.js (only if doing local TypeScript development)

## Quick Start

```bash
git clone https://github.com/cjnovak98/gassy
cd gassy
make        # Build Go CLI + agent container image + install to PATH
make validate  # Run end-to-end validation
```

## Architecture

See [PLAN.md](PLAN.md) for the full architecture, milestones, and technical design.

## Project Status

Early development — see [PLAN.md](PLAN.md) for the 5-phase build plan.
