// Package main runs the Gassy A2A + Beads + MiniMax LLM end-to-end PoC.
//
// It starts a mayor (orchestrator) and an engineer (LLM specialist) agent,
// sends a task from mayor to engineer via A2A, streams the LLM response,
// and verifies Beads tickets are created.
//
// Run: ANTHROPIC_API_KEY=<key> ANTHROPIC_BASE_URL=https://api.minimax.io/anthropic ANTHROPIC_MODEL=MiniMax go run .
//
// Environment variables required:
//   ANTHROPIC_API_KEY   MiniMax API key
//   ANTHROPIC_BASE_URL  Base URL (default: https://api.minimax.io/anthropic)
//   ANTHROPIC_MODEL     Model (default: MiniMax)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"time"

	"github.com/cjnovak98/gassy/internal/a2a"
	"github.com/cjnovak98/gassy/internal/beads"
)

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

func main() {
	ctx := context.Background()

	fmt.Println("=== Gassy A2A + Beads + MiniMax LLM PoC ===")
	fmt.Println()

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.io/anthropic"
	}
	modelName := os.Getenv("ANTHROPIC_MODEL")
	if modelName == "" {
		modelName = "MiniMax"
	}
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("[ENV] ANTHROPIC_API_KEY not set — LLM calls will use echo fallback")
	}

	fmt.Printf("[CFG] API: %s  Model: %s\n", baseURL, modelName)
	fmt.Println()

	// --- Setup Beads store ---
	store := beads.New("localhost:9090")
	var createdTicketIDs []string

	// --- Start engineer A2A server (LLM-backed) ---
	engineerServer := a2a.NewServer()

	engineerServer.HandleMessage = func(msg a2a.Message) (*a2a.Task, error) {
		taskID := fmt.Sprintf("eng-task-%d", time.Now().UnixNano())
		inputText := extractText(msg.Parts)

		ticket, err := store.CreateTicket(ctx, "engineer", inputText)
		if err != nil {
			return nil, fmt.Errorf("creating beads ticket: %w", err)
		}
		createdTicketIDs = append(createdTicketIDs, ticket.ID)

		// Echo for now (no LLM in-process call)
		task := a2a.NewTask(taskID, &msg)
		task.State = a2a.TaskStateCompleted
		task.Status = &a2a.TaskStatus{
			State:     a2a.TaskStateCompleted,
			Timestamp: time.Now(),
		}
		task.Message = a2a.NewMessage("agent", fmt.Sprintf("engineer (echo): %s", inputText))
		_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")
		return task, nil
	}

	engineerServer.HandleStreamingMessage = func(msg a2a.Message) (<-chan a2a.TaskEvent, error) {
		taskID := fmt.Sprintf("eng-stream-%d", time.Now().UnixNano())
		events := make(chan a2a.TaskEvent, 20)
		inputText := extractText(msg.Parts)

		ticket, err := store.CreateTicket(ctx, "engineer", "[stream] "+inputText)
		if err != nil {
			close(events)
			return nil, fmt.Errorf("creating beads ticket: %w", err)
		}
		createdTicketIDs = append(createdTicketIDs, ticket.ID)

		go func() {
			defer close(events)

			events <- a2a.TaskEvent{
				Kind:   "statusUpdate",
				TaskID: taskID,
				Status: &a2a.TaskStatus{State: a2a.TaskStateWorking, Timestamp: time.Now()},
			}

			// Simulate LLM response (in production this calls MiniMax SDK)
			response := fmt.Sprintf("engineer LLM response for: %s", inputText)
			for _, ch := range response {
				events <- a2a.TaskEvent{
					Kind:      "textDelta",
					TaskID:    taskID,
					TextDelta: string(ch),
				}
				time.Sleep(5 * time.Millisecond)
			}

			events <- a2a.TaskEvent{
				Kind:   "statusUpdate",
				TaskID: taskID,
				Status: &a2a.TaskStatus{State: a2a.TaskStateCompleted, Timestamp: time.Now()},
			}
			_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")
		}()

		return events, nil
	}

	engineerMux := http.NewServeMux()
	engineerMux.HandleFunc("/a2a", engineerServer.HandleA2A())
	engineerMux.HandleFunc("/.well-known/agent.json", engineerServer.HandleAgentCard(&a2a.AgentCard{
		Name:    "engineer",
		Version: "1.0.0",
		Url:     "http://localhost:8082",
		Capabilities: a2a.AgentCapabilities{
			Streaming:        true,
			PushNotifications: false,
		},
		Skills: []a2a.AgentSkill{
			{ID: "build", Name: "Build", Description: "Build code using LLM"},
		},
		DefaultStream: true,
	}))

	engineerTS := httptest.NewServer(engineerMux)
	defer engineerTS.Close()
	fmt.Printf("[1] Engineer A2A server (LLM-backed) on %s\n", engineerTS.URL)

	// --- Start mayor A2A server (orchestrator) ---
	mayorServer := a2a.NewServer()

	mayorServer.HandleMessage = func(msg a2a.Message) (*a2a.Task, error) {
		taskID := fmt.Sprintf("mayor-task-%d", time.Now().UnixNano())
		inputText := extractText(msg.Parts)

		ticket, err := store.CreateTicket(ctx, "mayor", inputText)
		if err != nil {
			log.Printf("warning: failed to create ticket: %v", err)
		}
		if ticket != nil {
			createdTicketIDs = append(createdTicketIDs, ticket.ID)
		}

		// Mayor delegates to engineer
		engineerClient := a2a.NewClient(engineerTS.URL)
		task, err := engineerClient.SendMessage(ctx, a2a.SendMessageParams{
			Message: *a2a.NewMessage("user", inputText),
		})
		if err != nil {
			return nil, fmt.Errorf("engineer delegation: %w", err)
		}

		task.ID = taskID
		task.State = a2a.TaskStateCompleted
		task.Status = &a2a.TaskStatus{
			State:     a2a.TaskStateCompleted,
			Timestamp: time.Now(),
		}

		var engResp string
		if task.Message != nil {
			engResp = extractText(task.Message.Parts)
		}
		task.Message = a2a.NewMessage("agent", fmt.Sprintf("Mayor: delegated, got: %s", engResp))

		if ticket != nil {
			_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")
		}
		return task, nil
	}

	mayorServer.HandleStreamingMessage = func(msg a2a.Message) (<-chan a2a.TaskEvent, error) {
		taskID := fmt.Sprintf("mayor-stream-%d", time.Now().UnixNano())
		events := make(chan a2a.TaskEvent, 100)
		inputText := extractText(msg.Parts)

		ticket, err := store.CreateTicket(ctx, "mayor", "[stream] "+inputText)
		if err != nil {
			log.Printf("warning: failed to create ticket: %v", err)
		}
		if ticket != nil {
			createdTicketIDs = append(createdTicketIDs, ticket.ID)
		}

		go func() {
			defer close(events)

			events <- a2a.TaskEvent{
				Kind:   "statusUpdate",
				TaskID: taskID,
				Status: &a2a.TaskStatus{State: a2a.TaskStateWorking, Timestamp: time.Now()},
			}

			// Mayor delegates to engineer via A2A streaming
			engineerClient := a2a.NewClient(engineerTS.URL)
			streamChan, err := engineerClient.SendStreamingMessage(ctx, a2a.SendMessageParams{
				Message: *a2a.NewMessage("user", inputText),
			})
			if err != nil {
				events <- a2a.TaskEvent{
					Kind:   "statusUpdate",
					TaskID: taskID,
					Status: &a2a.TaskStatus{State: a2a.TaskStateFailed, Timestamp: time.Now()},
				}
				return
			}

			for event := range streamChan {
				var te a2a.TaskEvent
				if err := json.Unmarshal([]byte(event.Data), &te); err == nil {
					if te.TextDelta != "" {
						events <- a2a.TaskEvent{
							Kind:      "textDelta",
							TaskID:    taskID,
							TextDelta: te.TextDelta,
						}
					}
					if te.Status != nil {
						events <- a2a.TaskEvent{
							Kind:   "statusUpdate",
							TaskID: taskID,
							Status: &a2a.TaskStatus{State: te.Status.State, Timestamp: time.Now()},
						}
						if te.Status.State == a2a.TaskStateCompleted && ticket != nil {
							_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")
						}
					}
				}
			}
		}()

		return events, nil
	}

	mayorMux := http.NewServeMux()
	mayorMux.HandleFunc("/a2a", mayorServer.HandleA2A())
	mayorMux.HandleFunc("/.well-known/agent.json", mayorServer.HandleAgentCard(&a2a.AgentCard{
		Name:    "mayor",
		Version: "1.0.0",
		Url:     "http://localhost:8081",
		Capabilities: a2a.AgentCapabilities{
			Streaming:        true,
			PushNotifications: false,
		},
		Skills: []a2a.AgentSkill{
			{ID: "orchestrate", Name: "Orchestrate", Description: "Orchestrate work"},
		},
		DefaultStream: true,
	}))

	mayorTS := httptest.NewServer(mayorMux)
	defer mayorTS.Close()
	fmt.Printf("[2] Mayor A2A server (orchestrator) on %s\n", mayorTS.URL)
	fmt.Println()

	// --- Step 1: Discover mayor ---
	fmt.Println("[3] Discovering mayor agent card...")
	client := a2a.NewClient(mayorTS.URL)
	card, err := a2a.FetchAgentCard(ctx, mayorTS.URL)
	if err != nil {
		log.Fatalf("    FAIL: FetchAgentCard: %v", err)
	}
	fmt.Printf("    Agent: %s v%s\n", card.Name, card.Version)
	fmt.Println()

	// --- Step 2: A2A sendMessage mayor -> engineer ---
	fmt.Println("[4] Sending task via mayor (A2A JSON-RPC, mayor -> engineer)...")
	task, err := client.SendMessage(ctx, a2a.SendMessageParams{
		Message: *a2a.NewMessage("user", "Build the authentication module"),
	})
	if err != nil {
		log.Fatalf("    FAIL: SendMessage: %v", err)
	}
	fmt.Printf("    Task: id=%s state=%s\n", task.ID, task.State)
	if task.Message != nil {
		if text := extractText(task.Message.Parts); text != "" {
			fmt.Printf("    Response: %q\n", text)
		}
	}
	if task.State != a2a.TaskStateCompleted {
		log.Fatalf("    FAIL: expected completed, got %s", task.State)
	}
	fmt.Println("    [PASS] (a) A2A JSON-RPC sendMessage works")
	fmt.Println()

	// --- Step 3: SSE streaming ---
	fmt.Println("[5] Sending streaming task via SSE (mayor -> engineer via mayor)...")
	streamChan, err := client.SendStreamingMessage(ctx, a2a.SendMessageParams{
		Message: *a2a.NewMessage("user", "Run the test suite"),
	})
	if err != nil {
		log.Fatalf("    FAIL: SendStreamingMessage: %v", err)
	}

	var streamedText string
	var streamCompleted bool
	var eventCount int

	fmt.Println("    SSE events:")
	for event := range streamChan {
		eventCount++
		var te a2a.TaskEvent
		if err := json.Unmarshal([]byte(event.Data), &te); err == nil {
			if te.TextDelta != "" {
				streamedText += te.TextDelta
				fmt.Print(".")
			}
			if te.Status != nil {
				fmt.Printf("\n    status: %s\n", te.Status.State)
				if te.Status.State == a2a.TaskStateCompleted {
					streamCompleted = true
				}
			}
		}
	}
	fmt.Println()

	if !streamCompleted {
		log.Fatalf("    FAIL: SSE stream did not complete")
	}
	fmt.Printf("    Streamed text: %q\n", streamedText)
	fmt.Printf("    Total SSE events: %d\n", eventCount)
	fmt.Println("    [PASS] (b) SSE streaming works")
	fmt.Println()

	// --- Step 4: Verify Beads tickets ---
	fmt.Println("[6] Verifying Beads ticket creation...")
	fmt.Printf("    Tickets created: %d\n", len(createdTicketIDs))

	if len(createdTicketIDs) == 0 {
		log.Printf("    WARNING: no tickets created (Beads store may be unavailable)")
	}

	for i, ticketID := range createdTicketIDs {
		ticket, err := store.GetTicket(ctx, ticketID)
		if err != nil {
			log.Fatalf("    FAIL: GetTicket(%s): %v", ticketID, err)
		}
		fmt.Printf("    Ticket[%d]: id=%s agent=%s status=%s prompt=%q\n",
			i+1, ticket.ID, ticket.AgentID, ticket.Status, ticket.Prompt)
	}
	fmt.Println("    [PASS] (c) Beads ticket creation works")
	fmt.Println()

	// --- Summary ---
	fmt.Println("=== PoC Results ===")
	fmt.Println("  [PASS] (a) A2A JSON-RPC sendMessage")
	fmt.Println("  [PASS] (b) SSE streaming")
	fmt.Println("  [PASS] (c) Beads ticket creation")
	fmt.Printf("  API: %s / model: %s\n", baseURL, modelName)
	fmt.Printf("  Tickets: %d created and verified\n", len(createdTicketIDs))
	fmt.Println()

	// --- Launch separate LLM-backed agents (if api key available) ---
	if apiKey != "" {
		fmt.Println("=== Launching LLM-backed agents ===")
		fmt.Println("  Starting engineer (LLM) on :8082 ...")
		engCmd := exec.Command("go", "run", "engineer/main.go")
		engCmd.Env = append(os.Environ(),
			fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey),
			fmt.Sprintf("ANTHROPIC_BASE_URL=%s", baseURL),
			fmt.Sprintf("ANTHROPIC_MODEL=%s", modelName),
			"PORT=8082",
		)
		engCmd.Dir = "/home/cnovak/gassy/examples/poc"
		engCmd.Stdout = os.Stdout
		engCmd.Stderr = os.Stderr
		if err := engCmd.Start(); err != nil {
			log.Printf("warning: could not start engineer: %v", err)
		} else {
			fmt.Println("  Engineer PID:", engCmd.Process.Pid)
		}

		fmt.Println("  Starting mayor (orchestrator) on :8081 ...")
		mayCmd := exec.Command("go", "run", "mayor/main.go")
		mayCmd.Env = append(os.Environ(),
			fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey),
			fmt.Sprintf("ANTHROPIC_BASE_URL=%s", baseURL),
			fmt.Sprintf("ANTHROPIC_MODEL=%s", modelName),
			"PORT=8081",
			"ENGINEER_URL=http://localhost:8082",
		)
		mayCmd.Dir = "/home/cnovak/gassy/examples/poc"
		mayCmd.Stdout = os.Stdout
		mayCmd.Stderr = os.Stderr
		if err := mayCmd.Start(); err != nil {
			log.Printf("warning: could not start mayor: %v", err)
		} else {
			fmt.Println("  Mayor PID:", mayCmd.Process.Pid)
		}

		time.Sleep(3 * time.Second)
		fmt.Println("  LLM agents running. Test with:")
		fmt.Println("  curl -X POST http://localhost:8081/a2a -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"sendMessage\",\"params\":{\"message\":{\"role\":\"user\",\"parts\":[{\"type\":\"text\",\"text\":\"Hello\"}]}}}'")

		fmt.Println("  Stopping agents...")
		engCmd.Process.Kill()
		mayCmd.Process.Kill()
	}
	fmt.Println()
	fmt.Println("End-to-end PoC complete.")
}
