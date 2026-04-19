package runtime

import (
	"context"
	"testing"
	"time"
)

func TestNewExecProvider(t *testing.T) {
	p := NewExecProvider()
	if p == nil {
		t.Fatal("NewExecProvider() returned nil")
	}
	if p.processes == nil {
		t.Error("processes map not initialized")
	}
}

func TestExecProviderStartStop(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	// Start a simple agent
	err := p.Start(ctx, "test-agent", "sleep 10")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer p.Stop(ctx, "test-agent")

	// Check status
	status, err := p.Status(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Alive {
		t.Error("status.Alive = false, want true")
	}
	if status.PID <= 0 {
		t.Error("status.PID should be positive")
	}
}

func TestExecProviderDoubleStart(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	err := p.Start(ctx, "test-agent", "sleep 10")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer p.Stop(ctx, "test-agent")

	err = p.Start(ctx, "test-agent", "sleep 10")
	if err != ErrAgentAlreadyRunning {
		t.Errorf("Start() second time error = %v, want ErrAgentAlreadyRunning", err)
	}
}

func TestExecProviderStopNonexistent(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	err := p.Stop(ctx, "nonexistent")
	if err != ErrAgentNotRunning {
		t.Errorf("Stop() error = %v, want ErrAgentNotRunning", err)
	}
}

func TestExecProviderStopWaitsForProcess(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	// Start a short-lived command
	err := p.Start(ctx, "short-lived", "sleep 0.1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Stop should wait for process to finish
	err = p.Stop(ctx, "short-lived")
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify process is removed
	_, err = p.Status(ctx, "short-lived")
	if err != ErrAgentNotFound {
		t.Errorf("Status() after Stop() error = %v, want ErrAgentNotFound", err)
	}
}

func TestExecProviderStopIdempotent(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	// Start a short command
	err := p.Start(ctx, "stop-test", "sleep 1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Stop once
	err = p.Stop(ctx, "stop-test")
	if err != nil {
		t.Fatalf("First Stop() error = %v", err)
	}

	// Stop again should return error (already stopped)
	err = p.Stop(ctx, "stop-test")
	if err != ErrAgentNotRunning {
		t.Errorf("Second Stop() error = %v, want ErrAgentNotRunning", err)
	}
}

func TestExecProviderStartInvalidCommand(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	err := p.Start(ctx, "test-agent", "")
	if err == nil {
		t.Error("Start() with empty command = nil, want error")
	}
}

func TestProviderInterface(t *testing.T) {
	var p Provider = NewExecProvider()
	if p == nil {
		t.Error("Provider interface implementation failed")
	}
}

func TestExecProviderCommandWithArgs(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	// Command with arguments: echo "hello" exits with code 0
	err := p.Start(ctx, "test-echo", "echo hello")
	if err != nil {
		t.Fatalf("Start() with args error = %v", err)
	}
	defer p.Stop(ctx, "test-echo")

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	status, err := p.Status(ctx, "test-echo")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.ID != "test-echo" {
		t.Errorf("status.ID = %q, want %q", status.ID, "test-echo")
	}
}

func TestExecProviderStartInvalidBinary(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	err := p.Start(ctx, "test-invalid", "/nonexistent/binary/path")
	if err == nil {
		t.Error("Start() with invalid binary = nil, want error")
	}
}

func TestExecProviderStopReleasesProcess(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	err := p.Start(ctx, "test-agent", "sleep 5")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	err = p.Stop(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify we can start a new agent with the same ID after stopping
	err = p.Start(ctx, "test-agent", "sleep 5")
	if err != nil {
		t.Errorf("Start() after Stop() error = %v", err)
	}
	p.Stop(ctx, "test-agent")
}

func TestExecProviderStatusNonexistent(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	status, err := p.Status(ctx, "nonexistent")
	if err != ErrAgentNotFound {
		t.Errorf("Status() error = %v, want ErrAgentNotFound", err)
	}
	if status.Alive {
		t.Error("status.Alive = true for nonexistent agent")
	}
}

func TestExecProviderMultipleAgents(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	agents := []string{"agent-1", "agent-2", "agent-3"}
	for _, id := range agents {
		err := p.Start(ctx, id, "sleep 10")
		if err != nil {
			t.Fatalf("Start(%q) error = %v", id, err)
		}
		defer p.Stop(ctx, id)
	}

	if len(p.processes) != 3 {
		t.Errorf("len(processes) = %d, want 3", len(p.processes))
	}

	for _, id := range agents {
		status, err := p.Status(ctx, id)
		if err != nil {
			t.Errorf("Status(%q) error = %v", id, err)
		}
		if !status.Alive {
			t.Errorf("status.Alive = false for %q", id)
		}
	}
}

func TestExecProviderStopAlreadyStopped(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	// Start and immediately stop
	err := p.Start(ctx, "test-agent", "sleep 1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	err = p.Stop(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Stop again should fail
	err = p.Stop(ctx, "test-agent")
	if err != ErrAgentNotRunning {
		t.Errorf("Stop() second time error = %v, want ErrAgentNotRunning", err)
	}
}

func TestExecProviderProcessGroup(t *testing.T) {
	p := NewExecProvider()
	ctx := context.Background()

	// Start a process
	err := p.Start(ctx, "test-agent", "sleep 10")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer p.Stop(ctx, "test-agent")

	status, _ := p.Status(ctx, "test-agent")
	if status.PID <= 0 {
		t.Error("PID should be positive for running process")
	}
}

func TestStatusErrorField(t *testing.T) {
	status := Status{
		ID:    "test-agent",
		Alive: false,
		PID:   0,
		Error: "process exited",
	}
	if status.Error != "process exited" {
		t.Errorf("Error = %q, want %q", status.Error, "process exited")
	}
	if status.Alive {
		t.Error("Alive = true for dead process")
	}
}

func TestErrorConstants(t *testing.T) {
	if ErrAgentNotFound == nil {
		t.Error("ErrAgentNotFound is nil")
	}
	if ErrAgentAlreadyRunning == nil {
		t.Error("ErrAgentAlreadyRunning is nil")
	}
	if ErrAgentNotRunning == nil {
		t.Error("ErrAgentNotRunning is nil")
	}

	// Verify errors are distinct
	if ErrAgentNotFound == ErrAgentAlreadyRunning {
		t.Error("ErrAgentNotFound and ErrAgentAlreadyRunning are the same")
	}
	if ErrAgentNotFound == ErrAgentNotRunning {
		t.Error("ErrAgentNotFound and ErrAgentNotRunning are the same")
	}
	if ErrAgentAlreadyRunning == ErrAgentNotRunning {
		t.Error("ErrAgentAlreadyRunning and ErrAgentNotRunning are the same")
	}
}
