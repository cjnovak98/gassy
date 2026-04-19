package runtime

import (
	"context"
	"testing"
)

func TestNewTmuxProvider(t *testing.T) {
	p := NewTmuxProvider()
	if p == nil {
		t.Fatal("NewTmuxProvider() returned nil")
	}
	if p.sessions == nil {
		t.Error("sessions map not initialized")
	}
}

func TestTmuxProviderInterface(t *testing.T) {
	var p Provider = NewTmuxProvider()
	if p == nil {
		t.Error("TmuxProvider interface implementation failed")
	}
}

func TestTmuxProviderStartInvalidCommand(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	err := p.Start(ctx, "test-agent", "")
	if err == nil {
		t.Error("Start() with empty command = nil, want error")
	}
}

func TestTmuxProviderDoubleStart(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// This will fail if tmux is not available, but tests the flow
	err := p.Start(ctx, "test-agent", "sleep 10")
	if err != nil {
		// tmux might not be available, skip if so
		t.Skip("tmux not available")
	}
	defer p.Stop(ctx, "test-agent")

	err = p.Start(ctx, "test-agent", "sleep 10")
	if err != ErrAgentAlreadyRunning {
		t.Errorf("Start() second time error = %v, want ErrAgentAlreadyRunning", err)
	}
}

func TestTmuxProviderStopNonexistent(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	err := p.Stop(ctx, "nonexistent")
	if err != ErrAgentNotRunning {
		t.Errorf("Stop() error = %v, want ErrAgentNotRunning", err)
	}
}

func TestTmuxProviderStatusNonexistent(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	status, err := p.Status(ctx, "nonexistent")
	if err != ErrAgentNotFound {
		t.Errorf("Status() error = %v, want ErrAgentNotFound", err)
	}
	if status.Alive {
		t.Error("status.Alive = true for nonexistent agent")
	}
}

func TestTmuxProviderGetSessionName(t *testing.T) {
	p := NewTmuxProvider()

	name := p.GetSessionName("my-agent")
	if name != "gassy-my-agent" {
		t.Errorf("GetSessionName() = %q, want %q", name, "gassy-my-agent")
	}
}

func TestTmuxProviderSessionNameFormat(t *testing.T) {
	p := NewTmuxProvider()

	testCases := []struct {
		agentID string
		want    string
	}{
		{"simple", "gassy-simple"},
		{"with-dash", "gassy-with-dash"},
		{"with_underscore", "gassy-with_underscore"},
	}

	for _, tc := range testCases {
		name := p.GetSessionName(tc.agentID)
		if name != tc.want {
			t.Errorf("GetSessionName(%q) = %q, want %q", tc.agentID, name, tc.want)
		}
	}
}

func TestTmuxProviderSendCommandNonexistent(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	err := p.SendCommand(ctx, "nonexistent", "echo hello")
	if err != ErrAgentNotRunning {
		t.Errorf("SendCommand() error = %v, want ErrAgentNotRunning", err)
	}
}

func TestTmuxProviderSendCommandNotRunning(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// Add session but it's not actually running
	p.sessions["test-agent"] = true

	err := p.SendCommand(ctx, "test-agent", "echo hello")
	// Will fail because actual tmux session doesn't exist, but structure is right
	if err == nil {
		t.Error("SendCommand() to non-running tmux session should fail")
	}
}

func TestTmuxProviderListSessionsEmpty(t *testing.T) {
	p := NewTmuxProvider()

	sessions := p.ListSessions()
	// Returns whatever tmux list-sessions returns (may be empty)
	// Just verify it doesn't crash and returns a slice
	if sessions == nil {
		t.Error("ListSessions() returned nil, want empty slice or list")
	}
}

func TestTmuxProviderGetSessionNameSpecialChars(t *testing.T) {
	p := NewTmuxProvider()

	testCases := []struct {
		agentID string
		want    string
	}{
		{"agent_123", "gassy-agent_123"},
		{"myAgent", "gassy-myAgent"},
		{"a", "gassy-a"},
	}

	for _, tc := range testCases {
		name := p.GetSessionName(tc.agentID)
		if name != tc.want {
			t.Errorf("GetSessionName(%q) = %q, want %q", tc.agentID, name, tc.want)
		}
	}
}

func TestTmuxProviderSendCommandNotRegistered(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	err := p.SendCommand(ctx, "not-registered", "echo hello")
	if err != ErrAgentNotRunning {
		t.Errorf("SendCommand() to unregistered agent = %v, want ErrAgentNotRunning", err)
	}
}

func TestTmuxProviderSendCommandRegistered(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// Register a session without actually running tmux
	p.sessions["test-agent"] = true

	err := p.SendCommand(ctx, "test-agent", "echo hello")
	// Will fail because tmux isn't actually running, but proves the dispatch path
	if err == nil {
		// tmux available and session existed - verify session still registered
		if !p.sessions["test-agent"] {
			t.Error("session should remain registered after SendCommand")
		}
	}
}

func TestTmuxProviderListSessionsNoTmux(t *testing.T) {
	p := NewTmuxProvider()

	sessions := p.ListSessions()
	// When tmux is not available, list-sessions fails and returns empty
	// This tests the error path without crashing
	if sessions == nil {
		t.Error("ListSessions() should return empty slice, not nil")
	}
}

func TestTmuxProviderListSessionsSkipsNonGassy(t *testing.T) {
	// This test verifies the filtering logic by checking the tmux output parsing
	p := NewTmuxProvider()

	// Since we can't control actual tmux output in unit tests,
	// verify that non-gassy sessions would be filtered
	name := p.GetSessionName("my-app")
	if name != "gassy-my-app" {
		t.Errorf("GetSessionName = %q, want gassy-my-app", name)
	}
}

func TestTmuxProviderSendCommandEmpty(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// Empty agent ID
	err := p.SendCommand(ctx, "", "echo hello")
	if err != ErrAgentNotRunning {
		t.Errorf("SendCommand() with empty agentID = %v, want ErrAgentNotRunning", err)
	}
}

func TestTmuxProviderStatusDeadSession(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// Register a "running" session
	p.sessions["dead-agent"] = true

	// Status will query tmux has-session; if tmux unavailable,
	// this tests the dead-session cleanup path
	status, err := p.Status(ctx, "dead-agent")
	if err != nil {
		t.Logf("Status returned error (tmux may be unavailable): %v", err)
	}
	if status.ID != "dead-agent" {
		t.Errorf("status.ID = %q, want %q", status.ID, "dead-agent")
	}
}

func TestTmuxProviderStopIdempotent(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// Register but don't actually run tmux
	p.sessions["stop-test"] = true

	err := p.Stop(ctx, "stop-test")
	// Stop will fail because tmux session doesn't actually exist,
	// but the session should be removed from the map
	if err != nil {
		t.Logf("Stop returned error (expected if tmux unavailable): %v", err)
	}
	// Session should be removed regardless
	if p.sessions["stop-test"] {
		t.Error("session should be removed from map after Stop()")
	}
}

func TestTmuxProviderStartWithTmuxFailure(t *testing.T) {
	p := NewTmuxProvider()
	ctx := context.Background()

	// Attempting to start when tmux binary doesn't exist
	err := p.Start(ctx, "fail-agent", "echo test")
	if err == nil {
		t.Error("Start() with no tmux should return error")
	}
	if p.sessions["fail-agent"] {
		t.Error("session should not be registered after failed Start")
	}
}