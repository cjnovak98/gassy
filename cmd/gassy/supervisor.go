package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

const supervisorHTTP = "http://localhost:9091"

var supervisorCmd = &cobra.Command{
	Use:   "supervisor",
	Short: "Supervisor agent management",
	Long:  "Manage agents via the supervisor process",
}

var supervisorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents",
	RunE:  runSupervisorList,
}

var supervisorHireCmd = &cobra.Command{
	Use:   "hire [name]",
	Short: "Hire a new agent",
	Long: `Hire a new agent by name. The supervisor handles configuration, port allocation,
and role mapping. Provide --role and --skills only if creating a custom agent.`,
	Args: cobra.ExactArgs(1),
	RunE: runSupervisorHire,
}

var (
	hireRole   string
	hireSkills string // comma-separated skills
	hirePort   int
	hireCity   string
)

var supervisorFireCmd = &cobra.Command{
	Use:   "fire [name]",
	Short: "Fire an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runSupervisorFire,
}

func init() {
	supervisorCmd.AddCommand(supervisorListCmd, supervisorHireCmd, supervisorFireCmd)
	rootCmd.AddCommand(supervisorCmd)

	supervisorHireCmd.Flags().StringVar(&hireRole, "role", "", "Role for the new agent (optional, supervisor uses base config)")
	supervisorHireCmd.Flags().StringVar(&hireSkills, "skills", "", "Comma-separated skills for the new agent")
	supervisorHireCmd.Flags().IntVar(&hirePort, "port", 0, "Port for the new agent (optional, supervisor allocates dynamically)")
	supervisorHireCmd.Flags().StringVarP(&hireCity, "city", "c", "", "Path to city.toml (optional, supervisor has base config)")
}

func sendHTTPRequest(path string, body map[string]interface{}) ([]map[string]interface{}, error) {
	if body == nil {
		resp, err := http.Get(supervisorHTTP + path)
		if err != nil {
			return nil, fmt.Errorf("connecting to supervisor: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("supervisor returned status %d: %s", resp.StatusCode, string(respBody))
		}

		var result []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		return result, nil
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	resp, err := http.Post(supervisorHTTP+path, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("connecting to supervisor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result, nil
}

func runSupervisorList(cmd *cobra.Command, args []string) error {
	agents, err := sendHTTPRequest("/registry/list", nil)
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
		fmt.Printf("Agent ID: %v\n", a["agent_id"])
		fmt.Printf("Role:     %v\n", a["role"])
		fmt.Printf("A2A URL:  %v\n", a["a2a_url"])
		fmt.Println("-------------------")
	}

	return nil
}

func runSupervisorHire(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If city flag provided, parse it for logging only - supervisor handles config
	if hireCity != "" {
		cityCfg, err := ParseFile(hireCity)
		if err != nil {
			return fmt.Errorf("parsing city config: %w", err)
		}
		if agentCfg := cityCfg.GetAgent(name); agentCfg.ID != "" {
			fmt.Printf("Hiring agent %q from city.toml\n", name)
		}
	}

	// Send hire request to supervisor - it handles port allocation and role mapping
	_, err := sendHTTPRequest("/supervisor/hire", map[string]interface{}{
		"name":   name,
		"role":   hireRole,
		"port":   hirePort, // 0 means supervisor allocates dynamically
		"skills": parseSkills(hireSkills),
	})
	if err != nil {
		return fmt.Errorf("hire failed: %w", err)
	}

	fmt.Printf("Successfully hired agent: %s\n", name)
	return nil
}

func parseSkills(skillsStr string) []string {
	if skillsStr == "" {
		return nil
	}
	var skills []string
	for _, s := range strings.Split(skillsStr, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			skills = append(skills, s)
		}
	}
	return skills
}

func runSupervisorFire(cmd *cobra.Command, args []string) error {
	name := args[0]

	_, err := sendHTTPRequest("/supervisor/fire", map[string]interface{}{
		"name": name,
	})
	if err != nil {
		return fmt.Errorf("fire failed: %w", err)
	}

	fmt.Printf("Successfully fired agent: %s\n", name)
	return nil
}

