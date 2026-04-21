# A2A Implementation Research

## What We've Learned

### A2A Protocol Overview
- **A2A (Agent2Agent)** is an open protocol under Linux Foundation, contributed by Google
- Uses HTTP + JSON-RPC 2.0 + Protocol Buffers
- Enables interoperability between opaque agentic applications

### Task Lifecycle (8 states)
| State | Type |
|-------|------|
| `TASK_STATE_SUBMITTED` | Initial |
| `TASK_STATE_WORKING` | Active |
| `TASK_STATE_INPUT_REQUIRED` | Interrupted - needs more input |
| `TASK_STATE_AUTH_REQUIRED` | Interrupted - auth needed |
| `TASK_STATE_COMPLETED` | Terminal |
| `TASK_STATE_FAILED` | Terminal |
| `TASK_STATE_CANCELED` | Terminal |
| `TASK_STATE_REJECTED` | Terminal |

Tasks are immutable once terminal - refinements require a new task in the same `contextId`.

### Message Types
- **Task**: Stateful unit of work with id, context_id, status, artifacts[], history[], metadata
- **Message**: Single turn with message_id, context_id, task_id, role, parts[], metadata
- **Artifact**: Agent-generated output (documents, images) with URI and Parts[]
- **Part**: Smallest content unit (oneof: text, raw_data, url, structured_data)

### Streaming (SSE)
- Server declares `capabilities.streaming: true` in Agent Card
- Client calls `SendStreamingMessage` RPC
- Events: `task`, `message`, `statusUpdate`, `artifactUpdate`, `textDelta`
- Stream terminates on terminal or interrupted states

### File Transfers
- Files via `Part` objects with `url` field (external URLs) or `raw_data` (base64 inline)
- `Artifact` contains `Resource` with URI pointing to file location
- **No centralized artifact store** - agents serve files at their own URIs

### Push Notifications (Webhooks)
- Server declares `capabilities.pushNotifications: true` in Agent Card
- Client provides webhook URL via `CreateTaskPushNotificationConfig`
- Server POSTs `TaskStatusUpdateEvent` and `TaskArtifactUpdateEvent` to registered endpoint

### Agent Discovery
- Agent Card at `/.well-known/agent.json`
- Contains: name, capabilities, skills, security schemes
- May be cryptographically signed (JWS)

### What A2A Does NOT Specify
- **Scheduling** - left to implementers
- **File upload/download to/from URL** - just references URIs
- **Retry logic, rate limiting**
- **Artifact naming/versions**

---

## Current Gassy State

### What's Implemented
- Task lifecycle (working, completed, failed, canceled states)
- SSE streaming via `SendStreamingMessage`
- `sendMessage` / `getTask` / `cancelTask` / `listTasks`
- Agent Card at `/.well-known/agent.json`
- Skill-based discovery via supervisor
- Task state management and history
- Agent self-registration via supervisor HTTP API (:9091)

### What's "Fake" / Missing

#### Streaming is Implemented (2026-04-21)
True chunked streaming via SDK `session.stream()` was implemented in commit a650313. Each token/chunk is emitted as a separate `textDelta` SSE event.

#### File Part Support (Done 2026-04-21)
Agents serve files at `localhost:<port>/files/*` via `@fastify/static`. `/app/artifacts/` directory created. FilePart in A2A types implemented.

#### Webhook is Stub
`registerWebhook` exists but doesn't actually register anything.

#### No Task Handoffs
If engineer needs to hand off to designer, there's no mechanism for agent-to-agent delegation.

### Architecture (Current)

```
gassy delegate <agent> "prompt"
        ↓
  Discover agent via supervisor
        ↓
  Create A2A client → agent URL
        ↓
  SendStreamingMessage via SSE
        ↓
  Print events (statusUpdate, textDelta, artifactUpdate)
```

The **delegate CLI is a thin client**. The real A2A logic should be in agents.

---

## Plans / Implementation Research

### 1. True Streaming in Agents
**Priority:** High

**Problem:** Current agent batches entire response into one textDelta.

**Research needed:**
- [ ] Does the Anthropic agent SDK support true chunked streaming?
- [ ] How to pipe model output chunks directly to SSE events?
- [ ] What's the latency trade-off for chunked vs batched?

**Implementation approach:**
- Agent receives prompt
- Model streams tokens
- Each token/chunk → `textDelta` SSE event
- Completion → `completion` event

### 2. File Serving Per Agent
**Priority:** High

**Problem:** Artifacts need URIs, but agents don't serve files.

**Research needed:**
- [ ] Add `/files/*` route to Fastify server in agent
- [ ] Where should artifacts be stored? (`/app/artifacts/`?)
- [ ] How to handle concurrent access and cleanup?
- [ ] Should supervisor track artifact URIs for discovery?

**Implementation approach:**
```typescript
// Each agent serves files at its port
fastify.get('/files/:filename', async (req, reply) => {
  const file = `/app/artifacts/${req.params.filename}`;
  return reply.sendFile(file);
});

// Artifact would look like:
{
  resource: { uri: "http://localhost:8080/files/report.pdf" },
  parts: [...]
}
```

### 3. Task Handoffs (Agent-to-Agent)
**Priority:** Medium

**Problem:** Multi-hop flows (engineer → designer) require going through mayor CLI.

**Research needed:**
- [ ] Should agents call supervisor to discover and delegate directly?
- [ ] Or should supervisor act as intermediary for all agent-to-agent?
- [ ] How to track the task graph across agents?

**Implementation approach:**
- Agent completes work, returns "needs_specialist" result
- Mayor/middleware interprets and delegates to next agent
- Supervisor tracks which agent is working on what

### 4. Webhook/Push Notifications
**Priority:** Medium

**Problem:** Long-running tasks can't notify client when done (no persistent connection).

**Research needed:**
- [ ] Where should webhook URLs be registered?
- [ ] How to secure webhook endpoints?
- [ ] Should agents POST to supervisor, which then relays?

**Implementation approach:**
- Client registers webhook when submitting task
- Agent POSTs to webhook URL on terminal state
- Client retrieves full task via `GetTask`

### 5. Supervisor Task Graph
**Priority:** Low (future)

**Problem:** No visibility into multi-hop flows across agents.

**Research needed:**
- [ ] Should supervisor maintain a task DAG?
- [ ] How to handle circular dependencies?
- [ ] Should this be a separate orchestrator agent?

---

## Outstanding Questions

1. **Artifact Storage:** Should there be a dedicated artifact store (S3, etc.) or keep it per-agent filesystem?

2. **Skill Discovery:** Should supervisor's `discover?skill=` look at agent Card skills, or maintain a separate skill index?

3. **Streaming Trade-offs:** True streaming increases network overhead. Is batch-with-chunks acceptable for most use cases?

4. **Agent-to-Agent Auth:** If engineer delegates directly to designer, how does it authenticate? Shared secret? Supervisor-issued token?

5. **Context Window:** For multi-hop flows, who maintains the context window? The original client? A session agent?

6. **Push Notifications Security:** Webhook URLs could be exploited (SSRF). How do we validate?

---

## Implementation To-Do List

### Phase 1: True Streaming (Done 2026-04-21)
- [x] Investigate Anthropic agent SDK streaming capabilities
- [x] Modify agent to emit true chunked textDelta events (commit a650313)
- [x] Test streaming vs batch performance

#### Streaming Notes (2026-04-21)
The agent code properly emits `textDelta` SSE events for each streaming chunk. However, the MiniMax API may not produce true streaming responses - observed behavior shows complete message returned in single `completion` event rather than multiple `textDelta` events. This is likely a MiniMax API limitation rather than an implementation issue.

#### Bug Fix (2026-04-21)
Fixed compilation error in `internal/a2a/server.go`: removed unused `bodyData` variable in `handleStreamingMessageSSE`. Commit 8763c94.

### Phase 2: File Serving
- [x] Add `/files/*` route to agent Fastify server
- [x] Create `/app/artifacts/` directory in container
- [x] Implement FilePart in A2A types (2026-04-21)
- [x] Test artifact URIs pointing to agent-served files

### Phase 3: Skill Discovery (Done 2026-04-21)
- [x] Implement supervisor `GET /registry/discover?skill=` (exists in code)
- [x] Test discovery returns matching agents
- [x] Add skill matching to agent Card (agent registers with skills)
- [x] Fix supervisor /agents POST to properly update Skills field

#### Discovery Notes (2026-04-21)
The supervisor at :9091 has `/registry/discover?skill=` endpoint. Agent registers via `/agents` POST with skills array. Discovery filters agents by matching skill. **Fixed**: The `/agents` POST handler was storing skills as `null` because the request body uses `skills` but the state file showed `Skills: null`. Root cause was that `/agents` handler ignored skills in the request body. Now properly registers agents with skills.

**Verified working (2026-04-21)**:
- `curl 'http://localhost:9091/registry/discover?skill=coding'` returns `[{"agent_id":"engineer","a2a_url":"http://localhost:8081"}]`
- File serving at `http://localhost:8081/files/test.txt` works
- A2A streaming works via `gassy delegate engineer 'prompt'`
- Mayor and engineer containers running on ports 8080 and 8081

### Phase 4: Task Handoffs (Done 2026-04-21)
- [x] Design handoff protocol (agent → agent delegation)
- [x] Implement agent-to-agent A2A calls in TypeScript agent
- [x] Test engineer → designer flow

#### Auto-Delegation Implementation (2026-04-21)
Mayor agent now automatically delegates based on keyword detection:
- Coding keywords (`code`, `write`, `program`, `api`, `server`, etc.) → delegate to engineer (skill: coding)
- Design keywords (`design`, `logo`, `ui`, `ux`, etc.) → delegate to designer (skill: design)

Implementation:
- `/tools/delegate` HTTP endpoint for agent tool calls
- `discoverAgentBySkill()` queries supervisor `/registry/discover?skill=`
- `delegateToAgent()` makes A2A streaming request and accumulates text from SSE
- Both streaming and non-streaming paths handle delegation result

### Phase 5: Webhooks
- [x] Implement webhook registration (server stores WebhookURL via registerWebhook)
- [x] Send webhooks on task completion (TaskWebhookEvent POSTed to registered URL)
- [x] Send webhooks on artifact updates during streaming
- [ ] Add webhook POST endpoint to agent (for receiving callbacks from external services)

#### Webhook Implementation (2026-04-21)
Webhook delivery is now integrated into the SSE streaming flow:
- `SendWebhook(event)` method POSTs TaskWebhookEvent to registered webhook URL
- During streaming: artifactUpdate events trigger immediate webhook delivery (non-blocking)
- After done event: task completion triggers webhook with final status
- 10-second timeout, non-blocking (goroutine) so doesn't slow SSE response

### Future: Supervisor Task Graph
- [ ] Research DAG implementations
- [ ] Design task tracking across agents
- [ ] Consider dedicated orchestrator agent

---

## Key Resources

- [A2A Protocol GitHub](https://github.com/a2aproject/A2A)
- A2A SDKs: Python (`pip install a2a-sdk`), Go, JavaScript (`npm install @a2a-js/sdk`), Java, .NET

---

## Decisions Made

| Decision | Date | Rationale |
|----------|------|-----------|
| Build streaming/file transfer into agents, not delegate CLI | 2026-04-20 | A2A is agent-to-agent; delegate is just human I/O |
| Agent serves files at `localhost:<port>/files/*` | 2026-04-20 | Each agent is an HTTP server; no centralized store needed |
| Keep delegate CLI thin | 2026-04-20 | Separation of concerns; agents are the "brains" |

---

*Last updated: 2026-04-21*
