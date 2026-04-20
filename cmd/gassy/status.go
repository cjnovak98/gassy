package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [agent-id]",
	Short: "Check agent status",
	Long:  "Show status of all agents or a specific agent",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	city, err := ParseFile(cityFile)
	if err != nil {
		return fmt.Errorf("parsing city config: %w", err)
	}

	ctx := context.Background()

	if len(args) == 1 {
		// Status for specific agent
		agent := city.GetAgent(args[0])
		if agent.ID == "" {
			return fmt.Errorf("agent %q not found in city config", args[0])
		}

		prov := getProvider(agent.Runtime)
		st, err := prov.Status(ctx, agent.ID)
		if err != nil {
			fmt.Printf("Agent %s: error - %v\n", agent.ID, err)
			return nil
		}

		if st.Alive {
			fmt.Printf("Agent %s: alive (PID %d)\n", agent.ID, st.PID)
		} else {
			fmt.Printf("Agent %s: not running\n", agent.ID)
		}
		return nil
	}

	// Status for all agents
	fmt.Printf("City: %s\n", city.City.Name)
	fmt.Printf("%-20s %-10s %-10s %-10s\n", "AGENT ID", "PROVIDER", "STATUS", "PID")
	fmt.Println("------------------------------------------------------------")

	for _, agent := range city.Agents {
		prov := getProvider(agent.Runtime)
		st, err := prov.Status(ctx, agent.ID)

		status := "unknown"
		pid := "-"

		if err != nil {
			status = "error"
		} else if st.Alive {
			status = "alive"
			if st.PID > 0 {
				pid = fmt.Sprintf("%d", st.PID)
			}
		} else {
			status = "stopped"
		}

		fmt.Printf("%-20s %-10s %-10s %-10s\n", agent.ID, agent.Runtime, status, pid)
	}

	return nil
}