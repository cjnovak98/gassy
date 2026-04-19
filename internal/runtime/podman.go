package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PodmanManager manages agents as containers via podman
type PodmanManager struct{}

// NewPodmanManager creates a new podman-based container manager
func NewPodmanManager() *PodmanManager {
	return &PodmanManager{}
}

// AgentConfig for podman
type AgentConfig struct {
	ID    string
	Image string
	Port  int
	Env   []string
}

// ContainerStatus represents agent/container status
type ContainerStatus struct {
	Alive bool
	PID   int
	Error string
}

// ContainerInfo represents information about a running container
type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	Status string
	Ports  string
}

// Start starts an agent as a podman container
func (p *PodmanManager) Start(ctx context.Context, agentID, image string, port int, env []string) error {
	containerName := "gassy-" + agentID

	// Build env flags
	var envArgs []string
	for _, e := range env {
		envArgs = append(envArgs, "-e", e)
	}

	// podman run -d --name gassy-<agentID> --network=host -p <port>:<port> -e AGENT_ROLE=<agentID> -e PORT=<port> <image>
	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network=host",
		"-p", fmt.Sprintf("%d:%d", port, port),
		"-e", fmt.Sprintf("AGENT_ROLE=%s", agentID),
		"-e", fmt.Sprintf("PORT=%d", port),
	}
	args = append(args, envArgs...)
	args = append(args, image)

	cmd := exec.CommandContext(ctx, "podman", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	return nil
}

// Stop stops and removes an agent container
func (p *PodmanManager) Stop(ctx context.Context, agentID string) error {
	containerName := "gassy-" + agentID

	cmd := exec.CommandContext(ctx, "podman", "rm", "-f", containerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}

	return nil
}

// Status returns the status of an agent container
func (p *PodmanManager) Status(ctx context.Context, agentID string) (ContainerStatus, error) {
	containerName := "gassy-" + agentID

	// podman ps --filter name=gassy-<agentID> -q
	cmd := exec.CommandContext(ctx, "podman", "ps", "--filter", "name="+containerName, "-q")
	output, err := cmd.Output()
	if err != nil {
		// Exit error means no container found
		return ContainerStatus{Alive: false}, nil
	}

	alive := strings.TrimSpace(string(output)) != ""

	return ContainerStatus{Alive: alive}, nil
}

// Logs returns the logs of an agent container
func (p *PodmanManager) Logs(ctx context.Context, agentID string) (string, error) {
	containerName := "gassy-" + agentID

	cmd := exec.CommandContext(ctx, "podman", "logs", containerName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("fetching logs: %w", err)
	}

	return string(output), nil
}

// Ps returns all gassy containers
func (p *PodmanManager) Ps() ([]ContainerInfo, error) {
	// podman ps --filter label=gassy=true --format json
	cmd := exec.Command("podman", "ps", "--filter", "label=gassy=true", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	if strings.TrimSpace(string(output)) == "" {
		return []ContainerInfo{}, nil
	}

	var containers []struct {
		ID    string `json:"Id"`
		Names []string `json:"Names"`
		Image string `json:"Image"`
		Status string `json:"Status"`
		Ports string `json:"Ports"`
	}

	if err := json.Unmarshal(output, &containers); err != nil {
		return nil, fmt.Errorf("parsing container info: %w", err)
	}

	var result []ContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		result = append(result, ContainerInfo{
			ID:     c.ID,
			Name:   name,
			Image:  c.Image,
			Status: c.Status,
			Ports:  c.Ports,
		})
	}

	return result, nil
}