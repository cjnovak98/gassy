package main

import (
	"github.com/cjnovak98/gassy/internal/city"
)

// Re-export types and functions from shared package
type (
	City            = city.City
	CityConfig      = city.CityConfig
	AgentConfig     = city.AgentConfig
	NetworkConfig   = city.NetworkConfig
	RuntimeDefaults = city.RuntimeDefaults
	Duration        = city.Duration
)