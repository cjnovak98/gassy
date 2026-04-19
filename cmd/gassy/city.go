package main

import (
	"fmt"
	"os"

	"github.com/cjnovak98/gassy/internal/city"
	"github.com/BurntSushi/toml"
)

// Re-export types from shared package
type (
	City            = city.City
	CityConfig      = city.CityConfig
	AgentConfig     = city.AgentConfig
	BudgetConfig    = city.BudgetConfig
	NetworkConfig   = city.NetworkConfig
	RuntimeDefaults = city.RuntimeDefaults
	Duration        = city.Duration
)

// ParseFile parses a city.toml file
var ParseFile = city.ParseFile

// Parse parses city.toml content
var Parse = city.Parse

// WriteFile marshals the City config and writes it to the specified path
func WriteFile(c *City, path string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling city config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing city config: %w", err)
	}
	return nil
}