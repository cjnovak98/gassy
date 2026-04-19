package main

import (
	"github.com/cjnovak98/gassy/cmd/gassy-admin/supervisor"
)

func init() {
	rootCmd.AddCommand(supervisor.SupervisorCmd)
}