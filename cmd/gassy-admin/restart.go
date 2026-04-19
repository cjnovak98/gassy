package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart [container-id]",
	Short: "Restart a gassy container",
	Long:  "Stop and start a specific gassy container",
	Args:  cobra.ExactArgs(1),
	RunE:  runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)
}

func runRestart(cmd *cobra.Command, args []string) error {
	containerName := args[0]
	if !strings.HasPrefix(containerName, "gassy-") {
		containerName = "gassy-" + containerName
	}

	// Stop the container
	fmt.Printf("Stopping %s...\n", containerName)
	if err := stopContainer(containerName); err != nil {
		fmt.Printf("Warning: could not stop %s: %v\n", containerName, err)
	}

	// Start the container
	fmt.Printf("Starting %s...\n", containerName)
	cmdExec := exec.Command("podman", "start", containerName)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	fmt.Printf("Restarted: %s\n", containerName)
	return nil
}