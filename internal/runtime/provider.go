package runtime

import (
	"context"
	"fmt"
)

// Provider represents a runtime provider for executing agents
type Provider interface {
	// Start starts an agent process
	Start(ctx context.Context, agentID, cmd string) error
	// Stop stops an agent process
	Stop(ctx context.Context, agentID string) error
	// Status returns the status of an agent
	Status(ctx context.Context, agentID string) (Status, error)
}

// Status represents the status of an agent
type Status struct {
	ID    string
	Alive bool
	PID   int
	Error string
}

// ErrAgentNotFound is returned when an agent is not found
var ErrAgentNotFound = fmt.Errorf("agent not found")

// ErrAgentAlreadyRunning is returned when an agent is already running
var ErrAgentAlreadyRunning = fmt.Errorf("agent already running")

// ErrAgentNotRunning is returned when an agent is not running
var ErrAgentNotRunning = fmt.Errorf("agent not running")
