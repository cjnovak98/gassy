package a2a

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewAgentRegistry(t *testing.T) {
	r := NewAgentRegistry()
	if r == nil {
		t.Fatal("NewAgentRegistry returned nil")
	}
	if r.agents == nil {
		t.Error("agents map is nil")
	}
	if r.lastSeen == nil {
		t.Error("lastSeen map is nil")
	}
}

func TestAgentRegistryRegister(t *testing.T) {
	r := NewAgentRegistry()

	card := &AgentCard{
		Name:    "engineer",
		Version: "1.0",
		Url:     "http://localhost:8002",
		Capabilities: AgentCapabilities{
			Streaming: true,
		},
		Skills: []AgentSkill{
			{ID: "code", Name: "Write Code"},
		},
	}

	r.Register(card)

	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1", r.Count())
	}

	retrieved, ok := r.Get("engineer")
	if !ok {
		t.Error("Get(engineer) returned false, want true")
	}
	if retrieved.Name != "engineer" {
		t.Errorf("retrieved.Name = %q, want %q", retrieved.Name, "engineer")
	}
}

func TestAgentRegistryUnregister(t *testing.T) {
	r := NewAgentRegistry()

	card := &AgentCard{Name: "engineer"}
	r.Register(card)

	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1", r.Count())
	}

	r.Unregister("engineer")

	if r.Count() != 0 {
		t.Errorf("Count() = %d, want 0", r.Count())
	}

	_, ok := r.Get("engineer")
	if ok {
		t.Error("Get(engineer) returned true after Unregister, want false")
	}
}

func TestAgentRegistryGetBySkill(t *testing.T) {
	r := NewAgentRegistry()

	r.Register(&AgentCard{
		Name:    "engineer",
		Skills:  []AgentSkill{{ID: "code"}, {ID: "test"}},
	})
	r.Register(&AgentCard{
		Name:    "designer",
		Skills:  []AgentSkill{{ID: "design"}},
	})
	r.Register(&AgentCard{
		Name:    "mayor",
		Skills:  []AgentSkill{},
	})

	codeAgents := r.GetBySkill("code")
	if len(codeAgents) != 1 {
		t.Errorf("len(codeAgents) = %d, want 1", len(codeAgents))
	}
	if codeAgents[0].Name != "engineer" {
		t.Errorf("codeAgents[0].Name = %q, want %q", codeAgents[0].Name, "engineer")
	}

	designAgents := r.GetBySkill("design")
	if len(designAgents) != 1 {
		t.Errorf("len(designAgents) = %d, want 1", len(designAgents))
	}
	if designAgents[0].Name != "designer" {
		t.Errorf("designAgents[0].Name = %q, want %q", designAgents[0].Name, "designer")
	}

	unknownAgents := r.GetBySkill("unknown")
	if len(unknownAgents) != 0 {
		t.Errorf("len(unknownAgents) = %d, want 0", len(unknownAgents))
	}
}

func TestAgentRegistryList(t *testing.T) {
	r := NewAgentRegistry()

	r.Register(&AgentCard{Name: "engineer"})
	r.Register(&AgentCard{Name: "designer"})

	list := r.List()
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}

func TestAgentRegistryLastSeen(t *testing.T) {
	r := NewAgentRegistry()

	r.Register(&AgentCard{Name: "engineer"})

	ts, ok := r.LastSeen("engineer")
	if !ok {
		t.Error("LastSeen(engineer) returned false, want true")
	}
	if ts.IsZero() {
		t.Error("LastSeen(engineer) returned zero time")
	}

	_, ok = r.LastSeen("unknown")
	if ok {
		t.Error("LastSeen(unknown) returned true, want false")
	}
}

func TestDiscoveryPollerStructure(t *testing.T) {
	r := NewAgentRegistry()
	urls := []string{"http://localhost:8001", "http://localhost:8002"}
	interval := 30 * time.Second

	p := NewDiscoveryPoller(r, urls, interval)
	if p == nil {
		t.Fatal("NewDiscoveryPoller returned nil")
	}
	if p.registry != r {
		t.Error("poller.registry != r")
	}
	if len(p.urls) != 2 {
		t.Errorf("len(p.urls) = %d, want 2", len(p.urls))
	}
	if p.interval != interval {
		t.Errorf("p.interval = %v, want %v", p.interval, interval)
	}
}

func TestDiscoveryPollerStop(t *testing.T) {
	r := NewAgentRegistry()
	p := NewDiscoveryPoller(r, nil, time.Second)

	// Stop should not panic
	p.Stop()
}

func TestAgentRegistryMultipleRegister(t *testing.T) {
	r := NewAgentRegistry()

	r.Register(&AgentCard{Name: "engineer", Url: "http://localhost:8001"})
	r.Register(&AgentCard{Name: "engineer", Url: "http://localhost:8002"})

	card, ok := r.Get("engineer")
	if !ok {
		t.Fatal("Get(engineer) returned false")
	}
	if card.Url != "http://localhost:8002" {
		t.Errorf("card.Url = %q, want %q", card.Url, "http://localhost:8002")
	}

	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1", r.Count())
	}
}

func TestAgentRegistryConcurrentAccess(t *testing.T) {
	registry := NewAgentRegistry()
	card := &AgentCard{
		Name:    "concurrent-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilities{Streaming: true},
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.Register(card)
			registry.Get("concurrent-agent")
			registry.List()
			registry.Count()
		}()
	}
	wg.Wait()
}

func TestDiscoveryPollerWithInvalidURL(t *testing.T) {
	registry := NewAgentRegistry()
	// Use a URL that will fail
	poller := NewDiscoveryPoller(registry, []string{"http://localhost:99999"}, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	poller.Start(ctx)
	// Should not panic - just skip the invalid URL
	poller.Stop()

	// Registry should remain empty
	if registry.Count() != 0 {
		t.Errorf("Count() = %d after failed discovery, want 0", registry.Count())
	}
}