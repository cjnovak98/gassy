package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// ExecProvider is a runtime provider that executes agents as subprocesses
type ExecProvider struct {
	processes map[string]*exec.Cmd
}

// NewExecProvider creates a new exec-based runtime provider
func NewExecProvider() *ExecProvider {
	return &ExecProvider{
		processes: make(map[string]*exec.Cmd),
	}
}

// Start starts an agent as a subprocess
func (p *ExecProvider) Start(ctx context.Context, agentID, cmd string) error {
	if _, exists := p.processes[agentID]; exists {
		return ErrAgentAlreadyRunning
	}

	args := strings.Fields(cmd)
	if len(args) == 0 {
		return fmt.Errorf("empty command")
	}

	execCmd := exec.CommandContext(ctx, args[0], args[1:]...)
	execCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := execCmd.Start(); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	p.processes[agentID] = execCmd
	return nil
}

// Stop stops an agent subprocess
func (p *ExecProvider) Stop(ctx context.Context, agentID string) error {
	cmd, exists := p.processes[agentID]
	if !exists {
		return ErrAgentNotRunning
	}

	if err := cmd.Process.Kill(); err != nil {
		// Process may already be dead, try to wait anyway
		cmd.Wait()
		return fmt.Errorf("killing process: %w", err)
	}

	// Wait for process to fully exit
	cmd.Wait()

	delete(p.processes, agentID)
	return nil
}

// Status returns the status of an agent
func (p *ExecProvider) Status(ctx context.Context, agentID string) (Status, error) {
	cmd, exists := p.processes[agentID]
	if !exists {
		return Status{ID: agentID, Alive: false}, ErrAgentNotFound
	}

	// Check if process is still running
	processState := cmd.ProcessState
	alive := processState == nil

	return Status{
		ID:    agentID,
		Alive: alive,
		PID:   cmd.Process.Pid,
	}, nil
}
