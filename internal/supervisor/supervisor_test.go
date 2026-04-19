package supervisor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cjnovak98/gassy/internal/runtime"
)

// mockRuntime is a simple mock for testing
type mockRuntime struct {
	agents map[string]bool
}

func (m *mockRuntime) Start(ctx context.Context, agentID, cmd string) error {
	m.agents[agentID] = true
	return nil
}

func (m *mockRuntime) Stop(ctx context.Context, agentID string) error {
	delete(m.agents, agentID)
	return nil
}

func (m *mockRuntime) Status(ctx context.Context, agentID string) (runtime.Status, error) {
	if !m.agents[agentID] {
		return runtime.Status{ID: agentID, Alive: false}, runtime.ErrAgentNotFound
	}
	return runtime.Status{ID: agentID, Alive: true}, nil
}

// mockCity is a mock city for testing
type mockCity struct {
	agents []AgentConfig
}

func (m *mockCity) GetAgent(id string) *AgentConfig {
	for i := range m.agents {
		if m.agents[i].ID == id {
			return &m.agents[i]
		}
	}
	return nil
}

func (m *mockCity) GetAllAgents() []AgentConfig {
	return m.agents
}

func TestReconcileWithoutCity(t *testing.T) {
	s := New(DefaultConfig())
	mock := &mockRuntime{agents: make(map[string]bool)}
	err := s.Reconcile(context.Background(), mock)
	if err == nil {
		t.Error("Reconcile() error = nil, want error when city not configured")
	}
}

func TestReconcileStartsStoppedAgents(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	// Create a mock city that returns agents
	city := &mockCity{
		agents: []AgentConfig{
			{ID: "agent-1", Role: "test", Provider: "exec", Cmd: "echo test", Budget: 0},
		},
	}
	s.SetCity(city)

	mock := &mockRuntime{agents: make(map[string]bool)}

	err := s.Reconcile(context.Background(), mock)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !mock.agents["agent-1"] {
		t.Error("agent-1 should be started by Reconcile")
	}
}

func TestReconcileStopsUnknownAgents(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	// City with no agents
	city := &mockCity{agents: []AgentConfig{}}
	s.SetCity(city)

	mock := &mockRuntime{agents: map[string]bool{"ghost-agent": true}}

	err := s.Reconcile(context.Background(), mock)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if mock.agents["ghost-agent"] {
		t.Error("ghost-agent should be stopped by Reconcile")
	}
}

func TestCheckAgentsWithRuntime(t *testing.T) {
	s := New(DefaultConfig())
	s.StartAgent(context.Background(), "test-agent")

	mock := &mockRuntime{agents: map[string]bool{"test-agent": true}}

	status := s.CheckAgents(context.Background(), mock)
	if status["test-agent"] == nil {
		t.Error("CheckAgents() should return status for test-agent")
	}
	if !status["test-agent"].Alive {
		t.Error("test-agent should be alive according to runtime")
	}
}

func TestCheckAgentsUnknown(t *testing.T) {
	s := New(DefaultConfig())

	mock := &mockRuntime{agents: make(map[string]bool)}

	status := s.CheckAgents(context.Background(), mock)
	if len(status) != 0 {
		t.Error("CheckAgents() should return empty for unknown agent")
	}
}

func TestAgentCount(t *testing.T) {
	s := New(DefaultConfig())

	if s.AgentCount() != 0 {
		t.Errorf("AgentCount() = %d, want 0", s.AgentCount())
	}

	s.StartAgent(context.Background(), "agent-1")
	if s.AgentCount() != 1 {
		t.Errorf("AgentCount() = %d, want 1", s.AgentCount())
	}

	s.StartAgent(context.Background(), "agent-2")
	if s.AgentCount() != 2 {
		t.Errorf("AgentCount() = %d, want 2", s.AgentCount())
	}
}

func TestGetStatus(t *testing.T) {
	s := New(DefaultConfig())

	status := s.GetStatus("nonexistent")
	if status != nil {
		t.Error("GetStatus() for nonexistent should return nil")
	}

	s.StartAgent(context.Background(), "test-agent")
	status = s.GetStatus("test-agent")
	if status == nil {
		t.Fatal("GetStatus() for existing agent should not return nil")
	}
	if status.ID != "test-agent" {
		t.Errorf("status.ID = %q, want %q", status.ID, "test-agent")
	}
	if !status.Alive {
		t.Error("status.Alive should be true after StartAgent")
	}
}

func TestStopAgent(t *testing.T) {
	s := New(DefaultConfig())

	s.StartAgent(context.Background(), "test-agent")
	if s.AgentCount() != 1 {
		t.Fatalf("AgentCount() = %d, want 1", s.AgentCount())
	}

	err := s.StopAgent(context.Background(), "test-agent")
	if err != nil {
		t.Fatalf("StopAgent() error = %v", err)
	}

	if s.AgentCount() != 0 {
		t.Errorf("AgentCount() after StopAgent = %d, want 0", s.AgentCount())
	}

	status := s.GetStatus("test-agent")
	if status != nil {
		t.Error("GetStatus() after StopAgent should return nil")
	}
}

func TestReconcileWithStopError(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	city := &mockCity{agents: []AgentConfig{}}
	s.SetCity(city)

	// Mock runtime that returns error on Stop
	mockWithStopError := &mockRuntimeWithError{
		agents:       map[string]bool{"ghost-agent": true},
		stopError:    true,
	}

	err := s.Reconcile(context.Background(), mockWithStopError)
	// Reconcile should still complete even if Stop fails
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Agent should still be removed from status map (hygiene fix)
	if mockWithStopError.agents["ghost-agent"] {
		t.Error("ghost-agent should be removed from mock agents")
	}
}

type mockRuntimeWithError struct {
	agents    map[string]bool
	stopError bool
}

func (m *mockRuntimeWithError) Start(ctx context.Context, agentID, cmd string) error {
	m.agents[agentID] = true
	return nil
}

func (m *mockRuntimeWithError) Stop(ctx context.Context, agentID string) error {
	delete(m.agents, agentID)
	if m.stopError {
		return runtime.ErrAgentNotRunning
	}
	return nil
}

func (m *mockRuntimeWithError) Status(ctx context.Context, agentID string) (runtime.Status, error) {
	if !m.agents[agentID] {
		return runtime.Status{ID: agentID, Alive: false}, runtime.ErrAgentNotFound
	}
	return runtime.Status{ID: agentID, Alive: true}, nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.HeartbeatInterval != 4*time.Hour {
		t.Errorf("HeartbeatInterval = %v, want 4h", cfg.HeartbeatInterval)
	}
	if cfg.StartupTimeout != 30*time.Second {
		t.Errorf("StartupTimeout = %v, want 30s", cfg.StartupTimeout)
	}
	if cfg.ReconcileInterval != 1*time.Minute {
		t.Errorf("ReconcileInterval = %v, want 1m", cfg.ReconcileInterval)
	}
}

func TestNewWithNilConfig(t *testing.T) {
	s := New(nil)
	if s.config == nil {
		t.Error("config should not be nil with nil input")
	}
	if s.config.HeartbeatInterval != 4*time.Hour {
		t.Errorf("default HeartbeatInterval = %v, want 4h", s.config.HeartbeatInterval)
	}
}

func TestSetCity(t *testing.T) {
	s := New(DefaultConfig())
	city := &mockCity{
		agents: []AgentConfig{
			{ID: "test-agent", Role: "test", Provider: "exec", Cmd: "echo", Budget: 0},
		},
	}
	s.SetCity(city)

	if s.city == nil {
		t.Error("city should not be nil after SetCity")
	}
}

func TestGetCityAgentsNil(t *testing.T) {
	s := New(DefaultConfig())
	// City not set
	agents := s.getCityAgents()
	if agents != nil {
		t.Errorf("getCityAgents() with nil city = %v, want nil", agents)
	}
}

func TestGetCityAgentsWithCity(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	city := &mockCity{
		agents: []AgentConfig{
			{ID: "agent-1", Role: "role1", Provider: "exec", Cmd: "cmd1", Budget: 50.0},
			{ID: "agent-2", Role: "role2", Provider: "tmux", Cmd: "cmd2", Budget: 75.0},
		},
	}
	s.SetCity(city)

	agents := s.getCityAgents()
	if len(agents) != 2 {
		t.Errorf("len(getCityAgents()) = %d, want 2", len(agents))
	}
	if agents[0].ID != "agent-1" {
		t.Errorf("agents[0].ID = %q, want %q", agents[0].ID, "agent-1")
	}
	if agents[0].Budget != 50.0 {
		t.Errorf("agents[0].Budget = %f, want 50.0", agents[0].Budget)
	}
}

func TestReconcileWithMultipleAgents(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	city := &mockCity{
		agents: []AgentConfig{
			{ID: "agent-1", Role: "role1", Provider: "exec", Cmd: "cmd1", Budget: 0},
			{ID: "agent-2", Role: "role2", Provider: "exec", Cmd: "cmd2", Budget: 0},
			{ID: "agent-3", Role: "role3", Provider: "exec", Cmd: "cmd3", Budget: 0},
		},
	}
	s.SetCity(city)

	mock := &mockRuntime{agents: make(map[string]bool)}
	err := s.Reconcile(context.Background(), mock)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	for _, id := range []string{"agent-1", "agent-2", "agent-3"} {
		if !mock.agents[id] {
			t.Errorf("agent %s should be started", id)
		}
	}
}

func TestReconcileSkipsRunningAgents(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	city := &mockCity{
		agents: []AgentConfig{
			{ID: "agent-1", Role: "role1", Provider: "exec", Cmd: "cmd1", Budget: 0},
		},
	}
	s.SetCity(city)

	// Agent already running
	mock := &mockRuntime{agents: map[string]bool{"agent-1": true}}
	err := s.Reconcile(context.Background(), mock)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// agent-1 should still be in mock.agents (not restarted)
	if !mock.agents["agent-1"] {
		t.Error("agent-1 should still be running")
	}
}

func TestReconcileSkipsOverBudgetAgents(t *testing.T) {
	cfg := DefaultConfig()
	s := New(cfg)

	city := &mockCity{
		agents: []AgentConfig{
			{ID: "agent-1", Role: "role1", Provider: "exec", Cmd: "cmd1", Budget: 50.0},
		},
	}
	s.SetCity(city)

	// Record spending that puts agent over budget
	s.RecordSpent("agent-1", 50.0)

	mock := &mockRuntime{agents: make(map[string]bool)}
	err := s.Reconcile(context.Background(), mock)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// agent-1 should NOT be started because it's over budget
	if mock.agents["agent-1"] {
		t.Error("agent-1 should NOT be started when over budget")
	}

	// Verify status shows over budget
	status := s.GetStatus("agent-1")
	if status == nil {
		t.Fatal("GetStatus() should return status for over-budget agent")
	}
	if status.Error != "over budget" {
		t.Errorf("status.Error = %q, want %q", status.Error, "over budget")
	}
}

func TestBudgetSpentAndRemaining(t *testing.T) {
	s := New(DefaultConfig())

	// Initially no spending
	spent, remaining, over := s.GetBudgetStatus("agent-1", 100.0)
	if spent != 0 {
		t.Errorf("spent = %f, want 0", spent)
	}
	if remaining != 100.0 {
		t.Errorf("remaining = %f, want 100.0", remaining)
	}
	if over {
		t.Error("overBudget = true, want false")
	}

	// Record some spending
	s.RecordSpent("agent-1", 30.0)
	spent, remaining, over = s.GetBudgetStatus("agent-1", 100.0)
	if spent != 30.0 {
		t.Errorf("spent = %f, want 30.0", spent)
	}
	if remaining != 70.0 {
		t.Errorf("remaining = %f, want 70.0", remaining)
	}
	if over {
		t.Error("overBudget = true, want false")
	}

	// Push to exactly budget
	s.RecordSpent("agent-1", 70.0)
	spent, remaining, over = s.GetBudgetStatus("agent-1", 100.0)
	if spent != 100.0 {
		t.Errorf("spent = %f, want 100.0", spent)
	}
	if remaining != 0.0 {
		t.Errorf("remaining = %f, want 0.0", remaining)
	}
	if !over {
		t.Error("overBudget = false, want true when at budget")
	}
}

func TestStartAgent(t *testing.T) {
	s := New(DefaultConfig())
	err := s.StartAgent(context.Background(), "new-agent")
	if err != nil {
		t.Fatalf("StartAgent() error = %v", err)
	}

	status := s.GetStatus("new-agent")
	if status == nil {
		t.Fatal("GetStatus() should return status for started agent")
	}
	if !status.Alive {
		t.Error("status.Alive should be true")
	}
}

func TestAgentStatus(t *testing.T) {
	status := &AgentStatus{
		ID:        "test-agent",
		Alive:     true,
		LastSeen:  time.Now(),
		TaskCount: 5,
		Error:     "",
	}
	if status.ID != "test-agent" {
		t.Errorf("ID = %q, want %q", status.ID, "test-agent")
	}
	if status.Alive != true {
		t.Error("Alive should be true")
	}
	if status.TaskCount != 5 {
		t.Errorf("TaskCount = %d, want 5", status.TaskCount)
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := &Config{
		HeartbeatInterval: 2 * time.Hour,
		StartupTimeout:   1 * time.Minute,
		ReconcileInterval: 30 * time.Second,
	}
	if cfg.HeartbeatInterval != 2*time.Hour {
		t.Errorf("HeartbeatInterval = %v, want 2h", cfg.HeartbeatInterval)
	}
	if cfg.StartupTimeout != 1*time.Minute {
		t.Errorf("StartupTimeout = %v, want 1m", cfg.StartupTimeout)
	}
	if cfg.ReconcileInterval != 30*time.Second {
		t.Errorf("ReconcileInterval = %v, want 30s", cfg.ReconcileInterval)
	}
}

func TestBudgetTrackerConcurrentRecordSpent(t *testing.T) {
	s := New(DefaultConfig())
	var wg sync.WaitGroup

	// 100 goroutines each recording 10 spend
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RecordSpent("agent-1", 10.0)
		}()
	}
	wg.Wait()

	spent, _, _ := s.GetBudgetStatus("agent-1", 10000.0)
	if spent != 1000.0 {
		t.Errorf("spent = %f, want 1000.0 after 100 concurrent RecordSpent calls", spent)
	}
}

func TestBudgetTrackerConcurrentGetStatus(t *testing.T) {
	s := New(DefaultConfig())
	s.RecordSpent("agent-1", 100.0)

	var wg sync.WaitGroup
	// Concurrent reads while also writing
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.GetBudgetStatus("agent-1", 1000.0)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RecordSpent("agent-1", 1.0)
		}()
	}
	wg.Wait()

	// Should not crash and should have recorded all spending
	spent, _, _ := s.GetBudgetStatus("agent-1", 10000.0)
	if spent < 150.0 {
		t.Errorf("spent = %f, want at least 150.0", spent)
	}
}
