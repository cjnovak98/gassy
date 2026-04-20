package city

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// City represents the top-level city configuration
type City struct {
	City    CityConfig      `toml:"city"`
	Agents  []AgentConfig   `toml:"agents"`
	Network NetworkConfig   `toml:"network"`
	Runtime RuntimeDefaults `toml:"runtime.defaults"`
}

// CityConfig contains city metadata
type CityConfig struct {
	Name     string         `toml:"name"`
	Version  string         `toml:"version"`
	Runtime  RuntimeConfig  `toml:"runtime"`
}

// RuntimeConfig contains runtime settings
type RuntimeConfig struct {
	PortRange          PortRange `toml:"port_range"`
	AgentImage         string    `toml:"agent_image"`
	HeartbeatInterval  Duration  `toml:"heartbeat_interval"`
	StartupTimeout     Duration  `toml:"startup_timeout"`
}

// PortRange defines the range of ports available for agent allocation
type PortRange struct {
	Min int `toml:"min"`
	Max int `toml:"max"`
}

// AgentConfig represents an agent in the city
type AgentConfig struct {
	ID      string   `toml:"id"`
	Role    string   `toml:"role"`
	Runtime string   `toml:"runtime"`
	Cmd     string   `toml:"cmd"`
	Skills  []string `toml:"skills,omitempty"`
}

// NetworkConfig represents agent network endpoints
type NetworkConfig struct {
	MayorURL    string `toml:"mayor_url"`
	EngineerURL string `toml:"engineer_url,omitempty"`
	DesignerURL string `toml:"designer_url,omitempty"`
}

// RuntimeDefaults contains default runtime settings
type RuntimeDefaults struct {
	HeartbeatInterval Duration `toml:"heartbeat_interval"`
	StartupTimeout    Duration `toml:"startup_timeout"`
}

// Duration is a TOML duration wrapper
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler
func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	return nil
}

// ParseFile parses a city.toml file
func ParseFile(path string) (*City, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading city.toml: %w", err)
	}
	return Parse(data)
}

// Parse parses city.toml content
func Parse(data []byte) (*City, error) {
	var city City
	if err := toml.Unmarshal(data, &city); err != nil {
		return nil, fmt.Errorf("parsing city.toml: %w", err)
	}
	return &city, nil
}

// GetAgent returns an agent config by ID
func (c *City) GetAgent(id string) AgentConfig {
	for i := range c.Agents {
		if c.Agents[i].ID == id {
			return c.Agents[i]
		}
	}
	return AgentConfig{}
}

// GetAgentsBySkill returns all agents with a given skill
func (c *City) GetAgentsBySkill(skill string) []*AgentConfig {
	var agents []*AgentConfig
	for i := range c.Agents {
		for _, s := range c.Agents[i].Skills {
			if s == skill {
				agents = append(agents, &c.Agents[i])
				break
			}
		}
	}
	return agents
}

// GetAllAgents returns all agent configs
func (c *City) GetAllAgents() []AgentConfig {
	return c.Agents
}