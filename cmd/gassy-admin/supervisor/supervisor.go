package supervisor

import (
	"github.com/spf13/cobra"
)

var SupervisorCmd = &cobra.Command{
	Use:   "supervisor",
	Short: "Supervisor management",
	Long:  "Manage the supervisor daemon",
}