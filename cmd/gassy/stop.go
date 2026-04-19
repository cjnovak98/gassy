package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [agent-id]",
	Short: "Stop the city or a specific agent",
	Long:  "Stop all running agents or a specific agent by ID",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	// Parse city config to get provider info
	city, err := ParseFile(cityFile)
	if err != nil {
		return fmt.Errorf("parsing city config: %w", err)
	}

	ctx := context.Background()

	if len(args) == 1 {
		// Stop specific agent
		agent := city.GetAgent(args[0])
		if agent.ID == "" {
			return fmt.Errorf("agent %q not found in city config", args[0])
		}

		prov := getProvider(agent.Provider)
		if err := prov.Stop(ctx, agent.ID); err != nil {
			return fmt.Errorf("stopping agent %s: %w", agent.ID, err)
		}

		fmt.Printf("Stopped agent %s\n", agent.ID)
		return nil
	}

	// Stop all agents
	for _, agent := range city.Agents {
		prov := getProvider(agent.Provider)
		if err := prov.Stop(ctx, agent.ID); err != nil {
			// Agent might not be running, that's ok
			continue
		}
		fmt.Printf("Stopped agent %s\n", agent.ID)
	}

	fmt.Printf("City %s stopped\n", city.City.Name)
	return nil
}