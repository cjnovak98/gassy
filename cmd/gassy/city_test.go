package main

import (
	"strconv"
	"testing"
	"time"
)

func TestParseCityWithSkills(t *testing.T) {
	data := `
[city]
name = "test-city"
version = "1"

[[agents]]
id = "engineer"
role = "cto"
runtime = "exec"
provider = "tmux"
cmd = "claude"
skills = ["code", "test", "review"]
budget = { monthly = 50.0 }

[network]
mayor_url = "http://localhost:8001"
engineer_url = "http://localhost:8002"

[runtime.defaults]
heartbeat_interval = "4h"
startup_timeout = "30s"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(city.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(city.Agents))
	}

	agent := city.GetAgent("engineer")
	if agent == nil {
		t.Fatal("GetAgent(\"engineer\") = nil")
	}
	if len(agent.Skills) != 3 {
		t.Errorf("len(agent.Skills) = %d, want 3", len(agent.Skills))
	}
	if agent.Runtime != "exec" {
		t.Errorf("agent.Runtime = %q, want %q", agent.Runtime, "exec")
	}
}

func TestParseCityEmpty(t *testing.T) {
	data := `
[city]
name = "empty-city"
version = "1"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if city.City.Name != "empty-city" {
		t.Errorf("City.Name = %q, want %q", city.City.Name, "empty-city")
	}
	if len(city.Agents) != 0 {
		t.Errorf("len(Agents) = %d, want 0", len(city.Agents))
	}
}

func TestGetAgentsBySkill(t *testing.T) {
	data := `
[city]
name = "test"
version = "1"

[[agents]]
id = "eng1"
skills = ["code", "test"]

[[agents]]
id = "eng2"
skills = ["code", "review"]

[[agents]]
id = "des1"
skills = ["design"]
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	codeAgents := city.GetAgentsBySkill("code")
	if len(codeAgents) != 2 {
		t.Errorf("len(GetAgentsBySkill(code)) = %d, want 2", len(codeAgents))
	}

	designAgents := city.GetAgentsBySkill("design")
	if len(designAgents) != 1 {
		t.Errorf("len(GetAgentsBySkill(design)) = %d, want 1", len(designAgents))
	}

	noneAgents := city.GetAgentsBySkill("nonexistent")
	if len(noneAgents) != 0 {
		t.Errorf("len(GetAgentsBySkill(nonexistent)) = %d, want 0", len(noneAgents))
	}
}

func TestDurationUnmarshal(t *testing.T) {
	d := &Duration{}
	err := d.UnmarshalText([]byte("4h"))
	if err != nil {
		t.Fatalf("UnmarshalText error = %v", err)
	}
	if d.Duration != 4*time.Hour {
		t.Errorf("Duration = %v, want 4h", d.Duration)
	}
}

func TestGetAgentNotFound(t *testing.T) {
	city := &City{}
	agent := city.GetAgent("nonexistent")
	if agent != nil {
		t.Errorf("GetAgent(nonexistent) = %v, want nil", agent)
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/city.toml")
	if err == nil {
		t.Error("ParseFile() for nonexistent file = nil, want error")
	}
}

func TestParseInvalidTOML(t *testing.T) {
	data := `[invalid toml that is not properly formatted`
	_, err := Parse([]byte(data))
	if err == nil {
		t.Error("Parse() with invalid TOML = nil, want error")
	}
}

func TestDurationUnmarshalInvalid(t *testing.T) {
	d := &Duration{}
	err := d.UnmarshalText([]byte("invalid"))
	if err == nil {
		t.Error("UnmarshalText() with invalid duration = nil, want error")
	}
}

func TestCityWithMultipleAgents(t *testing.T) {
	data := `
[city]
name = "multi-agent"
version = "1"

[[agents]]
id = "agent1"
role = "role1"

[[agents]]
id = "agent2"
role = "role2"

[[agents]]
id = "agent3"
role = "role3"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(city.Agents) != 3 {
		t.Errorf("len(Agents) = %d, want 3", len(city.Agents))
	}

	// Verify all agents can be retrieved
	for i := 1; i <= 3; i++ {
		agent := city.GetAgent("agent" + string(rune('0'+i)))
		if agent == nil {
			t.Errorf("GetAgent(agent%d) = nil", i)
		}
	}
}

func TestParseCityNetworkConfig(t *testing.T) {
	data := `
[city]
name = "network-test"
version = "1"

[network]
mayor_url = "http://localhost:8001"
engineer_url = "http://localhost:8002"
designer_url = "http://localhost:8003"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if city.Network.MayorURL != "http://localhost:8001" {
		t.Errorf("Network.MayorURL = %q, want %q", city.Network.MayorURL, "http://localhost:8001")
	}
	if city.Network.DesignerURL != "http://localhost:8003" {
		t.Errorf("Network.DesignerURL = %q, want %q", city.Network.DesignerURL, "http://localhost:8003")
	}
	if city.Network.EngineerURL != "http://localhost:8002" {
		t.Errorf("Network.EngineerURL = %q, want %q", city.Network.EngineerURL, "http://localhost:8002")
	}
}

func TestParseCityWithMinimalAgent(t *testing.T) {
	data := `
[city]
name = "minimal"
version = "1"

[[agents]]
id = "minimal-agent"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(city.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(city.Agents))
	}
	agent := city.GetAgent("minimal-agent")
	if agent == nil {
		t.Fatal("GetAgent(minimal-agent) = nil")
	}
	if agent.ID != "minimal-agent" {
		t.Errorf("agent.ID = %q, want %q", agent.ID, "minimal-agent")
	}
}

func TestGetAllAgents(t *testing.T) {
	data := `
[city]
name = "test"
version = "1"

[[agents]]
id = "agent1"
role = "role1"

[[agents]]
id = "agent2"
role = "role2"

[[agents]]
id = "agent3"
role = "role3"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	all := city.GetAllAgents()
	if len(all) != 3 {
		t.Errorf("len(GetAllAgents()) = %d, want 3", len(all))
	}

	// Verify all agents are returned
	ids := make(map[string]bool)
	for _, a := range all {
		ids[a.ID] = true
	}
	for i := 1; i <= 3; i++ {
		if !ids["agent"+strconv.Itoa(i)] {
			t.Errorf("agent%d not in GetAllAgents() result", i)
		}
	}
}

func TestGetAllAgentsEmpty(t *testing.T) {
	data := `
[city]
name = "empty"
version = "1"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	all := city.GetAllAgents()
	if len(all) != 0 {
		t.Errorf("len(GetAllAgents()) = %d, want 0", len(all))
	}
}

func TestBudgetConfig(t *testing.T) {
	data := `
[city]
name = "budget-test"
version = "1"

[[agents]]
id = "budget-agent"
budget = { monthly = 100.0 }
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	agent := city.GetAgent("budget-agent")
	if agent == nil {
		t.Fatal("GetAgent() returned nil")
	}
	if agent.Budget.Monthly != 100.0 {
		t.Errorf("Budget.Monthly = %f, want 100.0", agent.Budget.Monthly)
	}
}

func TestBudgetConfigValidate(t *testing.T) {
	budget := &BudgetConfig{Monthly: 100.0}
	if err := budget.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestBudgetConfigNegative(t *testing.T) {
	budget := &BudgetConfig{Monthly: -50.0}
	err := budget.Validate()
	if err == nil {
		t.Error("Validate() with negative budget = nil, want error")
	}
}

func TestParseCityWithNegativeBudget(t *testing.T) {
	data := `
[city]
name = "negative-budget"
version = "1"

[[agents]]
id = "bad-agent"
budget = { monthly = -100.0 }
`
	_, err := Parse([]byte(data))
	if err == nil {
		t.Error("Parse() with negative budget = nil, want error")
	}
}

func TestRuntimeDefaults(t *testing.T) {
	data := `
[city]
name = "runtime-test"
version = "1"

[runtime.defaults]
heartbeat_interval = "2h"
startup_timeout = "1m"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if city.Runtime.HeartbeatInterval.Duration != 2*time.Hour {
		t.Errorf("HeartbeatInterval = %v, want 2h", city.Runtime.HeartbeatInterval.Duration)
	}
	if city.Runtime.StartupTimeout.Duration != 1*time.Minute {
		t.Errorf("StartupTimeout = %v, want 1m", city.Runtime.StartupTimeout.Duration)
	}
}

func TestDurationUnmarshalText(t *testing.T) {
	d := &Duration{}
	err := d.UnmarshalText([]byte("30s"))
	if err != nil {
		t.Errorf("UnmarshalText(30s) error = %v", err)
	}
	if d.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", d.Duration)
	}
}

func TestDurationUnmarshalTextInvalid(t *testing.T) {
	d := &Duration{}
	err := d.UnmarshalText([]byte("invalid"))
	if err == nil {
		t.Error("UnmarshalText(invalid) = nil, want error")
	}
}

func TestDurationUnmarshalTextEmpty(t *testing.T) {
	d := &Duration{}
	err := d.UnmarshalText([]byte(""))
	// Empty duration is valid (0s)
	if err != nil {
		t.Errorf("UnmarshalText(empty) error = %v", err)
	}
	if d.Duration != 0 {
		t.Errorf("Duration = %v, want 0", d.Duration)
	}
}

func TestDurationUnmarshalTextHours(t *testing.T) {
	d := &Duration{}
	err := d.UnmarshalText([]byte("4h"))
	if err != nil {
		t.Errorf("UnmarshalText(4h) error = %v", err)
	}
	if d.Duration != 4*time.Hour {
		t.Errorf("Duration = %v, want 4h", d.Duration)
	}
}

func TestParseCityWithAllTaskStates(t *testing.T) {
	// Verify all agent states don't cause issues in parsing
	data := `
[city]
name = "all-agents"
version = "1"

[[agents]]
id = "agent1"
role = "role1"

[[agents]]
id = "agent2"
role = "role2"
`
	city, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(city.Agents) != 2 {
		t.Errorf("len(Agents) = %d, want 2", len(city.Agents))
	}
}
