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
	containers := []string{"gassy-supervisor", "gassy-mayor", "gassy-engineer", "gassy-designer"}
	for _, c := range containers {
		if err := stopContainer(c); err != nil {
			fmt.Printf("Warning: could not stop %s: %v\n", c, err)
		} else {
			fmt.Printf("Stopped: %s\n", c)
		}
	}

	fmt.Println("All gassy containers stopped")
	return nil
}

func stopContainer(name string) error {
	cmd := exec.Command("podman", "rm", "-f", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}