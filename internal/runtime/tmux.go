package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TmuxProvider is a runtime provider that executes agents in tmux sessions
type TmuxProvider struct {
	sessions map[string]bool
}

// NewTmuxProvider creates a new tmux-based runtime provider
func NewTmuxProvider() *TmuxProvider {
	return &TmuxProvider{
		sessions: make(map[string]bool),
	}
}

// Start starts an agent in a tmux session
func (p *TmuxProvider) Start(ctx context.Context, agentID, cmd string) error {
	if p.sessions[agentID] {
		return ErrAgentAlreadyRunning
	}

	if cmd == "" {
		return fmt.Errorf("empty command")
	}

	sessionName := "gassy-" + agentID

	// Create a detached tmux session with the command
	// Using -d to detach, -s for session name, -n for window name
	tmuxCmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, cmd)
	if err := tmuxCmd.Run(); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	p.sessions[agentID] = true
	return nil
}

// Stop stops an agent by killing its tmux session
func (p *TmuxProvider) Stop(ctx context.Context, agentID string) error {
	if !p.sessions[agentID] {
		return ErrAgentNotRunning
	}

	sessionName := "gassy-" + agentID
	tmuxCmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	if err := tmuxCmd.Run(); err != nil {
		return fmt.Errorf("killing tmux session: %w", err)
	}

	delete(p.sessions, agentID)
	return nil
}

// Status returns the status of an agent
func (p *TmuxProvider) Status(ctx context.Context, agentID string) (Status, error) {
	if !p.sessions[agentID] {
		return Status{ID: agentID, Alive: false}, ErrAgentNotFound
	}

	sessionName := "gassy-" + agentID

	// Check if the session exists and is running
	checkCmd := exec.Command("tmux", "has-session", "-t", sessionName)
	err := checkCmd.Run()

	alive := err == nil

	return Status{
		ID:    agentID,
		Alive: alive,
	}, nil
}

// SendCommand sends a command to a running tmux session (for interactive use)
func (p *TmuxProvider) SendCommand(ctx context.Context, agentID, cmd string) error {
	if !p.sessions[agentID] {
		return ErrAgentNotRunning
	}

	sessionName := "gassy-" + agentID
	tmuxCmd := exec.Command("tmux", "send-keys", "-t", sessionName, cmd, "Enter")
	if err := tmuxCmd.Run(); err != nil {
		return fmt.Errorf("sending command to tmux session: %w", err)
	}

	return nil
}

// GetSessionName returns the tmux session name for an agent
func (p *TmuxProvider) GetSessionName(agentID string) string {
	return "gassy-" + agentID
}

// ListSessions lists all gassy tmux sessions
func (p *TmuxProvider) ListSessions() []string {
	var sessions []string

	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return sessions
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.HasPrefix(line, "gassy-") {
			sessions = append(sessions, line)
		}
	}

	return sessions
}