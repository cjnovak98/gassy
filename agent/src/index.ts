/**
 * Gassy Agent - Container-based agent that runs in a multi-agent system.
 *
 * Supports two roles:
 * - engineer: Handles coding, testing, and build tasks delegated by the Mayor
 * - mayor: Orchestrates tasks, answers questions, delegates to engineer when needed
 *
 * Uses the Anthropic Agent SDK for API calls.
 */

import { unstable_v2_prompt, type SDKSessionOptions } from "@anthropic-ai/claude-agent-sdk";
import Fastify, { FastifyInstance } from "fastify";
import { dirname, resolve } from "path";
import { fileURLToPath } from "url";

// =============================================================================
// System Prompts
// =============================================================================

const SYSTEM_PROMPTS = {
  engineer: `You are the Engineer agent in a multi-agent system called Gassy. The Mayor orchestrator delegates coding, testing, and build tasks to you via A2A. You have access to tools for reading, editing, and running files in your workspace. When given a task, complete it thoroughly and report back the results.`,

  mayor: `You are the Mayor — an orchestrator agent in a multi-agent system called Gassy. Answer questions directly unless they require coding/testing, then delegate to an engineer agent via A2A. Your role is to coordinate the work between human requests and the engineer agent.`,
} as const;

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
  sessionOptions: SDKSessionOptions
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

    // Validate the message format
    if (!body.id || !body.method) {
      return {
        type: "error" as const,
        id: body?.id || "unknown",
        error: { code: "INVALID_REQUEST", message: "Missing id or method" },
      };
    }

    // Handle both "message" and "sendStreamingMessage" methods
    if (body.method !== "message" && body.method !== "sendStreamingMessage") {
      return {
        type: "error" as const,
        id: body.id,
        error: { code: "UNKNOWN_METHOD", message: `Unknown method: ${body.method}` },
      };
    }

    const isStreaming = body.method === "sendStreamingMessage" || body.params?.stream === true;

    try {
      // Extract message from params - handle both formats
      let messageText: string;
      const params = body.params as any;

      if (typeof params.message === "string") {
        // Simple string message
        messageText = params.message;
      } else if (params.message && params.message.parts) {
        // Parts format: [{type: "text", text: "..."}]
        const parts = params.message.parts as Array<{type: string; text: string}>;
        messageText = parts.map((p) => p.text || "").join("");
      } else if (params.message && params.message.content) {
        // Content format from some A2A clients
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

      // Call the agent SDK
      const response = await unstable_v2_prompt(fullMessage, sessionOptions);

      // Extract text from response
      let responseText = "No response";
      if (response.type === "result" && response.subtype === "success") {
        responseText = response.result;
      } else if (response.type === "result" && "error" in response) {
        responseText = `Error: ${JSON.stringify(response)}`;
      }

      // For streaming, send as SSE
      if (isStreaming) {
        reply.raw?.writeHead(200, {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          "Connection": "keep-alive",
        });

        // Send text delta events
        const event = {
          kind: "textDelta",
          textDelta: responseText,
        };
        reply.raw?.write(`data: ${JSON.stringify(event)}\n\n`);

        // Send completion event
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

      return {
        type: "message" as const,
        id: body.id,
        result: {
          message: responseText,
          sessionId: `session-${body.id}`,
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
            code: "AGENT_ERROR",
            message: error instanceof Error ? error.message : "Unknown error",
          },
        };
        reply.raw?.write(`data: ${JSON.stringify(errorEvent)}\n\n`);
        reply.raw?.write(`data: [done]\n\n`);
        reply.raw?.end();
        return null;
      }

      return {
        type: "error" as const,
        id: body.id,
        error: {
          code: "AGENT_ERROR",
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
  // Mayor gets a web UI on PORT+1
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
  // The SDK may pick the wrong platform binary, so we specify it explicitly
  const __dirname = dirname(fileURLToPath(import.meta.url));
  // Use musl variant for Alpine-based containers
  const claudeBinaryPath = resolve(__dirname, "../node_modules/@anthropic-ai/claude-agent-sdk-linux-x64-musl/claude");

  const sessionOptions: SDKSessionOptions = {
    model: env.ANTHROPIC_MODEL,
    env: sdkEnv,
    pathToClaudeCodeExecutable: claudeBinaryPath,
    // Container environment: no interactive user, so bypass permission prompts
    permissionMode: "bypassPermissions",
    allowDangerouslySkipPermissions: true,
  };

  console.log(`Starting Gassy agent...`);
  console.log(`Role: ${env.AGENT_ROLE}`);
  console.log(`Port: ${env.PORT}`);
  console.log(`Supervisor: ${env.SUPERVISOR_URL}`);
  console.log(`Model: ${env.ANTHROPIC_MODEL}`);
  console.log(`Base URL: ${env.ANTHROPIC_BASE_URL || "default"}`);

  // Register with supervisor
  await registerWithSupervisor(env.SUPERVISOR_URL, env.AGENT_ROLE, env.PORT);

  // Start A2A server
  const server = await createA2AServer(env.PORT, env.AGENT_ROLE, sessionOptions);

  // Mayor gets a web UI on PORT+1
  if (env.AGENT_ROLE === "mayor") {
    await createMayorWebUI(env.PORT);
  }

  console.log(`Agent "${env.AGENT_ROLE}" is ready and listening on port ${env.PORT}`);

  // Handle graceful shutdown
  const shutdown = async () => {
    console.log("Shutting down...");
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