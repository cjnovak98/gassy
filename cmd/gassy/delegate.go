package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	var err error

	if delegateSkill != "" {
		// Skill-based delegation: only prompt argument expected
		if len(args) < 1 {
			return fmt.Errorf("delegate --skill requires a prompt argument")
		}
		prompt = args[0]

		// Use supervisor's discover endpoint to find agents by skill
		agentURL, agentID, err = discoverAgentBySkill(ctx, delegateSkill)
		if err != nil {
			return err
		}
	} else {
		// Explicit agent ID delegation: requires agent-id and prompt
		if len(args) < 2 {
			return fmt.Errorf("delegate requires [agent-id] [prompt]")
		}
		agentID = args[0]
		prompt = args[1]

		// Query supervisor for agent URL
		agents, err := getAgentsFromSupervisor()
		if err != nil {
			return fmt.Errorf("fetching agents from supervisor: %w", err)
		}

		var found bool
		for _, a := range agents {
			if a["agent_id"] == agentID {
				agentURL = a["a2a_url"].(string)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("agent %q not found in supervisor registry", agentID)
		}
		if agentURL == "" {
			return fmt.Errorf("agent %q has no A2A URL registered", agentID)
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

// discoverAgentBySkill queries the supervisor's discover endpoint for agents with a skill
func discoverAgentBySkill(ctx context.Context, skill string) (url string, agentID string, err error) {
	resp, err := http.Get(supervisorHTTP + "/registry/discover?skill=" + skill)
	if err != nil {
		return "", "", fmt.Errorf("connecting to supervisor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("supervisor discover returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result []struct {
		AgentID string `json:"agent_id"`
		A2AURL  string `json:"a2a_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decoding discover response: %w", err)
	}

	if len(result) == 0 {
		return "", "", fmt.Errorf("no agent found with skill %q", skill)
	}

	return result[0].A2AURL, result[0].AgentID, nil
}
