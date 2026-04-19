package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [container-id]",
	Short: "Show status of gassy containers",
	Long:  "Show status of all gassy containers or a specific container",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		// Show specific container
		containerName := args[0]
		if !strings.HasPrefix(containerName, "gassy-") {
			containerName = "gassy-" + containerName
		}
		return showContainerStatus(containerName)
	}

	// Show all gassy containers using podman ps
	cmdExec := exec.Command("podman", "ps", "--filter", "label=gassy=true", "--format", "table {{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}")
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	return cmdExec.Run()
}

func showContainerStatus(name string) error {
	cmdExec := exec.Command("podman", "ps", "--filter", "name="+name, "--format", "table {{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}")
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	return cmdExec.Run()
}