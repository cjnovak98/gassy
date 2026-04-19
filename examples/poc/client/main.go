// Package main is an A2A client that discovers agents and sends tasks.
// It demonstrates the A2A protocol: JSON-RPC 2.0 over HTTP + SSE streaming.
//
// Usage:
//   go run .
//   A2A_SERVER_URL=http://localhost:8080 go run .
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cjnovak98/gassy/internal/a2a"
)

const defaultTimeout = 30 * time.Second

func main() {
	serverURL := getServerURL()
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	fmt.Println("=== A2A Client Demo ===")
	fmt.Println()

	// Step 1: Discover agent via Agent Card
	fmt.Println("[1] Discovering agent via Agent Card...")
	card, err := a2a.FetchAgentCard(ctx, serverURL)
	if err != nil {
		log.Fatalf("   ERROR: Failed to fetch agent card: %v", err)
	}
	fmt.Printf("    Agent: %s v%s\n", card.Name, card.Version)
	fmt.Printf("    URL: %s\n", card.Url)
	fmt.Printf("    Capabilities: streaming=%v, pushNotifications=%v\n",
		card.Capabilities.Streaming, card.Capabilities.PushNotifications)
	if len(card.Skills) > 0 {
		fmt.Printf("    Skills:")
		for _, s := range card.Skills {
			fmt.Printf(" %s(%s)", s.Name, s.ID)
		}
		fmt.Println()
	}
	fmt.Println()

	// Step 2: Create client and send message
	client := a2a.NewClient(serverURL)
	msg := a2a.NewMessage("user", "Hello, agent! Can you process this task?")

	fmt.Println("[2] Sending message via JSON-RPC 2.0...")
	task, err := client.SendMessage(ctx, a2a.SendMessageParams{
		Message: *msg,
		Stream:  false,
	})
	if err != nil {
		log.Fatalf("   ERROR: SendMessage failed: %v", err)
	}
	fmt.Printf("    Task created: ID=%s State=%s\n", task.ID, task.State)
	fmt.Println()

	// Step 3: Retrieve task status
	fmt.Println("[3] Retrieving task status...")
	fetched, err := client.GetTask(ctx, task.ID)
	if err != nil {
		log.Fatalf("   ERROR: GetTask failed: %v", err)
	}
	fmt.Printf("    Task: ID=%s State=%s\n", fetched.ID, fetched.State)
	fmt.Println()

	// Step 4: Send streaming message (if supported)
	if card.Capabilities.Streaming {
		fmt.Println("[4] Sending streaming message (SSE)...")
		streamChan, err := client.SendStreamingMessage(ctx, a2a.SendMessageParams{
			Message: *a2a.NewMessage("user", "Stream this please"),
			Stream:  true,
		})
		if err != nil {
			log.Fatalf("   ERROR: SendStreamingMessage failed: %v", err)
		}

		fmt.Println("    SSE Events:")
		var eventCount int
		var fullText string

		for event := range streamChan {
			eventCount++
			var te a2a.TaskEvent
			if json.Unmarshal([]byte(event.Data), &te) == nil {
				if te.TextDelta != "" {
					fullText += te.TextDelta
					fmt.Printf("      [%d] textDelta: %q\n", eventCount, te.TextDelta)
				}
				if te.Status != nil {
					fmt.Printf("      [%d] statusUpdate: %s\n", eventCount, te.Status.State)
				}
			} else {
				fmt.Printf("      [%d] %s: %s\n", eventCount, event.Event, event.Data)
			}
		}

		fmt.Println()
		if fullText != "" {
			fmt.Printf("    Assembled text: %s\n", fullText)
		}
		fmt.Printf("    Total events: %d\n", eventCount)
	} else {
		fmt.Println("[4] Streaming not supported by agent, skipping")
	}

	fmt.Println()
	fmt.Println("[5] Demo complete")
}

func getServerURL() string {
	if url := os.Getenv("A2A_SERVER_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}
