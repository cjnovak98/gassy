package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	if _, err := city.ParseFile(cityFile); err != nil {
		return fmt.Errorf("parsing city config: %w", err)
	}

	// Start supervisor container
	cityDir, err := filepath.Abs(filepath.Dir(cityFile))
	if err != nil {
		return fmt.Errorf("resolving city directory: %w", err)
	}
	if err := startSupervisor(cityDir); err != nil {
		return fmt.Errorf("starting supervisor: %w", err)
	}
	fmt.Println("Started supervisor container")

	// Supervisor handles agent lifecycle via reconcile
	fmt.Println("Supervisor will reconcile agents from city.toml")
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

// ensurePodmanSocket enables and starts the rootless podman socket via systemd
func ensurePodmanSocket() error {
	// Use systemctl --user enable --now to ensure the socket is enabled and started
	cmd := exec.Command("systemctl", "--user", "enable", "--now", "podman.socket")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("enabling podman socket: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	// Give systemd a moment to create the socket file
	time.Sleep(300 * time.Millisecond)
	return nil
}

func startSupervisor(cityDir string) error {
	// Ensure podman socket is running for rootless podman
	if err := ensurePodmanSocket(); err != nil {
		return fmt.Errorf("podman socket: %w", err)
	}

	socketPath := "/run/user/1000/podman/podman.sock"

	// Start supervisor in a container with root access
	// Mount the rootless podman socket and the city config directory
	cmd := exec.Command("podman", "run", "-d",
		"--name", "gassy-supervisor",
		"--label", "gassy=true",
		"--network=host",
		"-v", socketPath+":"+socketPath,
		"-v", cityDir+":/etc/gassy:ro",
		"-e", "PODMAN_SOCKET="+socketPath,
		"-e", "CONTAINER_HOST=unix://"+socketPath,
		"--env-file", envFile,
		"--security-opt", "label=disable",
		"localhost:5000/gassy/supervisor:latest",
		"/etc/gassy/city.toml",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting supervisor container: %w", err)
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