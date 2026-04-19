package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cjnovak98/gassy/internal/a2a"
	"github.com/spf13/cobra"
)

var delegateCmd = &cobra.Command{
	Use:   "delegate [agent-id] [prompt]",
	Short: "Delegate work to an agent via A2A",
	Long:  "Send a prompt to a specific agent and stream the response. Use --skill to find an agent by skill.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runDelegate,
}

var delegateStream bool
var delegateSkill string

func init() {
	delegateCmd.Flags().BoolVarP(&delegateStream, "stream", "s", false, "Stream response updates")
	delegateCmd.Flags().StringVarP(&delegateSkill, "skill", "k", "", "Find agent by skill instead of agent-id")
	rootCmd.AddCommand(delegateCmd)
}

func runDelegate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	var agentID string
	var agentURL string
	var prompt string

	if delegateSkill != "" {
		// Skill-based delegation: only prompt argument expected
		if len(args) < 1 {
			return fmt.Errorf("delegate --skill requires a prompt argument")
		}
		prompt = args[0]

		// Use skill-based discovery via AgentRegistry
		registry := a2a.NewAgentRegistry()
		urls := getAllNetworkURLs(ctx)

		// Discover agents from network URLs
		for _, url := range urls {
			card, err := a2a.FetchAgentCard(ctx, url)
			if err != nil {
				continue
			}
			registry.Register(card)
		}

		// Find agents with the requested skill
		agents := registry.GetBySkill(delegateSkill)
		if len(agents) == 0 {
			return fmt.Errorf("no agent found with skill %q", delegateSkill)
		}
		// Use first agent with the skill
		agentID = agents[0].Name
		agentURL = agents[0].Url
	} else {
		// Explicit agent ID delegation: requires agent-id and prompt
		if len(args) < 2 {
			return fmt.Errorf("delegate requires [agent-id] [prompt]")
		}
		agentID = args[0]
		prompt = args[1]

		// Parse city config to get agent URL
		city, err := ParseFile(cityFile)
		if err != nil {
			return fmt.Errorf("parsing city config: %w", err)
		}

		// Get agent config
		agent := city.GetAgent(agentID)
		if agent.ID == "" {
			return fmt.Errorf("agent %q not found in city config", agentID)
		}

		// Get agent URL from network config
		agentURL = getAgentURL(city, agentID)
		if agentURL == "" {
			return fmt.Errorf("no URL configured for agent %q", agentID)
		}
	}

	// Create A2A client
	client := a2a.NewClient(agentURL)

	message := a2a.Message{
		Role: "user",
		Parts: []a2a.Part{
			a2a.TextPart{Type: "text", Text: prompt},
		},
	}

	params := a2a.SendMessageParams{
		Message: message,
		Stream:  delegateStream,
	}

	if delegateStream {
		return streamDelegate(ctx, client, params)
	}

	task, err := client.SendMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	// Poll for task completion
	return pollTask(ctx, client, task.ID)
}

func streamDelegate(ctx context.Context, client *a2a.Client, params a2a.SendMessageParams) error {
	events, err := client.SendStreamingMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("streaming message: %w", err)
	}

	for event := range events {
		switch event.Event {
		case "statusUpdate":
			fmt.Printf("[status] %s\n", event.Data)
		case "artifactUpdate":
			fmt.Printf("[artifact] %s\n", event.Data)
		case "textDelta":
			fmt.Print(event.Data)
		default:
			fmt.Printf("[%s] %s\n", event.Event, event.Data)
		}
	}
	fmt.Println()
	return nil
}

func pollTask(ctx context.Context, client *a2a.Client, taskID string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("task timeout")
		case <-ticker.C:
			task, err := client.GetTask(ctx, taskID)
			if err != nil {
				return fmt.Errorf("get task: %w", err)
			}

			// Handle nil status
			if task.Status == nil {
				fmt.Print(".")
				continue
			}

			switch task.Status.State {
			case a2a.TaskStateCompleted:
				if task.Message != nil {
					for _, part := range task.Message.Parts {
						if tp, ok := part.(a2a.TextPart); ok {
							fmt.Print(tp.Text)
						}
					}
				}
				fmt.Println()
				return nil
			case a2a.TaskStateFailed:
				return fmt.Errorf("task failed")
			case a2a.TaskStateWorking:
				fmt.Print(".")
			case a2a.TaskStateInputReq:
				fmt.Print("[input-required] ")
			case a2a.TaskStateAuthReq:
				fmt.Print("[auth-required] ")
			case a2a.TaskStateCanceled:
				fmt.Println("[canceled]")
				return nil
			case a2a.TaskStateRejected:
				return fmt.Errorf("task rejected")
			}
		}
	}
}

func getAgentURL(city *City, agentID string) string {
	// Use map lookup for all agents including known ones
	networkURLs := mapAgentNetworkURLs(city)
	if url, ok := networkURLs[agentID]; ok {
		return url
	}
	return ""
}

// mapAgentNetworkURLs creates a map of agent IDs to their network URLs
func mapAgentNetworkURLs(city *City) map[string]string {
	urls := make(map[string]string)
	// Check network config fields for agent URLs
	if city.Network.MayorURL != "" {
		urls["mayor"] = city.Network.MayorURL
	}
	if city.Network.EngineerURL != "" {
		urls["engineer"] = city.Network.EngineerURL
	}
	if city.Network.DesignerURL != "" {
		urls["designer"] = city.Network.DesignerURL
	}
	return urls
}

// getAllNetworkURLs returns all configured network URLs from city config
func getAllNetworkURLs(ctx context.Context) []string {
	city, err := ParseFile(cityFile)
	if err != nil {
		return nil
	}
	var urls []string
	if city.Network.MayorURL != "" {
		urls = append(urls, city.Network.MayorURL)
	}
	if city.Network.EngineerURL != "" {
		urls = append(urls, city.Network.EngineerURL)
	}
	if city.Network.DesignerURL != "" {
		urls = append(urls, city.Network.DesignerURL)
	}
	return urls
}
