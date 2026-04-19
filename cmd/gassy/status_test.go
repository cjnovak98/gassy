package main

import (
	"testing"
)

func TestStatusCmdStructure(t *testing.T) {
	if statusCmd.Use != "status [agent-id]" {
		t.Errorf("statusCmd.Use = %q, want %q", statusCmd.Use, "status [agent-id]")
	}
	if statusCmd.Short == "" {
		t.Error("statusCmd.Short should not be empty")
	}
}

func TestStatusCmdMaxArgs(t *testing.T) {
	if statusCmd.Args != cobra.MaximumNArgs(1) {
		t.Error("statusCmd should accept 0 or 1 args")
	}
}

func TestStatusCmdHasRunE(t *testing.T) {
	if statusCmd.RunE == nil {
		t.Error("statusCmd.RunE should not be nil")
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
func TestRunStatusCommandStructure(t *testing.T) {
	if statusCmd.Use != "status [agent-id]" {
		t.Errorf("Use = %q, want %q", statusCmd.Use, "status [agent-id]")
	}
	if statusCmd.Args != cobra.MaximumNArgs(1) {
		t.Error("statusCmd should accept 0 or 1 args")
	}
	if statusCmd.RunE == nil {
		t.Error("statusCmd.RunE should not be nil")
	}
}

func TestStatusCmdHasNoFlags(t *testing.T) {
	flags := statusCmd.Flags()
	if flags.HasFlags() {
		t.Error("statusCmd should not have flags")
	}
}

func TestStopCmdStructure(t *testing.T) {
	if stopCmd.Use != "stop [agent-id]" {
		t.Errorf("Use = %q, want %q", stopCmd.Use, "stop [agent-id]")
	}
	if stopCmd.Args != cobra.MaximumNArgs(1) {
		t.Error("stopCmd should accept 0 or 1 args")
	}
	if stopCmd.RunE == nil {
		t.Error("stopCmd.RunE should not be nil")
	}
}

func TestStopCmdHasNoFlags(t *testing.T) {
	flags := stopCmd.Flags()
	if flags.HasFlags() {
		t.Error("stopCmd should not have flags")
	}
}

func TestRunStatusWithNonexistentCity(t *testing.T) {
	cityFile = "/nonexistent/city.toml"
	err := runStatus(&cobra.Command{}, []string{})
	if err == nil {
		t.Error("runStatus with nonexistent city should error")
	}
}

func TestRunStopWithNonexistentCity(t *testing.T) {
	cityFile = "/nonexistent/city.toml"
	err := runStop(&cobra.Command{}, []string{})
	if err == nil {
		t.Error("runStop with nonexistent city should error")
	}
}
