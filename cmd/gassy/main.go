package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gassy",
	Short: "Gassy - A2A-native orchestration platform",
	Long:  "An orchestration platform combining Gas City's model with the A2A protocol",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
