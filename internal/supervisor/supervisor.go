package supervisor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cjnovak98/gassy/internal/runtime"
)

// Config holds supervisor configuration
type Config struct {
	HeartbeatInterval time.Duration
	StartupTimeout    time.Duration
	ReconcileInterval time.Duration
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		HeartbeatInterval: 4 * time.Hour,
		StartupTimeout:    30 * time.Second,
		ReconcileInterval: 1 * time.Minute,
	}
}

// AgentStatus represents the current state of an agent
type AgentStatus struct {
	ID        string
	Alive     bool
	LastSeen  time.Time
	TaskCount int
	Error     string
}

// Supervisor manages agent lifecycle and reconciliation
type Supervisor struct {
	config *Config
	city   CityGetter
	budget *budgetTracker

	mu          sync.RWMutex
	agentStatus map[string]*AgentStatus
	portRange   PortRange
	usedPorts   map[int]bool
	agentPorts  map[string]int // agentID -> port
}

// AgentConfig is a subset of city.AgentConfig for the supervisor interface
type AgentConfig struct {
	ID       string
	Role     string
	Provider string
	Cmd      string
	Budget   float64 // Monthly budget limit in dollars
}

// CityGetter is implemented by city configuration
type CityGetter interface {
	GetAgent(id string) AgentConfig
	GetAllAgents() []AgentConfig
}

// New creates a new Supervisor
func New(cfg *Config) *Supervisor {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Supervisor{
		config:      cfg,
		agentStatus: make(map[string]*AgentStatus),
		budget:      newBudgetTracker(),
		usedPorts:   make(map[int]bool),
		agentPorts:  make(map[string]int),
	}
}

// PortRange defines available ports for agent allocation
type PortRange struct {
	Min int
	Max int
}

// SetPortRange sets the port range for agent allocation
func (s *Supervisor) SetPortRange(pr PortRange) {
	s.portRange = pr
}

// allocatePort finds and reserves the first available port
func (s *Supervisor) allocatePort() (int, error) {
	for port := s.portRange.Min; port <= s.portRange.Max; port++ {
		if !s.usedPorts[port] {
			s.usedPorts[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", s.portRange.Min, s.portRange.Max)
}

// releasePort releases a port back to the available pool
func (s *Supervisor) releasePort(port int) {
	delete(s.usedPorts, port)
}

// Hire starts an agent with an allocated port
func (s *Supervisor) Hire(agentID string, runtime interface {
	Start(ctx context.Context, agentID, image string, port int, env []string) error
}) error {
	port, err := s.allocatePort()
	if err != nil {
		return err
	}

	if err := runtime.Start(context.Background(), agentID, "", port, nil); err != nil {
		s.releasePort(port)
		return err
	}

	s.mu.Lock()
	s.agentPorts[agentID] = port
	s.mu.Unlock()

	return nil
}

// Fire stops an agent and releases its port
func (s *Supervisor) Fire(agentID string, runtime interface {
	Stop(ctx context.Context, agentID string) error
}) error {
	s.mu.Lock()
	port, ok := s.agentPorts[agentID]
	if ok {
		delete(s.agentPorts, agentID)
	}
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	if err := runtime.Stop(context.Background(), agentID); err != nil {
		return err
	}

	s.releasePort(port)
	return nil
}

// SetCity sets the city configuration
func (s *Supervisor) SetCity(city CityGetter) {
	s.city = city
}

// CheckAgents verifies agent health via runtime provider
func (s *Supervisor) CheckAgents(ctx context.Context, runtime interface {
	Status(ctx context.Context, agentID string) (runtime.Status, error)
}) map[string]*AgentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := make(map[string]*AgentStatus)
	for id := range s.agentStatus {
		st, err := runtime.Status(ctx, id)
		status[id] = &AgentStatus{
			ID:        id,
			Alive:     err == nil && st.Alive,
			LastSeen:  s.agentStatus[id].LastSeen,
			TaskCount: s.agentStatus[id].TaskCount,
			Error:     st.Error,
		}
	}
	return status
}

// Reconcile ensures desired state matches actual state
func (s *Supervisor) Reconcile(ctx context.Context, runtime interface {
	Start(ctx context.Context, agentID, cmd string) error
	Stop(ctx context.Context, agentID string) error
	Status(ctx context.Context, agentID string) (runtime.Status, error)
}) error {
	if s.city == nil {
		return fmt.Errorf("city not configured")
	}

	// Get all city agents
	cityAgents := s.getCityAgents()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Track which agents are in desired state
	runningAgents := make(map[string]bool)

	// Check each city agent
	for _, agent := range cityAgents {
		st, err := runtime.Status(ctx, agent.ID)
		running := err == nil && st.Alive

		if running {
			runningAgents[agent.ID] = true
			// Update last seen
			if existing, ok := s.agentStatus[agent.ID]; ok {
				existing.LastSeen = time.Now()
			}
		} else {
			// Check budget before starting
			if s.budget.IsOverBudget(agent.ID, agent.Budget) {
				s.agentStatus[agent.ID] = &AgentStatus{
					ID:        agent.ID,
					Alive:     false,
					LastSeen:  time.Now(),
					TaskCount: 0,
					Error:     "over budget",
				}
				continue
			}

			// Agent not running - start it
			if err := runtime.Start(ctx, agent.ID, agent.Cmd); err != nil {
				s.agentStatus[agent.ID] = &AgentStatus{
					ID:        agent.ID,
					Alive:     false,
					LastSeen:  time.Now(),
					TaskCount: 0,
					Error:     err.Error(),
				}
			} else {
				s.agentStatus[agent.ID] = &AgentStatus{
					ID:        agent.ID,
					Alive:     true,
					LastSeen:  time.Now(),
					TaskCount: 0,
				}
				runningAgents[agent.ID] = true
			}
		}
	}

	// Stop agents not in city config
	for id := range s.agentStatus {
		if !runningAgents[id] {
			if err := runtime.Stop(ctx, id); err != nil {
				// Log but continue - agent status will be cleaned up
				s.agentStatus[id].Error = err.Error()
			}
			delete(s.agentStatus, id)
		}
	}

	return nil
}

// getCityAgents returns all agents from the city config
func (s *Supervisor) getCityAgents() []AgentConfig {
	if s.city == nil {
		return nil
	}
	return s.city.GetAllAgents()
}

// StartAgent starts an agent and updates its status
func (s *Supervisor) StartAgent(ctx context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.agentStatus[agentID] = &AgentStatus{
		ID:       agentID,
		Alive:    true,
		LastSeen: time.Now(),
	}
	return nil
}

// StopAgent stops an agent and removes its status
func (s *Supervisor) StopAgent(ctx context.Context, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.agentStatus, agentID)
	return nil
}

// AgentCount returns the number of managed agents
func (s *Supervisor) AgentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agentStatus)
}

// GetStatus returns the status of a specific agent
func (s *Supervisor) GetStatus(agentID string) *AgentStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agentStatus[agentID]
}

// RecordSpent records spending for an agent
func (s *Supervisor) RecordSpent(agentID string, amount float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.budget.RecordSpent(agentID, amount)
}

// GetBudgetStatus returns budget status for an agent
func (s *Supervisor) GetBudgetStatus(agentID string, limit float64) (spent, remaining float64, overBudget bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spent = s.budget.GetSpent(agentID)
	remaining = s.budget.GetRemaining(agentID, limit)
	overBudget = s.budget.IsOverBudget(agentID, limit)
	return
}

// BudgetTracker tracks spending for agents
type BudgetTracker interface {
	GetSpent(agentID string) float64
}

// budgetTracker is a simple in-memory budget tracker
type budgetTracker struct {
	spent map[string]float64
}

func newBudgetTracker() *budgetTracker {
	return &budgetTracker{spent: make(map[string]float64)}
}

func (b *budgetTracker) GetSpent(agentID string) float64 {
	return b.spent[agentID]
}

// RecordSpent records spending for an agent
func (b *budgetTracker) RecordSpent(agentID string, amount float64) {
	b.spent[agentID] += amount
}

// GetRemaining returns remaining budget for an agent
func (b *budgetTracker) GetRemaining(agentID string, limit float64) float64 {
	spent := b.spent[agentID]
	if spent >= limit {
		return 0
	}
	return limit - spent
}

// IsOverBudget returns true if agent has exceeded budget
func (b *budgetTracker) IsOverBudget(agentID string, limit float64) bool {
	return b.spent[agentID] >= limit
}
