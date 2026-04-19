package main

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestStartCmdStructure(t *testing.T) {
	if startCmd.Use != "start [agent-id]" {
		t.Errorf("startCmd.Use = %q, want %q", startCmd.Use, "start [agent-id]")
	}
	if startCmd.Short == "" {
		t.Error("startCmd.Short should not be empty")
	}
}

func TestStartCmdMaxArgs(t *testing.T) {
	if startCmd.Args != cobra.MaximumNArgs(1) {
		t.Error("startCmd should accept 0 or 1 args")
	}
}

func TestStartCmdHasRunE(t *testing.T) {
	if startCmd.RunE == nil {
		t.Error("startCmd.RunE should not be nil")
	}
}

func TestStartCmdHasCityFlag(t *testing.T) {
	flag := startCmd.Flags().Lookup("city")
	if flag == nil {
		t.Error("startCmd should have --city flag")
	}
	if flag.DefValue != "city.toml" {
		t.Errorf("city flag default = %q, want %q", flag.DefValue, "city.toml")
	}
}

func TestEnsureCityFileExistsWithValidFile(t *testing.T) {
	// Create a temp file
	tmpfile, err := os.CreateTemp("", "city.toml")
	if err != nil {
		t.Fatalf("CreateTemp error: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	cityFile = tmpfile.Name()
	if err := ensureCityFileExists(); err != nil {
		t.Errorf("ensureCityFileExists() with valid file = %v, want nil", err)
	}
}

func TestEnsureCityFileExistsWithMissingFile(t *testing.T) {
	cityFile = "/nonexistent/path/city.toml"
	if err := ensureCityFileExists(); err == nil {
		t.Error("ensureCityFileExists() with nonexistent file = nil, want error")
	}
}

func TestGetProviderAllTypes(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{"tmux", "tmux"},
		{"exec", "exec"},
		{"unknown defaults to exec", "unknown"},
		{"empty defaults to exec", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov := getProvider(tt.provider)
			if prov == nil {
				t.Fatalf("getProvider(%q) returned nil", tt.provider)
			}
		})
	}
}

func TestRunStartWithMissingCityFile(t *testing.T) {
	// Save original cityFile
	originalCityFile := cityFile
	defer func() { cityFile = originalCityFile }()

	cityFile = "/nonexistent/city.toml"

	err := ensureCityFileExists()
	if err == nil {
		t.Error("expected error when city file does not exist")
	}
}