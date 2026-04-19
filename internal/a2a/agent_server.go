package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// AgentServer wraps an A2A Server with HTTP serving for a single agent
type AgentServer struct {
	Name           string
	URL            string
	Skills         []AgentSkill
	Capabilities   AgentCapabilities
	HandleMessage  func(Message) (*Task, error)
	DefaultStream  bool

	server   *Server
	httpSrv  *http.Server
	listener net.Listener
}

// NewAgentServer creates a new agent server for a specific agent
func NewAgentServer(name, url string, skills []AgentSkill, caps AgentCapabilities, handler func(Message) (*Task, error)) *AgentServer {
	s := &AgentServer{
		Name:          name,
		URL:           url,
		Skills:        skills,
		Capabilities:  caps,
		HandleMessage: handler,
		DefaultStream: true,
	}
	return s
}

// AgentCard returns the agent's AgentCard
func (s *AgentServer) AgentCard() *AgentCard {
	return &AgentCard{
		Name:          s.Name,
		Version:       "1.0",
		Url:           s.URL,
		Capabilities:  s.Capabilities,
		Skills:        s.Skills,
		DefaultStream: s.DefaultStream,
	}
}

// Start begins serving the A2A endpoints on an available port
func (s *AgentServer) Start(ctx context.Context) error {
	if s.HandleMessage == nil {
		return fmt.Errorf("no message handler configured")
	}

	s.server = NewServer()
	s.server.HandleMessage = s.HandleMessage

	mux := http.NewServeMux()

	card := s.AgentCard()
	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card.ToJSON())
	})

	mux.HandleFunc("/a2a", func(w http.ResponseWriter, r *http.Request) {
		s.server.HandleA2A()(w, r)
	})

	s.httpSrv = &http.Server{
		Handler: mux,
	}

	// Listen on localhost with port 0 for auto-assigned port
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = ln

	go s.httpSrv.Serve(ln)

	return nil
}

// StartWithAddr starts the server on a specific address
func (s *AgentServer) StartWithAddr(ctx context.Context, addr string) error {
	if s.HandleMessage == nil {
		return fmt.Errorf("no message handler configured")
	}

	s.server = NewServer()
	s.server.HandleMessage = s.HandleMessage

	mux := http.NewServeMux()

	card := s.AgentCard()
	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card.ToJSON())
	})

	mux.HandleFunc("/a2a", func(w http.ResponseWriter, r *http.Request) {
		s.server.HandleA2A()(w, r)
	})

	s.httpSrv = &http.Server{
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = ln

	go s.httpSrv.Serve(ln)

	return nil
}

// Stop stops the HTTP server
func (s *AgentServer) Stop(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return s.httpSrv.Shutdown(shutdownCtx)
}

// Address returns the address the server is listening on
func (s *AgentServer) Address() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
