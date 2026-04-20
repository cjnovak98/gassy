package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [container-id]",
	Short: "Stop gassy containers",
	Long:  "Stop all gassy containers or a specific container by ID",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		// Stop specific container
		containerName := args[0]
		if !strings.HasPrefix(containerName, "gassy-") {
			containerName = "gassy-" + containerName
		}
		return stopContainer(containerName)
	}

	// Stop all gassy containers (supervisor is now in a container again)
	containers := []string{"gassy-supervisor", "gassy-mayor", "gassy-engineer", "gassy-designer"}
	for _, c := range containers {
		stopContainer(c) // ignore errors, container may not exist
	}

	// Also stop any other gassy containers by label
	if err := stopSupervisorContainers(); err != nil {
		fmt.Printf("Warning: error stopping containers: %v\n", err)
	}

	fmt.Println("All gassy containers stopped")
	return nil
}

// stopSupervisorContainers finds and stops all gassy containers using label filter
func stopSupervisorContainers() error {
	// Use label filter to find ALL gassy containers - this is more reliable than
	// parsing command output which can be truncated. Use -a to include stopped containers.
	cmd := exec.Command("podman", "ps", "-a", "--filter", "label=gassy=true", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("listing gassy containers: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		fmt.Printf("Stopping gassy container: %s\n", name)
		exec.Command("podman", "rm", "-f", name).Run()
	}
	return nil
}

func stopContainer(name string) error {
	cmd := exec.Command("podman", "rm", "-f", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
}