package a2a

import (
	"context"
	"sync"
	"time"
)

// AgentRegistry manages discovered agents and their Agent Cards
type AgentRegistry struct {
	mu       sync.RWMutex
	agents   map[string]*AgentCard
	lastSeen map[string]time.Time
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents:   make(map[string]*AgentCard),
		lastSeen: make(map[string]time.Time),
	}
}

// Register adds or updates an agent card in the registry
func (r *AgentRegistry) Register(card *AgentCard) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[card.Name] = card
	r.lastSeen[card.Name] = time.Now()
}

// Unregister removes an agent from the registry
func (r *AgentRegistry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.agents, agentID)
	delete(r.lastSeen, agentID)
}

// Get returns an agent card by ID
func (r *AgentRegistry) Get(agentID string) (*AgentCard, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	card, ok := r.agents[agentID]
	return card, ok
}

// GetBySkill returns all agents that have the specified skill
func (r *AgentRegistry) GetBySkill(skill string) []*AgentCard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*AgentCard
	for _, card := range r.agents {
		for _, s := range card.Skills {
			if s.ID == skill {
				result = append(result, card)
				break
			}
		}
	}
	return result
}

// List returns all registered agent cards
func (r *AgentRegistry) List() []*AgentCard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*AgentCard, 0, len(r.agents))
	for _, card := range r.agents {
		result = append(result, card)
	}
	return result
}

// Count returns the number of registered agents
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.agents)
}

// LastSeen returns the last time an agent was seen
func (r *AgentRegistry) LastSeen(agentID string) (time.Time, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.lastSeen[agentID]
	return t, ok
}

// DiscoveryPoller periodically discovers agents from known URLs
type DiscoveryPoller struct {
	registry *AgentRegistry
	urls     []string
	interval time.Duration
	stopCh   chan struct{}
}

// NewDiscoveryPoller creates a new discovery poller
func NewDiscoveryPoller(registry *AgentRegistry, urls []string, interval time.Duration) *DiscoveryPoller {
	return &DiscoveryPoller{
		registry: registry,
		urls:     urls,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins periodic discovery
func (p *DiscoveryPoller) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Do initial discovery
	p.discover(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.discover(ctx)
		}
	}
}

// Stop stops the discovery poller
func (p *DiscoveryPoller) Stop() {
	close(p.stopCh)
}

// discover performs a single discovery pass
func (p *DiscoveryPoller) discover(ctx context.Context) {
	for _, url := range p.urls {
		card, err := FetchAgentCard(ctx, url)
		if err != nil {
			continue
		}
		p.registry.Register(card)
	}
}
