package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Agent represents a registered agent in the supervisor's registry
type Agent struct {
	Name         string
	Role         string // Agent role (e.g., "engineer", "tester")
	Binary       string
	Port         int
	Skills       []string
	ContainerID  string // podman container ID/name
	Status       AgentStatus
	URL          string
	CardURL      string // URL to this agent's agent card (e.g., http://localhost:8082/.well-known/agent.json)
	A2AURL       string // A2A endpoint URL for agent communication
}

// AgentStatus represents the current status of an agent
type AgentStatus string

const (
	StatusAlive   AgentStatus = "alive"
	StatusDead    AgentStatus = "dead"
	StatusUnknown AgentStatus = "unknown"
)

// Supervisor manages the agent registry and reconciliation
type Supervisor struct {
	mu         sync.RWMutex
	agents     map[string]Agent
	socketPath string
	stopCh     chan struct{}
	wg         sync.WaitGroup
	stateFile  string
}

// NewSupervisor creates a new supervisor instance
func NewSupervisor(socketPath string) *Supervisor {
	s := &Supervisor{
		agents:     make(map[string]Agent),
		socketPath: socketPath,
		stopCh:     make(chan struct{}),
		stateFile:  "/tmp/gassy-supervisor-registry.json",
	}
	s.loadState()
	return s
}

// Start begins the supervisor's background activities
func (s *Supervisor) Start(ctx context.Context) error {
	// Remove existing socket file if present
	os.Remove(s.socketPath)

	// Start the reconcile loop
	s.wg.Add(1)
	go s.reconcileLoop(ctx)

	// Start the socket listener for CLI commands
	s.wg.Add(1)
	go s.serveSocket(ctx)

	// Start the HTTP listener for mayor and agent CLI
	s.wg.Add(1)
	go s.serveHTTP(ctx)

	return nil
}

// Stop gracefully stops the supervisor
func (s *Supervisor) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// reconcileLoop runs health checks every 30 seconds
func (s *Supervisor) reconcileLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	s.healthCheckAndRestart(ctx)

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.healthCheckAndRestart(ctx)
		}
	}
}

// healthCheckAndRestart checks health and restarts dead agents
func (s *Supervisor) healthCheckAndRestart(ctx context.Context) {
	s.mu.RLock()
	agents := make(map[string]Agent, len(s.agents))
	for name, agent := range s.agents {
		agents[name] = agent
	}
	s.mu.RUnlock()

	for name, agent := range agents {
		alive := s.pingAgent(ctx, agent.URL)
		s.mu.Lock()
		if a, ok := s.agents[name]; ok {
			if alive {
				a.Status = StatusAlive
				s.agents[name] = a
			} else {
				// Agent is dead - try to restart
				a.Status = StatusDead
				s.agents[name] = a
				s.mu.Unlock()

				// Restart if binary/container is available
				if a.Binary != "" {
					log.Printf("agent %s is dead, attempting restart", name)
					newContainerID, err := s.spawnAgentProcess(a.Name, a.Role, a.Port)
					if err != nil {
						log.Printf("failed to restart %s: %v", name, err)
					} else {
						s.mu.Lock()
						if existing, ok := s.agents[name]; ok {
							existing.ContainerID = newContainerID
							existing.Status = StatusAlive
							s.agents[name] = existing
						}
						s.mu.Unlock()
					}
				}
				continue
			}
		}
		s.mu.Unlock()
	}
}

// pingAgent attempts to contact an agent via HTTP GET /health
func (s *Supervisor) pingAgent(ctx context.Context, url string) bool {
	// Try the /health endpoint
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// serveSocket handles incoming CLI commands over Unix socket
func (s *Supervisor) serveSocket(ctx context.Context) {
	defer s.wg.Done()

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		log.Printf("socket listen error: %v", err)
		return
	}
	defer ln.Close()

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		go s.handleConnection(conn)
	}
}

// serveHTTP runs an HTTP API for the mayor and agent CLI
func (s *Supervisor) serveHTTP(ctx context.Context) {
	defer s.wg.Done()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			s.mu.RLock()
			agents := make([]Agent, 0, len(s.agents))
			for _, a := range s.agents {
				agents = append(agents, a)
			}
			s.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"agents": agents})
			return
		}
		if r.Method == http.MethodPost {
			var req struct {
				Name    string `json:"name"`
				CardURL string `json:"cardURL"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			baseURL := req.CardURL
			if idx := strings.Index(baseURL, "/.well-known"); idx > 0 {
				baseURL = baseURL[:idx]
			}
			// If agent already exists, update its fields for restart case
			if existing, exists := s.agents[req.Name]; exists {
				existing.URL = baseURL
				existing.CardURL = req.CardURL
				existing.A2AURL = baseURL
				existing.Role = req.Name
				existing.Status = StatusAlive
				s.agents[req.Name] = existing
				s.saveStateLocked()
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"success": "agent reregistered"})
				return
			}
			agent := Agent{
				Name:    req.Name,
				Role:    req.Name, // Use name as role (mayor/engineer serve as both ID and role)
				CardURL: req.CardURL,
				URL:     baseURL,
				A2AURL:  baseURL,
				Status:  StatusAlive,
			}
			s.agents[req.Name] = agent
			s.saveStateLocked()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"success": "agent registered"})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// Registry API handlers
	mux.HandleFunc("/registry/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			AgentID string   `json:"agent_id"`
			Role    string   `json:"role"`
			Skills  []string `json:"skills"`
			A2AURL  string   `json:"a2a_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		baseURL := req.A2AURL
		cardURL := baseURL + "/.well-known/agent.json"
		existing, exists := s.agents[req.AgentID]
		if exists {
			existing.Role = req.Role
			existing.Skills = req.Skills
			existing.A2AURL = req.A2AURL
			existing.URL = baseURL
			existing.CardURL = cardURL
			existing.Status = StatusAlive
			s.agents[req.AgentID] = existing
			s.saveStateLocked()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"success": "agent reregistered"})
			return
		}
		agent := Agent{
			Name:    req.AgentID,
			Role:    req.Role,
			Skills:  req.Skills,
			A2AURL:  req.A2AURL,
			URL:     baseURL,
			CardURL: cardURL,
			Status:  StatusAlive,
		}
		s.agents[req.AgentID] = agent
		s.saveStateLocked()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"success": "agent registered"})
	})

	mux.HandleFunc("/registry/discover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		skill := r.URL.Query().Get("skill")
		if skill == "" {
			http.Error(w, "skill parameter required", http.StatusBadRequest)
			return
		}
		agents := s.DiscoverAgents(skill)
		var result []struct {
			AgentID string `json:"agent_id"`
			A2AURL  string `json:"a2a_url"`
		}
		for _, a := range agents {
			result = append(result, struct {
				AgentID string `json:"agent_id"`
				A2AURL  string `json:"a2a_url"`
			}{AgentID: a.Name, A2AURL: a.A2AURL})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("/registry/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mu.RLock()
		var result []struct {
			AgentID string `json:"agent_id"`
			Role    string `json:"role"`
			A2AURL  string `json:"a2a_url"`
		}
		for _, a := range s.agents {
			result = append(result, struct {
				AgentID string `json:"agent_id"`
				Role    string `json:"role"`
				A2AURL  string `json:"a2a_url"`
			}{AgentID: a.Name, Role: a.Role, A2AURL: a.A2AURL})
		}
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("/registry/unregister/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Extract agent_id from path /registry/unregister/{agent_id}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/registry/unregister/"), "/")
		agentID := parts[0]
		if agentID == "" {
			http.Error(w, "agent_id required", http.StatusBadRequest)
			return
		}
		if s.UnregisterAgent(agentID) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"success": "agent unregistered"})
		} else {
			http.Error(w, "agent not found", http.StatusNotFound)
		}
	})

	mux.HandleFunc("/supervisor/hire", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name   string   `json:"name"`
			Role   string   `json:"role"`
			Skills []string `json:"skills"`
			Port   int      `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		result := s.hireAgent(req.Name, req.Role, req.Port, req.Skills)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{result})
	})

	ln, err := net.Listen("tcp", ":9091")
	if err != nil {
		log.Printf("http listen error: %v", err)
		return
	}
	defer ln.Close()

	log.Printf("supervisor HTTP API listening on :9091")
	if err := http.Serve(ln, mux); err != nil {
		log.Printf("http serve error: %v", err)
	}
}

// handleConnection processes a single socket connection
func (s *Supervisor) handleConnection(conn net.Conn) {
	defer conn.Close()

	var cmd struct {
		Action  string   `json:"action"`
		Name    string   `json:"name,omitempty"`
		Role    string   `json:"role,omitempty"`
		Port    int      `json:"port,omitempty"`
		Skills  []string `json:"skills,omitempty"`
		CardURL string   `json:"cardURL,omitempty"`
	}

	dec := json.NewDecoder(conn)
	if err := dec.Decode(&cmd); err != nil {
		json.NewEncoder(conn).Encode(map[string]string{"error": err.Error()})
		return
	}

	var resp interface{}
	switch cmd.Action {
	case "list":
		resp = s.listAgents()
	case "hire":
		resp = s.hireAgent(cmd.Name, cmd.Role, cmd.Port, cmd.Skills)
	case "fire":
		resp = s.fireAgent(cmd.Name)
	case "register":
		resp = s.registerAgent(cmd.Name, cmd.CardURL)
	default:
		resp = map[string]string{"error": "unknown action"}
	}

	json.NewEncoder(conn).Encode(resp)
}

// listAgents returns all registered agents
func (s *Supervisor) listAgents() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	return map[string]interface{}{
		"agents": agents,
	}
}

// loadState loads agent registry from disk if it exists
func (s *Supervisor) loadState() {
	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("warning: failed to load state: %v", err)
		}
		return
	}

	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		log.Printf("warning: failed to unmarshal state: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range agents {
		s.agents[a.Name] = a
	}
	log.Printf("loaded %d agents from state file", len(agents))
}

// saveState persists the agent registry to disk
func (s *Supervisor) saveState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveStateLocked()
}

// saveStateLocked saves state assuming the caller holds the lock
func (s *Supervisor) saveStateLocked() {
	agents := make([]Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}

	data, err := json.Marshal(agents)
	if err != nil {
		log.Printf("error: failed to marshal state: %v", err)
		return
	}

	if err := os.WriteFile(s.stateFile, data, 0644); err != nil {
		log.Printf("error: failed to write state: %v", err)
	}
}

// hireAgent adds a new agent to the registry and starts it
func (s *Supervisor) hireAgent(name, role string, port int, skills []string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.agents[name]; exists {
		return map[string]string{"error": "agent already exists"}
	}

	// Allocate port dynamically if not specified
	if port == 0 {
		port = findAvailablePort()
		if port == 0 {
			s.mu.Unlock()
			return map[string]string{"error": "no available ports found"}
		}
	}

	// Spawn the agent container
	containerID, err := s.spawnAgentProcess(name, role, port)
	if err != nil {
		return map[string]string{"error": fmt.Sprintf("failed to start agent: %v", err)}
	}

	agent := Agent{
		Name:        name,
		Role:        role,
		Binary:      name, // binary name is derived from agent name
		Port:        port,
		Skills:      skills,
		ContainerID: containerID,
		Status:      StatusAlive,
		URL:         fmt.Sprintf("http://localhost:%d", port),
		CardURL:     fmt.Sprintf("http://localhost:%d/.well-known/agent.json", port),
		A2AURL:      fmt.Sprintf("http://localhost:%d", port),
	}
	s.agents[name] = agent
	s.saveStateLocked()

	return map[string]string{"success": "agent hired"}
}

// getPodmanSocket returns the podman socket path to use
// Priority: PODMAN_SOCKET env > /run/user/<uid>/podman/podman.sock > /var/run/podman/podman.sock
func getPodmanSocket() string {
	// Check explicit env first
	if socket := os.Getenv("PODMAN_SOCKET"); socket != "" {
		return socket
	}

	// Check common rootless podman socket locations
	uid := os.Getuid()
	if uid != 0 {
		// Try rootless socket path first
		socketPath := fmt.Sprintf("/run/user/%d/podman/podman.sock", uid)
		if _, err := os.Stat(socketPath); err == nil {
			return socketPath
		}
	}

	// Fallback to rootful socket
	if _, err := os.Stat("/var/run/podman/podman.sock"); err == nil {
		return "/var/run/podman/podman.sock"
	}

	// Let podman auto-detect
	return ""
}

// podmanCmd returns an exec.Cmd configured with the correct socket path
func podmanCmd(args ...string) *exec.Cmd {
	socket := getPodmanSocket()
	if socket != "" {
		// Use CONTAINER_HOST env var instead of --url flag to avoid storage path issues
		env := append(os.Environ(), "CONTAINER_HOST=unix://"+socket)
		cmd := exec.Command("podman", args...)
		cmd.Env = env
		return cmd
	}
	return exec.Command("podman", args...)
}

// spawnAgentProcess starts an agent container and waits for it to be ready
func (s *Supervisor) spawnAgentProcess(name, role string, port int) (string, error) {
	// Get supervisor host (use SUPERVISOR_HOST env or detect hostname)
	supervisorHost := os.Getenv("SUPERVISOR_HOST")
	if supervisorHost == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return "", fmt.Errorf("getting hostname: %w", err)
		}
		supervisorHost = hostname
	}

	// Build podman run command
	containerName := "gassy-" + name
	cmd := podmanCmd("run", "-d",
		"--name", containerName,
		"--network=host",
		"--env", "AGENT_ROLE="+role,
		"--env", "SUPERVISOR_URL=http://127.0.0.1:9091",
		"--env", "PORT="+strconv.Itoa(port),
	)

	// Forward ANTHROPIC_* and MINIMAX_* env vars
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "ANTHROPIC_") || strings.HasPrefix(e, "MINIMAX_") {
			cmd.Args = append(cmd.Args, "--env", e)
		}
	}

	cmd.Args = append(cmd.Args, "localhost:5000/gassy/agent:latest")
	log.Printf("DEBUG: podman args: %v", cmd.Args)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("starting container: %w", err)
	}

	// Agent container started - let it start asynchronously
	// The reconcile loop will handle restarting if needed
	return containerName, nil
}

// waitForPort waits for a TCP port to become reachable
func (s *Supervisor) waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %d", port)
}

// fireAgent removes an agent from the registry and kills its container
func (s *Supervisor) fireAgent(name string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.agents[name]

	// Always try to kill the container by name, even if not in registry
	containerName := "gassy-" + name
	if err := podmanCmd("kill", containerName).Run(); err != nil {
		log.Printf("warning: failed to kill container %s: %v", containerName, err)
	}
	podmanCmd("rm", containerName).Run() // cleanup

	if exists {
		delete(s.agents, name)
		s.saveStateLocked()
	}
	return map[string]string{"success": "agent fired"}
}

// registerAgent registers an agent that is already running and exposing its own card
func (s *Supervisor) registerAgent(name, cardURL string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// cardURL should be like http://localhost:8082/.well-known/agent.json
	// Extract the base URL from it
	baseURL := cardURL
	if idx := strings.Index(baseURL, "/.well-known"); idx > 0 {
		baseURL = baseURL[:idx]
	}

	// If agent already exists, update its fields for restart case
	if existing, exists := s.agents[name]; exists {
		existing.URL = baseURL
		existing.CardURL = cardURL
		existing.Status = StatusAlive
		// ContainerID may be different after restart - update it
		// Binary and Skills are preserved from the original registration
		s.agents[name] = existing
		s.saveStateLocked()
		return map[string]string{"success": "agent reregistered"}
	}

	agent := Agent{
		Name:    name,
		CardURL: cardURL,
		URL:     baseURL,
		A2AURL:  baseURL,
		Status:  StatusAlive,
	}
	s.agents[name] = agent
	s.saveStateLocked()

	return map[string]string{"success": "agent registered"}
}

// DiscoverAgents returns all agents that have the specified skill
func (s *Supervisor) DiscoverAgents(skill string) []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Agent
	for _, a := range s.agents {
		for _, s := range a.Skills {
			if s == skill {
				result = append(result, a)
				break
			}
		}
	}
	return result
}

// UnregisterAgent removes an agent from the registry
func (s *Supervisor) UnregisterAgent(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.agents[agentID]; exists {
		delete(s.agents, agentID)
		s.saveStateLocked()
		return true
	}
	return false
}

// loadEnv loads environment variables from a .env file
func loadEnv() {
	// Try multiple locations for .env file
	locations := []string{
		".env",
		"/workspace/group/gassy/.env",
		filepath.Join(filepath.Dir(os.Executable()), ".env"),
	}

	for _, loc := range locations {
		f, err := os.Open(loc)
		if err != nil {
			continue
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip comments and empty lines
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// Parse KEY=VALUE
			idx := strings.Index(line, "=")
			if idx > 0 {
				key := strings.TrimSpace(line[:idx])
				value := strings.TrimSpace(line[idx+1:])
				// Don't override if already set
				if _, exists := os.LookupEnv(key); !exists {
					os.Setenv(key, value)
				}
			}
		}
		return
	}
}

func main() {
	// Load .env file for ANTHROPIC_AUTH_TOKEN and other env vars
	loadEnv()

	ctx := context.Background()
	sockPath := "/tmp/gassy-supervisor.sock"

	supervisor := NewSupervisor(sockPath)
	if err := supervisor.Start(ctx); err != nil {
		log.Fatalf("failed to start supervisor: %v", err)
	}

	log.Printf("supervisor started on socket %s", sockPath)

	// Wait for interrupt signal
	<-make(chan struct{})
}
// findAvailablePort finds an available TCP port for agent allocation
func findAvailablePort() int {
	for port := 8080; port <= 9000; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 0 // no port found
}
