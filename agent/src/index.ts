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

// =============================================================================
// System Prompts
// =============================================================================

const SYSTEM_PROMPTS = {
  engineer: `You are the Engineer agent in a multi-agent system called Gassy. The Mayor orchestrator delegates coding, testing, and build tasks to you via A2A. You have access to tools for reading, editing, and running files in your workspace.

Available tools:
- /app/gassy agent list — List all available agents and their A2A URLs
- /app/gassy delegate [agent-id] [prompt] — Send a task to a specific agent
- /app/gassy delegate --skill [skill] [prompt] — Find an agent by skill and delegate

When given a task, complete it thoroughly and report back the results.`,

  mayor: `You are the Mayor — an orchestrator agent in a multi-agent system called Gassy. Your role is to coordinate the work between human requests and specialist agents.

How to delegate work:
1. Use /app/gassy agent list to see available agents, their URLs, and roles
2. Use /app/gassy delegate [agent-id] [prompt] to send a task to a specific agent
3. Or use /app/gassy delegate --skill [skill] [prompt] to find an agent by skill

When you receive a request, determine if it requires coding/testing/building. If so, delegate to the engineer agent. For questions, explanations, or coordination tasks, respond directly.

Available agents:
- engineer: Handles coding, testing, and build tasks`,
} as const;

// =============================================================================
// CLAUDE.md Generation
// =============================================================================

interface SkillConfig {
  name: string;
  role: string;
  description: string;
  system_prompt?: string;
  skills?: Array<{ name: string; description: string }>;
}

async function generateCLAUDEMd(agentRole: string): Promise<void> {
  const configPath = resolve(__dirname, `../../config/${agentRole}/skill.yaml`);
  const appDir = "/app";

  // Ensure /app directory exists
  if (!existsSync(appDir)) {
    mkdirSync(appDir, { recursive: true });
  }

  let claudeMdContent = "";

  if (existsSync(configPath)) {
    try {
      const fileContent = readFileSync(configPath, "utf-8");
      const config = yaml.parse(fileContent) as SkillConfig;

      // Build CLAUDE.md from config
      claudeMdContent = `# ${config.name} Agent\n\n`;
      claudeMdContent += `${config.description}\n\n`;

      if (config.skills && config.skills.length > 0) {
        claudeMdContent += `## Skills\n\n`;
        for (const skill of config.skills) {
          claudeMdContent += `- ${skill.name}: ${skill.description}\n`;
        }
        claudeMdContent += "\n";
      }

      if (config.system_prompt) {
        claudeMdContent += `## System Prompt\n\n${config.system_prompt}\n`;
      }
    } catch (error) {
      console.warn(`Failed to read config from ${configPath}: ${error}`);
    }
  }

  // Always include basic workspace info
  if (!claudeMdContent) {
    claudeMdContent = `# ${agentRole} Agent\n\n`;
    claudeMdContent += `Working directory: /app\n\n`;
  }

  // Add workspace instructions
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
  session: Awaited<ReturnType<typeof unstable_v2_createSession>>
): Promise<FastifyInstance> {
  const fastify = Fastify({
    logger: false,
  });

  const agentCard: AgentCard = {
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
    return agentCard;
  });

  // GET /.well-known/agent (alt path) - Same card
  fastify.get("/.well-known/agent", async () => {
    return agentCard;
  });

  // GET /health - Health check endpoint for supervisor
  fastify.get("/health", async () => {
    return { status: "ok", role: agentRole };
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

      // Call the agent SDK using persistent session
      const response = await session.send(fullMessage);

      console.log(`[${agentRole}] SDK response type: ${typeof response}`);

      // session.send() returns the result directly
      const responseText = typeof response === "string" ? response : JSON.stringify(response);

      // For streaming, send as SSE
      if (isStreaming) {
        reply.raw?.writeHead(200, {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          "Connection": "keep-alive",
        });

        const event = {
          kind: "textDelta",
          textDelta: responseText,
        };
        reply.raw?.write(`data: ${JSON.stringify(event)}\n\n`);

        const doneEvent = {
          kind: "completion",
          completion: {
            message: responseText,
            sessionId: `session-${body.id}`,
          },
        };
        reply.raw?.write(`data: ${JSON.stringify(doneEvent)}\n\n`);
        reply.raw?.write(`data: [done]\n\n`);
        reply.raw?.end();

        return null;
      }

      // Non-streaming response
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
// Supervisor Registration
// =============================================================================

async function registerWithSupervisor(
  supervisorUrl: string,
  agentRole: string,
  port: number
): Promise<void> {
  const cardUrl = `http://localhost:${port}/.well-known/agent.json`;

  try {
    const response = await fetch(`${supervisorUrl}/agents`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name: agentRole,
        cardURL: cardUrl,
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

  // Generate CLAUDE.md from config before creating session
  await generateCLAUDEMd(env.AGENT_ROLE);

  // Create persistent session using unstable_v2_createSession
  const session = await unstable_v2_createSession({
    model: env.ANTHROPIC_MODEL,
    cwd: "/app",
    settingSources: ["project"],  // Enable CLAUDE.md loading
    permissionMode: "bypassPermissions",
    allowDangerouslySkipPermissions: true,
    env: sdkEnv,
  });

  const sessionId = session.sessionId;
  console.log(`Session created: ${sessionId}`);

  // Register with supervisor
  await registerWithSupervisor(env.SUPERVISOR_URL, env.AGENT_ROLE, env.PORT);

  // Start A2A server with the persistent session
  const server = await createA2AServer(env.PORT, env.AGENT_ROLE, session);

  // Mayor gets a web UI on PORT+1
  if (env.AGENT_ROLE === "mayor") {
    await createMayorWebUI(env.PORT);
  }

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