package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gassy-admin",
	Short: "Gassy Admin - Ops and container management",
	Long:  "Operations CLI for managing containers, supervisor, and agent lifecycle",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}