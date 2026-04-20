package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
	// Validate credentials before starting
	if err := validateCredentials(); err != nil {
		return fmt.Errorf("credential validation failed: %w", err)
	}

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

// validateCredentials checks that required environment variables are set with valid values
func validateCredentials() error {
	required := []string{"ANTHROPIC_AUTH_TOKEN"}
	placeholder := []string{"test", "your_key_here", ""}

	for _, env := range required {
		value := os.Getenv(env)
		if value == "" {
			// Try reading from env file
			fileValue, err := readEnvFile(envFile, env)
			if err != nil || fileValue == "" {
				return fmt.Errorf("%s is not set in %s", env, envFile)
			}
			value = fileValue
		}

		// Check for placeholder values
		for _, p := range placeholder {
			if value == p {
				return fmt.Errorf("%s has placeholder value %q in %s - please set a valid API key", env, p, envFile)
			}
		}
	}
	return nil
}

// readEnvFile reads a specific environment variable from a .env file
func readEnvFile(path, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.SplitN(line, "=", 2)[1], nil
		}
	}
	return "", fmt.Errorf("key %s not found", key)
}

// ensurePodmanSocket starts the podman socket if not already running
func ensurePodmanSocket() {
	socketPath := "/run/user/1000/podman/podman.sock"

	// Check if socket already exists
	if _, err := os.Stat(socketPath); err == nil {
		return // Socket already exists, no action needed
	}

	// Try to start the socket via systemd first
	cmd := exec.Command("systemctl", "--user", "start", "podman.socket")
	if err := cmd.Run(); err != nil {
		// If systemctl fails, try running podman system service directly in background
		// This creates a temporary socket that persists until the process is killed
		proc, err := os.StartProcess("/usr/bin/podman", []string{"podman", "system", "service", "--time=0"}, &os.ProcAttr{
			Dir: "/tmp",
			Env: append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS=autolaunch:"),
		})
		if err == nil {
			proc.Release() // Let it run independently
			// Give it a moment to start
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func startSupervisor() error {
	// Ensure podman socket is running for rootless podman
	ensurePodmanSocket()

	// Run supervisor on the HOST (not in a container) so it can spawn containers
	// The supervisor binary should be in PATH or GOBIN
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		gobin = os.Getenv("GOPATH") + "/bin"
	}
	supervisorBin := gobin + "/supervisor"

	// Check if supervisor binary exists
	if _, err := os.Stat(supervisorBin); os.IsNotExist(err) {
		return fmt.Errorf("supervisor binary not found at %s (run 'go install ./cmd/supervisor/' to build)", supervisorBin)
	}

	// Start supervisor as a background process on the host
	cmd := exec.Command(supervisorBin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting supervisor: %w", err)
	}

	// Give it a moment to start
	time.Sleep(500 * time.Millisecond)
	return nil
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
		"-e", "AGENT_ROLE=" + agent.Cmd,
		"-e", fmt.Sprintf("PORT=%d", port),
		"-e", "SUPERVISOR_URL=http://127.0.0.1:"+supervisorPort,
		"--env-file", envFile,
		"localhost:5000/gassy/agent:latest",
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
		return 8082
	case "designer":
		return 8083
	default:
		// Use hash of ID for consistent port assignment
		hash := 0
		for _, c := range agent.ID {
			hash += int(c)
		}
		return 8084 + (hash % 10)
	}
}

func ensureCityFileExists() error {
	if _, err := os.Stat(cityFile); os.IsNotExist(err) {
		return fmt.Errorf("city file %q not found", cityFile)
	}
	return nil
}