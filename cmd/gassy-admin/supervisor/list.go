package supervisor

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/spf13/cobra"
)

const socketPath = "/tmp/gassy-supervisor.sock"

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered agents",
	Long:  "List all registered agents via supervisor socket",
	RunE:  runSupervisorList,
}

func init() {
	SupervisorCmd.AddCommand(listCmd)
}

func sendSocketCommand(cmd map[string]interface{}) (map[string]interface{}, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to supervisor socket: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return nil, fmt.Errorf("encoding command: %w", err)
	}

	resp := make(map[string]interface{})
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("supervisor returned empty response")
		}
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return resp, nil
}

func runSupervisorList(cmd *cobra.Command, args []string) error {
	resp, err := sendSocketCommand(map[string]interface{}{"action": "list"})
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if errMsg, ok := resp["error"].(string); ok {
		return fmt.Errorf("error: %s", errMsg)
	}

	agents, ok := resp["agents"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid response format")
	}

	if len(agents) == 0 {
		fmt.Println("No agents registered")
		return nil
	}

	fmt.Println("Registered Agents:")
	fmt.Println("-------------------")
	for _, a := range agents {
		agent := a.(map[string]interface{})
		fmt.Printf("Name:   %s\n", agent["Name"])
		fmt.Printf("URL:    %s\n", agent["URL"])
		if cardURL, ok := agent["CardURL"].(string); ok && cardURL != "" {
			fmt.Printf("Card:   %s\n", cardURL)
		}
		if pid, ok := agent["PID"].(float64); ok {
			fmt.Printf("PID:    %.0f\n", pid)
		}
		fmt.Printf("Status: %s\n", agent["Status"])
		if skills, ok := agent["Skills"].([]interface{}); ok && len(skills) > 0 {
			fmt.Printf("Skills: %v\n", skills)
		}
		fmt.Println("-------------------")
	}

	return nil
}