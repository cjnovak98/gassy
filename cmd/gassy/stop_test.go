package main

import (
	"testing"
)

func TestStopCmdStructure(t *testing.T) {
	if stopCmd.Use != "stop [agent-id]" {
		t.Errorf("stopCmd.Use = %q, want %q", stopCmd.Use, "stop [agent-id]")
	}
	if stopCmd.Short == "" {
		t.Error("stopCmd.Short should not be empty")
	}
}

func TestStopCmdMaxArgs(t *testing.T) {
	if stopCmd.Args != cobra.MaximumNArgs(1) {
		t.Error("stopCmd should accept 0 or 1 args")
	}
}

func TestStopCmdHasRunE(t *testing.T) {
	if stopCmd.RunE == nil {
		t.Error("stopCmd.RunE should not be nil")
	}
}

func TestEnsureCityFileExists(t *testing.T) {
	// Should return error for nonexistent file
	err := ensureCityFileExists("/nonexistent/city.toml")
	if err == nil {
		t.Error("ensureCityFileExists() for nonexistent file = nil, want error")
	}
}