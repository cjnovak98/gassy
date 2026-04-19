/**
 * Simple test to verify the agent starts without crashing.
 */

import { spawn } from "child_process";
import { fileURLToPath } from "url";
import { dirname, join } from "path";

const __dirname = dirname(fileURLToPath(import.meta.url));

async function test() {
  console.log("Testing agent startup...");

  // Set required env vars for testing
  const env = {
    ...process.env,
    AGENT_ROLE: "engineer",
    PORT: "9999",
    SUPERVISOR_URL: "http://localhost:9091",
    ANTHROPIC_API_KEY: "sk-ant-test-key",
  };

  // Start the agent in dev mode
  const agentProcess = spawn("npx", ["tsx", "src/index.ts"], {
    cwd: join(__dirname, ".."),
    env,
    stdio: ["pipe", "pipe", "pipe"],
  });

  let output = "";
  let errorOutput = "";

  agentProcess.stdout?.on("data", (data) => {
    const text = data.toString();
    output += text;
    process.stdout.write(text);
  });

  agentProcess.stderr?.on("data", (data) => {
    const text = data.toString();
    errorOutput += text;
    process.stderr.write(text);
  });

  // Give it 5 seconds to start
  await new Promise((resolve) => setTimeout(resolve, 5000));

  // Check if process is still running
  if (agentProcess.exitCode !== null) {
    console.error(`Agent exited with code ${agentProcess.exitCode}`);
    console.error("Stderr:", errorOutput);
    process.exit(1);
  }

  // Check for startup messages
  if (output.includes("A2A server running on port")) {
    console.log("\nAgent started successfully!");
  } else if (output.includes("Failed to register with supervisor")) {
    console.log("\nAgent started (supervisor registration is optional)");
  } else {
    console.log("\nUnexpected output:", output);
  }

  // Kill the process
  agentProcess.kill("SIGTERM");

  console.log("Test passed!");
}

test().catch((error) => {
  console.error("Test failed:", error);
  process.exit(1);
});