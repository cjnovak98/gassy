package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List all gassy containers",
	Long:  "List all gassy containers using podman ps",
	RunE:  runPs,
}

func init() {
	rootCmd.AddCommand(psCmd)
}

func runPs(cmd *cobra.Command, args []string) error {
	// Show all gassy containers with more detail
	cmdExec := exec.Command("podman", "ps", "--filter", "label=gassy=true")
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	return cmdExec.Run()
}