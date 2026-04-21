/**
 * Gassy Agent - Container-based agent that runs in a multi-agent system.
 *
 * Supports two roles:
 * - engineer: Handles coding, testing, and build tasks delegated by the Mayor
 * - mayor: Orchestrates tasks, answers questions, delegates to engineer when needed
 *
 * Uses the Anthropic Agent SDK for API calls.
 */

import { unstable_v2_prompt, unstable_v2_createSession, unstable_v2_resumeSession, type SDKSessionOptions } from "@anthropic-ai/claude-agent-sdk";
import Fastify, { FastifyInstance } from "fastify";
import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { readFileSync, writeFileSync, existsSync, mkdirSync } from "fs";
import yaml from "yaml";
import fastifyStatic from "@fastify/static";

// =============================================================================
// System Prompts
// =============================================================================

const SYSTEM_PROMPTS = {
  engineer: `You are the Engineer agent in a multi-agent system called Gassy. The Mayor orchestrator delegates coding, testing, and build tasks to you via A2A. You have access to tools for reading, editing, and running files in your workspace.

IMPORTANT - Task Handoffs:
When you need to hand off work to another agent (like a designer), use the Bash tool to run the gassy CLI:
  /app/gassy delegate --skill <skill> <task description>

Examples:
- /app/gassy delegate --skill design "Create a logo for my project"
- /app/gassy delegate --skill writing "Write documentation for the API"

When given a task, complete it thoroughly and report back the results.`,

  mayor: `You are the Mayor — an orchestrator agent in a multi-agent system called Gassy. Your role is to coordinate the work between human requests and specialist agents.

IMPORTANT - Task Handoffs:
When you need to delegate work to the engineer or other agents, use the Bash tool to run the gassy CLI:
  /app/gassy delegate [agent-id] [prompt]  - Delegate to a specific agent
  /app/gassy delegate --skill <skill> [prompt] - Find agent by skill and delegate

Examples:
- /app/gassy delegate engineer "Write a web server in Go"
- /app/gassy delegate --skill design "Create a logo for my project"

When you receive a request, determine if it requires coding/testing/building. If so, delegate to the engineer agent using Bash. For questions, explanations, or coordination tasks, respond directly.

Available agents:
- engineer: Handles coding, testing, and build tasks`,
} as const;

// =============================================================================
// CLAUDE.md Generation
// =============================================================================

interface ManifestConfig {
  name: string;
  role: string;
  description: string;
  version?: string;
}

interface SkillDefinition {
  name: string;
  description: string;
  triggers?: string[];
}

interface SkillsConfig {
  name: string;
  role: string;
  description: string;
  system_prompt?: string;
  skills?: SkillDefinition[];
}

async function generateCLAUDEMd(agentRole: string, configDir: string): Promise<void> {
  const manifestPath = resolve(configDir, `${agentRole}/manifest.yaml`);
  const skillsPath = resolve(configDir, `${agentRole}/skills.yaml`);
  const systemMdPath = resolve(configDir, `${agentRole}/system.md`);
  const appDir = "/app";

  // Ensure /app directory exists
  if (!existsSync(appDir)) {
    mkdirSync(appDir, { recursive: true });
  }

  let agentName = agentRole;
  let agentDescription = "";
  let skills: SkillDefinition[] = [];
  let systemPrompt = "";

  // Read manifest.yaml for metadata
  if (existsSync(manifestPath)) {
    try {
      const manifestContent = readFileSync(manifestPath, "utf-8");
      const manifest = yaml.parse(manifestContent) as ManifestConfig;
      agentName = manifest.name || agentRole;
      agentDescription = manifest.description || "";
    } catch (error) {
      console.warn(`Failed to read manifest from ${manifestPath}: ${error}`);
    }
  }

  // Read skills.yaml for skills and optional system_prompt
  if (existsSync(skillsPath)) {
    try {
      const skillsContent = readFileSync(skillsPath, "utf-8");
      const skillsConfig = yaml.parse(skillsContent) as SkillsConfig;
      skills = skillsConfig.skills || [];
      // Use system_prompt from skills.yaml if system.md doesn't exist
      if (!existsSync(systemMdPath)) {
        systemPrompt = skillsConfig.system_prompt || "";
      }
    } catch (error) {
      console.warn(`Failed to read skills from ${skillsPath}: ${error}`);
    }
  }

  // Read system.md for system prompt (overrides skills.yaml system_prompt)
  if (existsSync(systemMdPath)) {
    try {
      systemPrompt = readFileSync(systemMdPath, "utf-8");
    } catch (error) {
      console.warn(`Failed to read system.md from ${systemMdPath}: ${error}`);
    }
  }

  // Build CLAUDE.md content
  let claudeMdContent = `# ${agentName} Agent\n\n`;
  claudeMdContent += `${agentDescription}\n\n`;

  if (skills.length > 0) {
    claudeMdContent += `## Skills\n\n`;
    for (const skill of skills) {
      claudeMdContent += `- ${skill.name}: ${skill.description}`;
      if (skill.triggers && skill.triggers.length > 0) {
        claudeMdContent += ` (triggers: ${skill.triggers.join(", ")})`;
      }
      claudeMdContent += "\n";
    }
    claudeMdContent += "\n";
  }

  if (systemPrompt) {
    claudeMdContent += `## System Prompt\n\n${systemPrompt}\n`;
  }

  // Always include basic workspace info
  claudeMdContent += `\n## Workspace\n\n`;
  claudeMdContent += `- Working directory: /app\n`;
  claudeMdContent += `- Claude Code executable: /app/gassy-agent\n`;

  writeFileSync(resolve(appDir, "CLAUDE.md"), claudeMdContent);
  console.log(`Generated /app/CLAUDE.md for ${agentRole} agent`);
}

// =============================================================================
// A2A Types (Agent-to-Agent Protocol)
// =============================================================================

interface AgentCard {
  name: string;
  version: string;
  capability: {
    agent: true;
  };
  provider: {
    organization: string;
    url: string;
  };
  skills: Array<{
    name: string;
    description: string;
  }>;
}

interface A2AMessage {
  type: "message";
  id: string;
  method: string;
  params: {
    message: string;
    sessionId?: string;
    context?: Record<string, unknown>;
  };
}

// =============================================================================
// A2A Server
// =============================================================================

async function createA2AServer(
  port: number,
  agentRole: string,
  session: Awaited<ReturnType<typeof unstable_v2_createSession>>,
  agentCard: AgentCard
): Promise<FastifyInstance> {
  const fastify = Fastify({
    logger: false,
  });

  // Ensure /app/artifacts directory exists for file serving
  const artifactsDir = "/app/artifacts";
  if (!existsSync(artifactsDir)) {
    mkdirSync(artifactsDir, { recursive: true });
  }

  // Register static file serving for /files/* route
  await fastify.register(fastifyStatic, {
    root: artifactsDir,
    prefix: "/files/",
    decorateReply: false,
  });

  const card: AgentCard = {
    name: agentRole,
    version: "0.1.0",
    capability: { agent: true },
    provider: {
      organization: "Gassy",
      url: `http://localhost:${port}`,
    },
    skills:
      agentRole === "engineer"
        ? [
            { name: "coding", description: "Write and edit code files" },
            { name: "testing", description: "Run tests and verify functionality" },
            { name: "building", description: "Build and compile projects" },
          ]
        : [
            { name: "orchestration", description: "Coordinate tasks between agents" },
            { name: "delegation", description: "Delegate tasks to the engineer agent" },
            { name: "answering", description: "Answer questions directly" },
          ],
  };

  // GET /.well-known/agent.json - Agent card endpoint
  fastify.get("/.well-known/agent.json", async () => {
    return card;
  });

  // GET /.well-known/agent (alt path) - Same card
  fastify.get("/.well-known/agent", async () => {
    return card;
  });

  // GET /health - Health check endpoint for supervisor
  fastify.get("/health", async () => {
    return { status: "ok", role: agentRole };
  });

  // POST /webhook - Receive webhook callbacks from external services
  // This allows the agent to receive push notifications from external systems
  fastify.post("/webhook", async (request, reply) => {
    const body = request.body as any;

    console.log(`[${agentRole}] Received webhook:`, JSON.stringify(body).substring(0, 200));

    // Log the webhook event for debugging
    if (body && body.eventType) {
      console.log(`[${agentRole}] Webhook event type: ${body.eventType}`);
    }

    if (body && body.data) {
      console.log(`[${agentRole}] Webhook data:`, JSON.stringify(body.data).substring(0, 200));
    }

    // Return acknowledgment
    return {
      jsonrpc: "2.0",
      id: body?.id || null,
      result: { status: "received" },
    };
  });

  // POST /tools/delegate - Agent tool for delegating to other agents
  // This endpoint allows the agent's LLM to request delegation via tool call
  fastify.post("/tools/delegate", async (request, reply) => {
    const body = request.body as any;

    if (!body || !body.function || !body.params) {
      return {
        jsonrpc: "2.0",
        id: body?.id || "unknown",
        error: { code: -32600, message: "Missing function or params" },
      };
    }

    const { function: fn, params, id } = body;

    try {
      if (fn === "discover_agent_by_skill") {
        const { skill } = params;
        const result = await discoverAgentBySkill(process.env.SUPERVISOR_URL || "http://localhost:9091", skill, agentRole);
        return {
          jsonrpc: "2.0",
          id,
          result,
        };
      }

      if (fn === "delegate_to_agent") {
        const { agent_url, task, skill_hint } = params;
        const result = await delegateToAgent(agent_url, task, agentRole, skill_hint);
        return {
          jsonrpc: "2.0",
          id,
          result: { response: result },
        };
      }

      if (fn === "delegate_by_skill") {
        const { skill, task } = params;
        // First discover agent by skill
        const discovered = await discoverAgentBySkill(process.env.SUPERVISOR_URL || "http://localhost:9091", skill, agentRole);
        if (!discovered) {
          return {
            jsonrpc: "2.0",
            id,
            error: { code: -32000, message: `No agent found with skill: ${skill}` },
          };
        }
        // Then delegate to it
        const result = await delegateToAgent(discovered.a2aUrl, task, agentRole, skill);
        return {
          jsonrpc: "2.0",
          id,
          result: { response: result, agent_id: discovered.agentId, a2a_url: discovered.a2aUrl },
        };
      }

      return {
        jsonrpc: "2.0",
        id,
        error: { code: -32601, message: `Unknown function: ${fn}` },
      };
    } catch (error) {
      console.error(`[${agentRole}] Tool error:`, error);
      return {
        jsonrpc: "2.0",
        id,
        error: {
          code: -32000,
          message: error instanceof Error ? error.message : "Unknown error",
        },
      };
    }
  });

  // POST /a2a - A2A JSON-RPC endpoint
  fastify.post("/a2a", async (request, reply) => {
    const body = request.body as any;

    // Handle getTask separately - agent doesn't maintain task state
    if (body.method === "getTask") {
      const params = body.params as any;
      return {
        jsonrpc: "2.0",
        id: body.id,
        result: {
          id: params?.taskId || "unknown",
          state: "completed",
          status: { state: "completed" },
        },
      };
    }

    // Validate the message format
    if (!body.id || !body.method) {
      return {
        jsonrpc: "2.0",
        id: body?.id || "unknown",
        error: { code: -32600, message: "Missing id or method" },
      };
    }

    // Handle message, sendMessage, and sendStreamingMessage methods
    if (body.method !== "message" && body.method !== "sendMessage" && body.method !== "sendStreamingMessage") {
      return {
        jsonrpc: "2.0",
        id: body.id,
        error: { code: -32601, message: `Unknown method: ${body.method}` },
      };
    }

    const isStreaming = body.method === "sendStreamingMessage" || body.params?.stream === true;

    try {
      // Extract message from params - handle both formats
      let messageText: string;
      const params = body.params as any;

      if (typeof params.message === "string") {
        messageText = params.message;
      } else if (params.message && params.message.parts) {
        const parts = params.message.parts as Array<{type: string; text: string}>;
        messageText = parts.map((p) => p.text || "").join("");
      } else if (params.message && params.message.content) {
        messageText = params.message.content;
      } else {
        messageText = JSON.stringify(params.message || params);
      }

      const context = params.context;

      // Build conversation with context
      const systemPrompt = SYSTEM_PROMPTS[agentRole as keyof typeof SYSTEM_PROMPTS] || SYSTEM_PROMPTS.engineer;
      const contextStr = context ? `Previous context: ${JSON.stringify(context)}` : "";

      const fullMessage = systemPrompt
        ? `${systemPrompt}\n\n${contextStr ? contextStr + "\n\n" : ""}User message: ${messageText}`
        : `${contextStr ? contextStr + "\n\n" : ""}User message: ${messageText}`;

      console.log(`[${agentRole}] Received prompt: ${messageText.substring(0, 200)}${messageText.length > 200 ? "..." : ""}`);

      // For streaming, we use session.stream() to get chunked output
      if (isStreaming) {
        reply.raw?.writeHead(200, {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          "Connection": "keep-alive",
        });

        const sessionId = `session-${body.id}`;
        const taskId = `task-${body.id}`;

        // Send initial task event with working state
        const taskEvent = {
          kind: "task",
          task: {
            id: taskId,
            sessionId: sessionId,
            status: { state: "working" },
          },
        };
        reply.raw?.write(`data: ${JSON.stringify(taskEvent)}\n\n`);

        // Start the conversation
        await session.send(fullMessage);

        // Stream events as they come
        let finalMessage = "";
        let eventCount = 0;
        let hasContent = false;

        try {
          for await (const msg of session.stream()) {
            eventCount++;

            // Handle result message - contains the final response text
            if (msg.type === 'result' && 'result' in msg) {
              const resultMsg = msg as any;
              if (resultMsg.result) {
                finalMessage = resultMsg.result;
                console.log(`[${agentRole}] Got result: ${finalMessage.substring(0, 100)}...`);
              }
            }

            // SDKPartialAssistantMessage has type 'stream_event' and contains text deltas
            if (msg.type === 'stream_event' && 'event' in msg) {
              const streamEvent = (msg as any).event;

              // Handle content_block_delta with text
              if (streamEvent.type === 'content_block_delta') {
                const delta = streamEvent.delta;
                if (delta.type === 'text_delta' && delta.text) {
                  finalMessage += delta.text;
                  hasContent = true;
                  const textDeltaEvent = {
                    kind: "textDelta",
                    textDelta: delta.text,
                  };
                  reply.raw?.write(`data: ${JSON.stringify(textDeltaEvent)}\n\n`);
                }
              }

              // Handle message delta (completion info)
              if (streamEvent.type === 'message_delta') {
                // Could capture usage, stop_reason here if needed
              }
            }

            // Check if session is still active - when it's done, we'll get the result differently
            // The stream ends when the session completes
          }
        } catch (err) {
          console.error(`[${agentRole}] Stream error:`, err);
        }

        console.log(`[${agentRole}] Stream complete. Total events: ${eventCount}, final message length: ${finalMessage.length}`);

        // Send completion event with final status
        const doneEvent = {
          kind: "completion",
          completion: {
            id: taskId,
            sessionId: sessionId,
            message: finalMessage,
            status: { state: finalMessage ? "completed" : "failed" },
          },
        };
        reply.raw?.write(`data: ${JSON.stringify(doneEvent)}\n\n`);
        reply.raw?.write(`data: [done]\n\n`);
        reply.raw?.end();

        return null;
      }

      // Non-streaming response - send message then collect response via stream
      await session.send(fullMessage);

      let responseText = "";
      try {
        for await (const msg of session.stream()) {
          // Handle result message - contains the final response text
          if (msg.type === 'result' && 'result' in msg) {
            const resultMsg = msg as any;
            if (resultMsg.result) {
              responseText = resultMsg.result;
              console.log(`[${agentRole}] Got result: ${responseText.substring(0, 100)}...`);
            }
          }

          // Also capture text deltas for incremental response
          if (msg.type === 'stream_event' && 'event' in msg) {
            const streamEvent = (msg as any).event;
            if (streamEvent.type === 'content_block_delta') {
              const delta = streamEvent.delta;
              if (delta.type === 'text_delta' && delta.text) {
                responseText += delta.text;
              }
            }
          }
        }
      } catch (err) {
        console.error(`[${agentRole}] Stream error:`, err);
      }

      return {
        jsonrpc: "2.0",
        id: body.id,
        result: {
          id: `task-${body.id}`,
          state: "completed",
          status: { state: "completed" },
          message: { role: "agent", parts: [{ type: "text", text: responseText }] },
        },
      };
    } catch (error) {
      console.error("Agent error:", error);

      if (isStreaming) {
        reply.raw?.writeHead(200, {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
        });
        const errorEvent = {
          kind: "error",
          error: {
            code: -32000,
            message: error instanceof Error ? error.message : "Unknown error",
          },
        };
        reply.raw?.write(`data: ${JSON.stringify(errorEvent)}\n\n`);
        reply.raw?.write(`data: [done]\n\n`);
        reply.raw?.end();
        return null;
      }

      return {
        jsonrpc: "2.0",
        id: body.id,
        error: {
          code: -32000,
          message: error instanceof Error ? error.message : "Unknown error",
        },
      };
    }
  });

  await fastify.listen({ port, host: "0.0.0.0" });
  console.log(`A2A server running on port ${port}`);
  console.log(`Agent card: http://localhost:${port}/.well-known/agent.json`);

  return fastify;
}

// =============================================================================
// A2A Client - Agent-to-Agent Delegation
// =============================================================================

interface A2ADiscoveredAgent {
  agentId: string;
  a2aUrl: string;
}

// Discover an agent by skill via supervisor's discover endpoint
async function discoverAgentBySkill(supervisorUrl: string, skill: string, roleName: string): Promise<A2ADiscoveredAgent | null> {
  try {
    const response = await fetch(`${supervisorUrl}/registry/discover?skill=${encodeURIComponent(skill)}`);
    if (!response.ok) {
      console.warn(`[${roleName}] Discovery failed: ${response.status}`);
      return null;
    }
    const agents = await response.json() as Array<{agent_id: string; a2a_url: string}>;
    if (agents.length === 0) {
      console.warn(`[${roleName}] No agent found with skill: ${skill}`);
      return null;
    }
    return { agentId: agents[0].agent_id, a2aUrl: agents[0].a2a_url };
  } catch (error) {
    console.warn(`[${roleName}] Discovery error: ${error}`);
    return null;
  }
}

// Delegate a task to another agent via A2A streaming
async function delegateToAgent(
  agentUrl: string,
  taskMessage: string,
  roleName: string,
  skillHint?: string
): Promise<string> {
  console.log(`[${roleName}] Delegating to ${agentUrl}${skillHint ? ` (skill: ${skillHint})` : ""}`);

  // A2A streaming request
  const request = {
    jsonrpc: "2.0",
    id: Date.now(),
    method: "sendStreamingMessage",
    params: {
      message: {
        role: "user",
        parts: [{ type: "text", text: taskMessage }],
      },
      stream: true,
    },
  };

  try {
    const response = await fetch(`${agentUrl}/a2a`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "Accept": "text/event-stream" },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`A2A request failed: ${response.status}`);
    }

    // Read SSE stream and accumulate text deltas
    const reader = response.body?.getReader();
    if (!reader) {
      throw new Error("No response body");
    }

    const decoder = new TextDecoder();
    let buffer = "";
    let fullResponse = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (line.startsWith("data: ")) {
          const dataStr = line.slice(6);
          if (dataStr === "[done]") {
            return fullResponse;
          }
          try {
            const event = JSON.parse(dataStr);
            if (event.kind === "textDelta" && event.textDelta) {
              fullResponse += event.textDelta;
            }
          } catch {
            // Ignore parse errors for non-JSON events
          }
        }
      }
    }

    return fullResponse;
  } catch (error) {
    console.error(`[${roleName}] Delegation error:`, error);
    throw error;
  }
}

// =============================================================================
// Supervisor Registration
// =============================================================================

async function registerWithSupervisor(
  supervisorUrl: string,
  agentRole: string,
  port: number,
  skills: Array<{name: string, description: string}>
): Promise<void> {
  const cardUrl = `http://localhost:${port}/.well-known/agent.json`;
  const skillNames = skills.map(s => s.name);

  try {
    const response = await fetch(`${supervisorUrl}/agents`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name: agentRole,
        cardUrl: cardUrl,
        skills: skillNames,
      }),
    });

    if (response.ok) {
      console.log(`Registered with supervisor at ${supervisorUrl}`);
    } else {
      console.warn(`Failed to register with supervisor: ${response.status} ${response.statusText}`);
    }
  } catch (error) {
    console.warn(`Could not register with supervisor at ${supervisorUrl}: ${error}`);
  }
}

// =============================================================================
// Mayor Web UI (simple status page)
// =============================================================================

async function createMayorWebUI(port: number): Promise<FastifyInstance | null> {
  const uiPort = port + 1;

  const fastify = Fastify({
    logger: false,
  });

  fastify.get("/", async () => {
    return `
<!DOCTYPE html>
<html>
<head>
  <title>Gassy Mayor Agent</title>
  <style>
    body { font-family: system-ui; max-width: 800px; margin: 50px auto; padding: 20px; }
    h1 { color: #333; }
    .status { background: #e8f5e9; padding: 15px; border-radius: 8px; margin: 20px 0; }
    .info { color: #666; margin: 10px 0; }
    a { color: #1976d2; }
  </style>
</head>
<body>
  <h1>Gassy Mayor Agent</h1>
  <div class="status">
    <strong>Status:</strong> Running<br>
    <strong>Role:</strong> Mayor (Orchestrator)<br>
    <strong>A2A Port:</strong> ${port}
  </div>
  <div class="info">
    <p>The Mayor agent coordinates tasks between humans and the engineer agent.</p>
    <p>Use the A2A protocol to send messages to this agent at <code>/a2a</code>.</p>
    <p>Agent card: <a href="/.well-known/agent.json">/.well-known/agent.json</a></p>
  </div>
</body>
</html>
    `;
  });

  await fastify.listen({ port: uiPort, host: "0.0.0.0" });
  console.log(`Mayor web UI running on port ${uiPort}`);

  return fastify;
}

// =============================================================================
// Main
// =============================================================================

async function main() {
  const env = {
    AGENT_ROLE: process.env.AGENT_ROLE || "engineer",
    PORT: parseInt(process.env.PORT || "8082", 10),
    SUPERVISOR_URL: process.env.SUPERVISOR_URL || "http://localhost:9091",
    ANTHROPIC_AUTH_TOKEN: process.env.ANTHROPIC_AUTH_TOKEN || process.env.ANTHROPIC_API_KEY || process.env.MINIMAX_API_KEY || "",
    ANTHROPIC_BASE_URL: process.env.ANTHROPIC_BASE_URL || "",
    ANTHROPIC_MODEL: process.env.ANTHROPIC_MODEL || "MiniMax-M2.7",
  };

  if (!env.ANTHROPIC_AUTH_TOKEN) {
    console.error("ANTHROPIC_AUTH_TOKEN environment variable is required");
    process.exit(1);
  }

  if (env.AGENT_ROLE !== "engineer" && env.AGENT_ROLE !== "mayor") {
    console.error("AGENT_ROLE must be 'engineer' or 'mayor'");
    process.exit(1);
  }

  // Build environment variables for the SDK
  const sdkEnv: Record<string, string | undefined> = {
    ANTHROPIC_AUTH_TOKEN: env.ANTHROPIC_AUTH_TOKEN,
  };
  if (env.ANTHROPIC_BASE_URL) {
    sdkEnv.ANTHROPIC_BASE_URL = env.ANTHROPIC_BASE_URL;
  }
  if (env.ANTHROPIC_MODEL) {
    sdkEnv.ANTHROPIC_MODEL = env.ANTHROPIC_MODEL;
  }

  // Create session options for the agent SDK
  const __dirname = dirname(fileURLToPath(import.meta.url));
  const claudeBinaryPath = resolve(__dirname, "../node_modules/@anthropic-ai/claude-agent-sdk-linux-x64-musl/claude");

  console.log(`Starting Gassy agent...`);
  console.log(`Role: ${env.AGENT_ROLE}`);
  console.log(`Port: ${env.PORT}`);
  console.log(`Supervisor: ${env.SUPERVISOR_URL}`);
  console.log(`Model: ${env.ANTHROPIC_MODEL}`);
  console.log(`Base URL: ${env.ANTHROPIC_BASE_URL || "default"}`);

  // Create config dir path for CLAUDE.md generation
  // __dirname is /app/dist when running in container, config is at /app/config/{role}/
  // New structure: gassy/config/{role}/manifest.yaml, skills.yaml, system.md
  const configDir = resolve(__dirname, "..", "..", "config");

  // Generate CLAUDE.md from config before creating session
  await generateCLAUDEMd(env.AGENT_ROLE, configDir);

  // Create persistent session using unstable_v2_createSession
  const session = await unstable_v2_createSession({
    model: env.ANTHROPIC_MODEL,
    cwd: "/app",
    settingSources: ["project"],  // Enable CLAUDE.md loading
    permissionMode: "bypassPermissions",
    allowDangerouslySkipPermissions: true,
    env: sdkEnv,
  });

  console.log(`Session created for ${env.AGENT_ROLE} agent`);

  // Build agent card for registration and server
  const agentCard: AgentCard = {
    name: env.AGENT_ROLE,
    version: "0.1.0",
    capability: { agent: true },
    provider: {
      organization: "Gassy",
      url: `http://localhost:${env.PORT}`,
    },
    skills:
      env.AGENT_ROLE === "engineer"
        ? [
            { name: "coding", description: "Write and edit code files" },
            { name: "testing", description: "Run tests and verify functionality" },
            { name: "building", description: "Build and compile projects" },
          ]
        : [
            { name: "orchestration", description: "Coordinate tasks between agents" },
            { name: "delegation", description: "Delegate tasks to the engineer agent" },
            { name: "answering", description: "Answer questions directly" },
          ],
  };

  // Register with supervisor
  await registerWithSupervisor(env.SUPERVISOR_URL, env.AGENT_ROLE, env.PORT, agentCard.skills);

  // Start A2A server with the persistent session
  const server = await createA2AServer(env.PORT, env.AGENT_ROLE, session, agentCard);

  // Mayor gets a web UI on PORT+1 (disabled for now to avoid port conflicts)
  // if (env.AGENT_ROLE === "mayor") {
  //   await createMayorWebUI(env.PORT);
  // }

  console.log(`Agent "${env.AGENT_ROLE}" is ready and listening on port ${env.PORT}`);

  // Handle graceful shutdown
  const shutdown = async () => {
    console.log("Shutting down...");
    try {
      await session.close();
    } catch (error) {
      console.warn("Error closing session:", error);
    }
    await server.close();
    process.exit(0);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);
}

main().catch((error) => {
  console.error("Failed to start agent:", error);
  process.exit(1);
});