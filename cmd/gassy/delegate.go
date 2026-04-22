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
	Long:  "Send a prompt to a specific agent and stream the response. Use --skill to find an agent by skill. Use --task-id to continue a conversation.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runDelegate,
}

var delegateSkill string
var delegateTaskID string

func init() {
	delegateCmd.Flags().StringVarP(&delegateTaskID, "task-id", "t", "", "Task ID (provide same ID to resume a conversation)")
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

	// Create A2A client with extended timeout for delegation (may involve multiple agent hops)
	client := a2a.NewClientWithTimeout(agentURL, 120*time.Second)

	message := a2a.Message{
		Role: "user",
		Parts: []a2a.Part{
			a2a.TextPart{Type: "text", Text: prompt},
		},
	}

	params := a2a.SendMessageParams{
		Message: message,
		Stream:  true, // Always stream
	}

	// Use provided task ID - if not provided, agent generates one
	if delegateTaskID != "" {
		params.TaskID = delegateTaskID
	}

	return streamDelegate(ctx, client, params)
}

func streamDelegate(ctx context.Context, client *a2a.Client, params a2a.SendMessageParams) error {
	events, err := client.SendStreamingMessage(ctx, params)
	if err != nil {
		return fmt.Errorf("streaming message: %w", err)
	}

	var finalTaskID string
	for event := range events {
		switch event.Event {
		case "statusUpdate":
			fmt.Printf("[status] %s\n", event.Data)
		case "artifactUpdate":
			fmt.Printf("[artifact] %s\n", event.Data)
		case "textDelta":
			// Parse JSON to extract text field
			var textMsg struct {
				Kind      string `json:"kind"`
				TextDelta string `json:"textDelta"`
			}
			if json.Unmarshal([]byte(event.Data), &textMsg) == nil {
				fmt.Print(textMsg.TextDelta)
			} else {
				fmt.Print(event.Data)
			}
		case "message":
			// Handle plain text [done] marker from agent - task already completed
			if event.Data == "[done]" {
				break
			}
			// Fall through to default for other message events
			fmt.Printf("[message] %s\n", event.Data)
		case "task":
			// Parse task event to extract taskId
			var taskMsg struct {
				Kind    string `json:"kind"`
				Task    struct {
					ID       string `json:"id"`
					SessionID string `json:"sessionId"`
				} `json:"task"`
			}
			if json.Unmarshal([]byte(event.Data), &taskMsg) == nil {
				finalTaskID = taskMsg.Task.ID
			}
		default:
			fmt.Printf("[%s] %s\n", event.Event, event.Data)
		}
	}
	fmt.Println()
	if finalTaskID != "" {
		fmt.Printf("task-id: %s\n", finalTaskID)
	}
	return nil
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
