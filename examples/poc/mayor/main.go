// Package main runs the mayor orchestrator agent.
// It exposes an A2A server (so tools/delegate can call it) and uses its own
// LLM "brain" with proper tool calling to decide how to respond or delegate.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/cjnovak98/gassy/internal/a2a"
	"github.com/cjnovak98/gassy/internal/beads"
)

// Environment variables:
//   ANTHROPIC_API_KEY    MiniMax API key (required)
//   ANTHROPIC_BASE_URL   Base URL for the API (default: https://api.minimax.io/anthropic)
//   ANTHROPIC_MODEL      Model name (default: MiniMax-M2.7)
//   ENGINEER_URL         Engineer A2A server URL (default: http://localhost:8082)
//   WEB_PORT             Web UI port (default: 8083)

const webUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"/>
<title>Mayor Agent</title>
<style>
  body { font-family: monospace; margin: 40px; background: #1e1e1e; color: #d4d4d4; }
  #output { white-space: pre-wrap; word-wrap: break-word; margin-bottom: 20px; padding: 10px;
            border: 1px solid #333; background: #111; min-height: 300px; max-height: 60vh; overflow-y: auto; }
  textarea { width: 100%; height: 60px; background: #111; color: #d4d4d4; border: 1px solid #333;
             padding: 8px; font-family: monospace; resize: vertical; box-sizing: border-box; }
  button { margin-top: 10px; padding: 8px 20px; background: #0e639c; color: #fff; border: none;
           cursor: pointer; font-family: monospace; }
  button:disabled { background: #333; cursor: not-allowed; }
  .status { color: #888; font-size: 0.85em; }
  h1 { color: #3794ff; }
</style>
</head>
<body>
<h1>Mayor Agent</h1>
<p>Type a prompt and press Enter or click Send. The response streams in real-time.</p>
<div id="output"></div>
<textarea id="prompt" placeholder="Ask the mayor anything..."></textarea><br/>
<button id="send" onclick="sendPrompt()">Send</button>
<button id="clear" onclick="clearOutput()">Clear</button>
<span class="status" id="status"></span>

<script>
const output = document.getElementById('output');
const prompt = document.getElementById('prompt');
const sendBtn = document.getElementById('send');
const status = document.getElementById('status');

const BASE = window.location.origin;

function clearOutput() {
  output.textContent = '';
}

function appendOutput(text) {
  output.textContent += text;
  output.scrollTop = output.scrollHeight;
}

async function sendPrompt() {
  const q = prompt.value.trim();
  if (!q) return;

  sendBtn.disabled = true;
  status.textContent = 'streaming...';
  appendOutput('\n[i] ' + q + '\n');

  try {
    const res = await fetch(BASE + '/web/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: q }),
    });

    if (!res.ok) {
      appendOutput('\n[!] Error: HTTP ' + res.status + '\n');
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop();

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        const data = line.slice(6).trim();
        if (!data || data === '[done]') continue;
        try {
          const ev = JSON.parse(data);
          if ((ev.kind === 'textDelta' || ev.kind === 'statusUpdate') && ev.textDelta) {
            appendOutput(ev.textDelta);
          }
        } catch (_) {}
      }
    }
    appendOutput('\n\n');
  } catch (e) {
    appendOutput('\n[!] ' + e.message + '\n');
  } finally {
    sendBtn.disabled = false;
    status.textContent = '';
    prompt.value = '';
    prompt.focus();
  }
}

prompt.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendPrompt();
  }
});
</script>
</body>
</html>`

// mayorSystemPrompt is the system prompt for the mayor's own LLM voice.
var mayorSystemPrompt = `You are the Mayor — an orchestrator agent in a multi-agent system.

When a user asks you something, you handle it yourself unless it clearly requires coding, building, or testing. You have access to tools that let you delegate work to a specialist Engineer agent.

**Your tools:**

1. **delegate_to_engineer(task)** — Use this when the user asks for something that requires coding, building, testing, or any technical implementation. When delegating, instruct the engineer to include the FULL code in its response (not truncated, not summarized). Pass the full task description along with instruction to return complete code.

2. **inspect_engineer()** — Use this to learn about the engineer's capabilities before delegating.

**When you respond:**
- Answer directly in your own voice for general questions
- Use delegate_to_engineer for technical tasks, then present the engineer's response in your own voice
- Never say "I will delegate" — actually call the tool to delegate, then share the result

Keep responses concise and helpful.`

// llmClient is the shared Anthropic client for mayor's own LLM calls.
var llmClient anthropic.Client

// modelName is the model to use for mayor's own reasoning.
var modelName = "MiniMax-M2.7"

// conversationHistory holds the message history for multi-turn conversations.
var conversationHistory []anthropic.MessageParam

// agentClients maps agent names to their A2A clients (populated from supervisor on startup)
var agentClients = make(map[string]*a2a.Client)

// defaultAgent is the agent name to delegate to by default
const defaultAgent = "engineer"

// supervisorURL is the supervisor's HTTP API URL
var supervisorURL = "http://localhost:9091"

func main() {
	ctx := context.Background()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	webPort := os.Getenv("WEB_PORT")
	if webPort == "" {
		webPort = "8083"
	}

	// --- Build Anthropic client for mayor's own reasoning ---
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	modelName = os.Getenv("ANTHROPIC_MODEL")
	if modelName == "" {
		modelName = "MiniMax-M2.7"
	}

	var opts []option.RequestOption
	opts = append(opts, option.WithAPIKey(apiKey))
	if baseURL == "" {
		baseURL = "https://api.minimax.io/anthropic"
	}
	opts = append(opts, option.WithBaseURL(baseURL))
	llmClient = anthropic.NewClient(opts...)

	// --- Setup Beads store ---
	store := beads.New("localhost:9090")

	// --- Discover agents from supervisor ---
	discoverAgents()

	// --- Build A2A server ---
	server := a2a.NewServer()

	// HandleMessage: forward to default agent and return response directly
	server.HandleMessage = func(msg a2a.Message) (*a2a.Task, error) {
		taskID := fmt.Sprintf("mayor-task-%d", time.Now().UnixNano())
		inputText := extractText(msg.Parts)

		ticket, err := store.CreateTicket(ctx, "mayor", fmt.Sprintf("Delegating to %s: %s", defaultAgent, inputText))
		if err != nil {
			log.Printf("warning: failed to create ticket: %v", err)
		}

		client, ok := agentClients[defaultAgent]
		if !ok {
			return nil, fmt.Errorf("no agent named %q found", defaultAgent)
		}

		task, err := client.SendMessage(ctx, a2a.SendMessageParams{
			Message: *a2a.NewMessage("user", inputText),
		})
		if err != nil {
			if ticket != nil {
				_ = store.UpdateTicketStatus(ctx, ticket.ID, "error")
			}
			return nil, fmt.Errorf("%s delegation: %w", defaultAgent, err)
		}

		if ticket != nil {
			_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")
		}

		task.ID = taskID
		task.State = a2a.TaskStateCompleted
		task.Status = &a2a.TaskStatus{
			State:     a2a.TaskStateCompleted,
			Timestamp: time.Now(),
		}

		return task, nil
	}

	// HandleStreamingMessage: forward default agent SSE events directly without mayor wrapping
	server.HandleStreamingMessage = func(msg a2a.Message) (<-chan a2a.TaskEvent, error) {
		inputText := extractText(msg.Parts)

		client, ok := agentClients[defaultAgent]
		if !ok {
			return nil, fmt.Errorf("no agent named %q found", defaultAgent)
		}

		streamChan, err := client.SendStreamingMessage(ctx, a2a.SendMessageParams{
			Message: *a2a.NewMessage("user", inputText),
		})
		if err != nil {
			return nil, err
		}

		out := make(chan a2a.TaskEvent, 100)
		go func() {
			defer close(out)
			for event := range streamChan {
				var te a2a.TaskEvent
				if err := json.Unmarshal([]byte(event.Data), &te); err == nil {
					out <- te
				}
			}
		}()
		return out, nil
	}

	// --- A2A HTTP server (port 8081) ---
	a2aMux := http.NewServeMux()

	card := &a2a.AgentCard{
		Name:    "mayor",
		Version: "1.0.0",
		Url:     fmt.Sprintf("http://localhost:%s", port),
		Capabilities: a2a.AgentCapabilities{
			Streaming:          true,
			PushNotifications:   false,
			ExtendedAgentCard:   false,
		},
		Skills: []a2a.AgentSkill{
			{ID: "orchestrate", Name: "Orchestrate", Description: "Orchestrate work across agents"},
		},
		DefaultStream: true,
	}

	a2aMux.HandleFunc("/a2a", server.HandleA2A())
	a2aMux.HandleFunc("/.well-known/agent.json", server.HandleAgentCard(card))

	a2aAddr := fmt.Sprintf(":%s", port)
	go func() {
		log.Printf("A2A server on %s", a2aAddr)
		if err := http.ListenAndServe(a2aAddr, a2aMux); err != nil && err != http.ErrServerClosed {
			log.Fatalf("A2A server error: %v", err)
		}
	}()

	// --- Web UI HTTP server (port 8083) ---
	webMux := http.NewServeMux()
	webMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		io.WriteString(w, webUIHTML)
	})

	webMux.HandleFunc("/web/prompt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Reset conversation history for this request
		conversationHistory = nil

		// Stream the response
		streamResponse(w, r.Context(), req.Message)

		fmt.Fprintf(w, "data: [done]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	webAddr := fmt.Sprintf("0.0.0.0:%s", webPort)
	fmt.Printf("Web UI: http://0.0.0.0:%s\n", webPort)
	fmt.Printf("A2A server: http://localhost:%s\n", port)
	log.Printf("Web UI listening on %s", webAddr)
	if err := http.ListenAndServe(webAddr, webMux); err != nil {
		log.Fatalf("web server error: %v", err)
	}
}

// buildTools creates the tool definitions for the mayor's LLM.
func buildTools() []anthropic.ToolUnionParam {
	delegateTool := anthropic.ToolParam{
		Name:        "delegate_to_engineer",
		Description: anthropic.String("Delegates a coding, build, or testing task to the engineer agent. Use this when the user asks for something that requires creating, writing, building, or testing code."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "The full task description to send to the engineer",
				},
			},
			Required: []string{"task"},
		},
	}

	inspectTool := anthropic.ToolParam{
		Name:        "inspect_engineer",
		Description: anthropic.String("Gets the engineer's agent card to understand their capabilities, skills, and how to interact with them."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
	}

	tools := make([]anthropic.ToolUnionParam, 2)
	tools[0] = anthropic.ToolUnionParam{OfTool: &delegateTool}
	tools[1] = anthropic.ToolUnionParam{OfTool: &inspectTool}
	return tools
}

// streamResponse handles the LLM conversation with tool calling and streams results to the web UI.
func streamResponse(w http.ResponseWriter, ctx context.Context, userMessage string) {
	// Add user message to conversation
	conversationHistory = append(conversationHistory, anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)))

	// Keep calling the LLM until no more tool calls
	for {
		// Build the messages list for this call
		// We need to pass the full conversation history
		messages := make([]anthropic.MessageParam, len(conversationHistory))
		copy(messages, conversationHistory)

		// Call the LLM with tools
		stream := llmClient.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(modelName),
			MaxTokens: 8192,
			System:    []anthropic.TextBlockParam{{Text: mayorSystemPrompt}},
			Messages:  messages,
			Tools:     buildTools(),
		})
		defer stream.Close()

		var accumulatedMessage anthropic.Message
		var toolCalls []toolUseInfo

		for stream.Next() {
			event := stream.Current()
			if err := accumulatedMessage.Accumulate(event); err != nil {
				log.Printf("[mayor] accumulate error: %v", err)
				continue
			}

			switch e := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				// Use AsToolUse() method on the union type to get ToolUseBlock
				toolUseBlock := e.ContentBlock.AsToolUse()
				toolCalls = append(toolCalls, toolUseInfo{
					ID:   toolUseBlock.ID,
					Name: toolUseBlock.Name,
				})
				emitSSE(w, "statusUpdate", fmt.Sprintf("[Calling %s...]", toolUseBlock.Name))

			case anthropic.ContentBlockDeltaEvent:
				// Stream text deltas to the web UI
				if e.Delta.Text != "" {
					emitSSE(w, "textDelta", e.Delta.Text)
				}

			case anthropic.MessageStopEvent:
				// Message complete
			}
		}

		if stream.Err() != nil {
			emitSSE(w, "statusUpdate", fmt.Sprintf("[Error: %v]", stream.Err()))
			break
		}

		// Check for tool calls in the accumulated message
		newToolCalls := extractToolCalls(accumulatedMessage)
		if len(newToolCalls) == 0 {
			// No more tool calls, we're done
			break
		}

		// Add the assistant's message to conversation history
		conversationHistory = append(conversationHistory, accumulatedMessage.ToParam())

		// Execute tool calls and collect results
		for _, tc := range newToolCalls {
			emitSSE(w, "statusUpdate", fmt.Sprintf("[%s completed, processing result...]", tc.Name))

			var result string
			switch tc.Name {
			case "delegate_to_engineer":
				result = executeDelegateToEngineer(ctx, tc.Input)
			case "inspect_engineer":
				result = executeInspectEngineer(ctx)
			default:
				result = fmt.Sprintf(`{"error": "unknown tool: %s"}`, tc.Name)
			}

			// Add tool result to conversation
			toolResult := anthropic.NewToolResultBlock(tc.ID, result, false)
			conversationHistory = append(conversationHistory, anthropic.NewUserMessage(toolResult))
		}
	}

	// Emit final newline
	emitSSE(w, "statusUpdate", "\n")
}

// toolUseInfo holds information about a tool call from the LLM.
type toolUseInfo struct {
	ID     string
	Name   string
	Input  string
}

// extractToolCalls extracts tool calls from an accumulated message.
func extractToolCalls(msg anthropic.Message) []toolUseInfo {
	var result []toolUseInfo
	for _, block := range msg.Content {
		// Use AsToolUse() method on ContentBlockUnion
		toolUseBlock := block.AsToolUse()
		if toolUseBlock.Name != "" {
			result = append(result, toolUseInfo{
				ID:    toolUseBlock.ID,
				Name:  toolUseBlock.Name,
				Input: string(toolUseBlock.JSON.Input.Raw()),
			})
		}
	}
	return result
}

// discoverAgents queries the supervisor for registered agents and creates A2A clients for each.
func discoverAgents() {
	supervisorURL = os.Getenv("SUPERVISOR_URL")
	if supervisorURL == "" {
		supervisorURL = "http://localhost:9091"
	}

	resp, err := http.Get(supervisorURL + "/agents")
	if err != nil {
		log.Printf("warning: failed to discover agents from supervisor: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("warning: supervisor returned status %d", resp.StatusCode)
		return
	}

	var result struct {
		Agents []struct {
			Name    string `json:"Name"`
			URL     string `json:"URL"`
			CardURL string `json:"CardURL"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("warning: failed to decode agent list: %v", err)
		return
	}

	for _, a := range result.Agents {
		if a.URL != "" {
			agentClients[a.Name] = a2a.NewClient(a.URL)
			log.Printf("discovered agent %q at %s", a.Name, a.URL)
		}
	}
}

// executeDelegate delegates to the default agent via A2A and returns the response.
func executeDelegateToEngineer(ctx context.Context, taskInput string) string {
	// Parse the task from the input JSON
	var input struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal([]byte(taskInput), &input); err != nil {
		return fmt.Sprintf(`{"error": "failed to parse task input: %v"}`, err)
	}

	client, ok := agentClients[defaultAgent]
	if !ok {
		return fmt.Sprintf(`{"error": "no agent named %q found — use 'gassy supervisor hire %s' to register one"}`, defaultAgent, defaultAgent)
	}

	// Call the agent via A2A streaming
	streamChan, err := client.SendStreamingMessage(ctx, a2a.SendMessageParams{
		Message: *a2a.NewMessage("user", input.Task),
	})
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to call %s: %v"}`, defaultAgent, err)
	}

	var response string
	for event := range streamChan {
		var te a2a.TaskEvent
		if err := json.Unmarshal([]byte(event.Data), &te); err == nil {
			if te.TextDelta != "" {
				response += te.TextDelta
			}
		}
	}

	if response == "" {
		return fmt.Sprintf(`{"response": "%s returned empty response"}`, defaultAgent)
	}

	// Compact the agent's response to a single line so SSE parsing doesn't drop content.
	compact, err := json.Marshal(map[string]string{"response": response})
	if err != nil {
		return fmt.Sprintf(`{"response": %s}`, escapeJSONString(response))
	}
	return string(compact)
}

// executeInspectAgent fetches the agent's card for the default agent.
func executeInspectEngineer(ctx context.Context) string {
	cardURL := supervisorURL + "/agents"
	resp, err := http.Get(cardURL)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to fetch agent list: %v"}`, err)
	}
	defer resp.Body.Close()

	var result struct {
		Agents []struct {
			Name    string `json:"Name"`
			URL     string `json:"URL"`
			CardURL string `json:"CardURL"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Sprintf(`{"error": "failed to decode agent list: %v"}`, err)
	}

	for _, a := range result.Agents {
		if a.Name == defaultAgent && a.CardURL != "" {
			card, err := a2a.FetchAgentCard(ctx, a.URL)
			if err != nil {
				return fmt.Sprintf(`{"error": "failed to fetch agent card: %v"}`, err)
			}
			cardJSON, _ := json.Marshal(card)
			return fmt.Sprintf(`{"agentCard": %s}`, string(cardJSON))
		}
	}

	return fmt.Sprintf(`{"error": "agent %q not found"}`, defaultAgent)
}

// escapeJSONString escapes a string for use in JSON.
func escapeJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// emitSSE emits a Server-Sent Event to the web UI.
func emitSSE(w io.Writer, kind, text string) {
	te := a2a.TaskEvent{Kind: kind}
	if kind == "textDelta" || kind == "statusUpdate" {
		te.TextDelta = text
	}
	data, _ := json.Marshal(te)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// extractText extracts the first text string from message parts.
func extractText(parts []a2a.Part) string {
	for _, part := range parts {
		switch p := part.(type) {
		case a2a.TextPart:
			return p.Text
		case map[string]interface{}:
			if text, ok := p["text"].(string); ok {
				return text
			}
		}
	}
	return ""
}

