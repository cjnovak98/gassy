package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs [container-id]",
	Short: "Tail logs from a container",
	Long:  "Tail logs from a container using podman logs -f",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	containerName := args[0]
	if !strings.HasPrefix(containerName, "gassy-") {
		containerName = "gassy-" + containerName
	}

	// Use podman logs -f <container-name>
	cmdExec := exec.Command("podman", "logs", "-f", containerName)
	cmdExec.Stdin = os.Stdin
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	fmt.Printf("Tailing logs for container: %s\n", containerName)
	if err := cmdExec.Run(); err != nil {
		return fmt.Errorf("podman logs failed: %w", err)
	}

	return nil
}