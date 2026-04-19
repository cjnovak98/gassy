// Package main runs the LLM-backed engineer agent.
// It implements an A2A server that calls MiniMax via the Anthropic SDK
// and streams the LLM response back via SSE.
package main

import (
	"context"
	"fmt"
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
//   ANTHROPIC_MODEL      Model name (default: MiniMax-Text-01)

func main() {
	ctx := context.Background()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	// --- Build Anthropic client ---
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	modelName := os.Getenv("ANTHROPIC_MODEL")
	if modelName == "" {
		modelName = "MiniMax-M2.7"
	}

	var opts []option.RequestOption
	opts = append(opts, option.WithAPIKey(apiKey))
	if baseURL == "" {
		baseURL = "https://api.minimax.io/anthropic"
	}
	opts = append(opts, option.WithBaseURL(baseURL))
	client := anthropic.NewClient(opts...)

	// --- Setup Beads store (in-memory) ---
	store := beads.New("localhost:9090")

	// --- Build A2A server ---
	server := a2a.NewServer()

	// HandleMessage: non-streaming LLM call + Beads ticket
	server.HandleMessage = func(msg a2a.Message) (*a2a.Task, error) {
		taskID := fmt.Sprintf("eng-task-%d", time.Now().UnixNano())
		inputText := extractText(msg.Parts)

		// Create Beads ticket
		ticket, err := store.CreateTicket(ctx, "engineer", inputText)
		if err != nil {
			return nil, fmt.Errorf("creating beads ticket: %w", err)
		}

		// Call LLM with system prompt so engineer knows its role
		result, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{{
				Text: "You are the Engineer agent in a multi-agent system called Gassy. The Mayor orchestrator delegates coding, testing, and build tasks to you via A2A. Complete the delegated task and return your work directly. Do not refuse, do not say you cannot — just do it.",
			}},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(inputText)),
			},
			Model: anthropic.Model(modelName),
		})
		if err != nil {
			_ = store.UpdateTicketStatus(ctx, ticket.ID, "error")
			return nil, fmt.Errorf("llm call: %w", err)
		}

		// Extract text response
		var responseText string
		for _, block := range result.Content {
			tb := block.AsText()
			responseText += tb.Text
		}

		_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")

		task := a2a.NewTask(taskID, &msg)
		task.State = a2a.TaskStateCompleted
		task.Status = &a2a.TaskStatus{
			State:     a2a.TaskStateCompleted,
			Timestamp: time.Now(),
		}
		task.Message = a2a.NewMessage("agent", responseText)
		return task, nil
	}

	// HandleStreamingMessage: streaming LLM call + Beads ticket
	server.HandleStreamingMessage = func(msg a2a.Message) (<-chan a2a.TaskEvent, error) {
		taskID := fmt.Sprintf("eng-stream-%d", time.Now().UnixNano())
		events := make(chan a2a.TaskEvent, 100)
		inputText := extractText(msg.Parts)

		// Create Beads ticket
		ticket, err := store.CreateTicket(ctx, "engineer", "[stream] "+inputText)
		if err != nil {
			close(events)
			return nil, fmt.Errorf("creating beads ticket: %w", err)
		}

		go func() {
			defer close(events)

			log.Printf("[engineer] received delegation request: %s", inputText)

			// Send working status
			events <- a2a.TaskEvent{
				Kind:   "statusUpdate",
				TaskID: taskID,
				Status: &a2a.TaskStatus{State: a2a.TaskStateWorking, Timestamp: time.Now()},
			}

			// Stream LLM response with system prompt
			stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
				MaxTokens: 8192,
				System: []anthropic.TextBlockParam{{
					Text: "You are the Engineer agent in a multi-agent system called Gassy. The Mayor orchestrator delegates coding, testing, and build tasks to you via A2A. Complete the delegated task and return your work directly. Do not refuse, do not say you cannot — just do it.",
				}},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock(inputText)),
				},
				Model: anthropic.Model(modelName),
			})
			defer stream.Close()

			for stream.Next() {
				event := stream.Current()
				switch e := event.AsAny().(type) {
				case anthropic.ContentBlockDeltaEvent:
					td := e.Delta.AsTextDelta()
					events <- a2a.TaskEvent{
						Kind:      "textDelta",
						TaskID:    taskID,
						TextDelta: td.Text,
					}
				case anthropic.MessageStopEvent:
					_ = store.UpdateTicketStatus(ctx, ticket.ID, "completed")
					events <- a2a.TaskEvent{
						Kind:   "statusUpdate",
						TaskID: taskID,
						Status: &a2a.TaskStatus{State: a2a.TaskStateCompleted, Timestamp: time.Now()},
					}
				}
			}

			if stream.Err() != nil {
				_ = store.UpdateTicketStatus(ctx, ticket.ID, "error")
				events <- a2a.TaskEvent{
					Kind:   "statusUpdate",
					TaskID: taskID,
					Status: &a2a.TaskStatus{State: a2a.TaskStateFailed, Timestamp: time.Now()},
				}
			}
		}()

		return events, nil
	}

	// --- Wire HTTP ---
	mux := http.NewServeMux()

	card := &a2a.AgentCard{
		Name:    "engineer",
		Version: "1.0.0",
		Url:     fmt.Sprintf("http://localhost:%s", port),
		Capabilities: a2a.AgentCapabilities{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: false,
		},
		Skills: []a2a.AgentSkill{
			{ID: "build", Name: "Build", Description: "Build code using LLM"},
			{ID: "test", Name: "Test", Description: "Run tests"},
		},
		DefaultStream: true,
	}

	mux.HandleFunc("/a2a", server.HandleA2A())
	mux.HandleFunc("/.well-known/agent.json", server.HandleAgentCard(card))

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Engineer agent listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
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
