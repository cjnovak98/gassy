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

	// Stop all gassy containers
	// First try by known names, then by label, then by image/command
	containers := []string{"gassy-supervisor", "gassy-mayor", "gassy-engineer", "gassy-designer"}
	for _, c := range containers {
		stopContainer(c) // ignore errors, container may not exist
	}

	// Also stop any container running supervisor command with gassy image
	if err := stopSupervisorContainers(); err != nil {
		fmt.Printf("Warning: error stopping supervisor: %v\n", err)
	}

	fmt.Println("All gassy containers stopped")
	return nil
}

// stopSupervisorContainers finds and stops any container running supervisor
func stopSupervisorContainers() error {
	// List all containers
	cmd := exec.Command("podman", "ps", "-a", "--format", "{{.ID}} {{.Names}} {{.Command}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		id, name, command := fields[0], fields[1], strings.Join(fields[2:], " ")
		// Check if this is a supervisor container
		if strings.Contains(command, "supervisor") || name == "dreamy_dubinsky" {
			fmt.Printf("Stopping supervisor container: %s (%s)\n", name, id)
			exec.Command("podman", "rm", "-f", id).Run()
		}
	}
	return nil
}

func stopContainer(name string) error {
	cmd := exec.Command("podman", "rm", "-f", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}