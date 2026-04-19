package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent operations",
	Long:  "List registered agents from supervisor",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents",
	RunE:  runAgentList,
}

func init() {
	agentCmd.AddCommand(agentListCmd)
	rootCmd.AddCommand(agentCmd)
}

// getAgentsFromSupervisor fetches the agent list from the supervisor HTTP API
func getAgentsFromSupervisor() ([]map[string]interface{}, error) {
	resp, err := http.Get(supervisorHTTP + "/registry/list")
	if err != nil {
		return nil, fmt.Errorf("connecting to supervisor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor returned status %d: %s", resp.StatusCode, string(body))
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result, nil
}

func runAgentList(cmd *cobra.Command, args []string) error {
	agents, err := getAgentsFromSupervisor()
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents registered")
		return nil
	}

	fmt.Println("Registered Agents:")
	fmt.Println("-------------------")
	for _, a := range agents {
		if name, ok := a["agent_id"].(string); ok {
			fmt.Printf("Agent ID: %s\n", name)
		}
		if role, ok := a["role"].(string); ok {
			fmt.Printf("Role:     %s\n", role)
		}
		if url, ok := a["a2a_url"].(string); ok {
			fmt.Printf("A2A URL:  %s\n", url)
		}
		fmt.Println("-------------------")
	}

	return nil
}