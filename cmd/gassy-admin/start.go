package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cjnovak98/gassy/internal/city"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start all gassy containers",
	Long:  "Start supervisor and agent containers using podman",
	Args:  cobra.NoArgs,
	RunE:  runStart,
}

var (
	cityFile string
	envFile  string
)

func init() {
	startCmd.Flags().StringVarP(&cityFile, "city", "c", "city.toml", "Path to city.toml")
	startCmd.Flags().StringVarP(&envFile, "env", "e", ".env", "Path to env file")
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	c, err := city.ParseFile(cityFile)
	if err != nil {
		return fmt.Errorf("parsing city config: %w", err)
	}

	// Start supervisor container
	if err := startSupervisor(); err != nil {
		return fmt.Errorf("starting supervisor: %w", err)
	}
	fmt.Println("Started supervisor container")

	// Start agent containers
	supervisorPort := "9091"
	for _, agent := range c.Agents {
		if err := startAgentContainer(agent, supervisorPort); err != nil {
			return fmt.Errorf("starting agent %s: %w", agent.ID, err)
		}
		fmt.Printf("Started agent container: %s\n", agent.ID)
	}

	fmt.Printf("City %s started with %d agents\n", c.City.Name, len(c.Agents))
	return nil
}

func startSupervisor() error {
	args := []string{
		"run", "-d",
		"--name", "gassy-supervisor",
		"--label", "gassy=true",
		"--network=host",
		"--env-file", envFile,
		"gassy-agent:latest",
		"supervisor",
	}
	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func startAgentContainer(agent city.AgentConfig, supervisorPort string) error {
	// Determine port - default to 8080 + offset based on agent index
	port := getAgentPort(agent)

	args := []string{
		"run", "-d",
		"--name", "gassy-" + agent.ID,
		"--label", "gassy=true",
		"--network=host",
		"-p", fmt.Sprintf("%d:%d", port, port),
		"-e", "AGENT_ROLE=" + agent.Role,
		"-e", fmt.Sprintf("PORT=%d", port),
		"-e", "SUPERVISOR_URL=http://127.0.0.1:"+supervisorPort,
		"--env-file", envFile,
		"gassy-agent:latest",
	}

	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getAgentPort(agent city.AgentConfig) int {
	// Map agent roles to default ports
	switch agent.Role {
	case "mayor":
		return 8080
	case "engineer":
		return 8081
	case "designer":
		return 8082
	default:
		// Use hash of ID for consistent port assignment
		hash := 0
		for _, c := range agent.ID {
			hash += int(c)
		}
		return 8083 + (hash % 10)
	}
}

func ensureCityFileExists() error {
	if _, err := os.Stat(cityFile); os.IsNotExist(err) {
		return fmt.Errorf("city file %q not found", cityFile)
	}
	return nil
}