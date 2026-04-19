package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cjnovak98/gassy/internal/runtime"
	"github.com/cjnovak98/gassy/internal/supervisor"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [agent-id]",
	Short: "Start the city or a specific agent",
	Long:  "Start the supervisor reconcile loop, optionally for a specific agent",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStart,
}

var cityFile string

func init() {
	startCmd.Flags().StringVarP(&cityFile, "city", "c", "city.toml", "Path to city.toml")
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Validate city file exists before parsing
	if err := ensureCityFileExists(); err != nil {
		return err
	}

	// Parse city config
	city, err := ParseFile(cityFile)
	if err != nil {
		return fmt.Errorf("parsing city config: %w", err)
	}

	// Create supervisor with city config
	cfg := supervisor.DefaultConfig()
	s := supervisor.New(cfg)

	// Create runtime provider based on city config
	var prov runtime.Provider

	if len(args) == 1 {
		// Start specific agent
		agent := city.GetAgent(args[0])
		if agent.ID == "" {
			return fmt.Errorf("agent %q not found in city config", args[0])
		}

		prov = getProvider(agent.Provider)
		ctx := context.Background()

		if err := prov.Start(ctx, agent.ID, agent.Cmd); err != nil {
			return fmt.Errorf("starting agent %s: %w", agent.ID, err)
		}

		fmt.Printf("Started agent %s\n", agent.ID)
		return nil
	}

	// Start all agents via supervisor reconcile
	prov = runtime.NewExecProvider()

	ctx := context.Background()
	if err := s.Reconcile(ctx, prov); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	fmt.Printf("City %s started with %d agents\n", city.City.Name, len(city.Agents))
	return nil
}

func getProvider(providerType string) runtime.Provider {
	switch providerType {
	case "tmux":
		return runtime.NewTmuxProvider()
	case "exec":
		fallthrough
	default:
		return runtime.NewExecProvider()
	}
}

func ensureCityFileExists() error {
	if _, err := os.Stat(cityFile); os.IsNotExist(err) {
		return fmt.Errorf("city file %q not found", cityFile)
	}
	return nil
}