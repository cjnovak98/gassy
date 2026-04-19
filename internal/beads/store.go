package beads

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Store provides an interface to the Beads issue tracker
type Store struct {
	// TODO: actual Beads RPC client connection
	addr string

	mu      sync.RWMutex
	tickets map[string]*Ticket
	budget  map[string]float64
}

// New creates a new Beads store client
func New(addr string) *Store {
	return &Store{
		addr:    addr,
		tickets: make(map[string]*Ticket),
		budget:  make(map[string]float64),
	}
}

// Ticket represents a Beads issue/ticket
type Ticket struct {
	ID        string
	AgentID   string
	Prompt    string
	Status    string
	CreatedAt time.Time
}

// CreateTicket creates a new Beads ticket
func (s *Store) CreateTicket(ctx context.Context, agentID, prompt string) (*Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticket := &Ticket{
		ID:        "ticket-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		AgentID:   agentID,
		Prompt:    prompt,
		Status:    "open",
		CreatedAt: time.Now(),
	}
	s.tickets[ticket.ID] = ticket
	return ticket, nil
}

// GetTicket retrieves a ticket by ID
func (s *Store) GetTicket(ctx context.Context, ticketID string) (*Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ticket, ok := s.tickets[ticketID]
	if !ok {
		return nil, fmt.Errorf("ticket not found: %s", ticketID)
	}
	return ticket, nil
}

// UpdateTicketStatus updates the status of a ticket
func (s *Store) UpdateTicketStatus(ctx context.Context, ticketID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticket, ok := s.tickets[ticketID]
	if !ok {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}
	ticket.Status = status
	return nil
}

// GetOpenTickets returns all open tickets for an agent
func (s *Store) GetOpenTickets(ctx context.Context, agentID string) ([]*Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var open []*Ticket
	for _, t := range s.tickets {
		if t.AgentID == agentID && t.Status == "open" {
			open = append(open, t)
		}
	}
	return open, nil
}

// GetBudget returns the remaining budget for an agent
func (s *Store) GetBudget(ctx context.Context, agentID string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	budget, ok := s.budget[agentID]
	if !ok {
		return 0, nil
	}
	return budget, nil
}

// SetBudget sets the budget for an agent
func (s *Store) SetBudget(ctx context.Context, agentID string, amount float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.budget[agentID] = amount
	return nil
}

// DeductBudget deducts from an agent's budget
func (s *Store) DeductBudget(ctx context.Context, agentID string, amount float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.budget[agentID]
	if !ok {
		return fmt.Errorf("no budget set for agent: %s", agentID)
	}
	if current < amount {
		return fmt.Errorf("insufficient budget: have %.2f, need %.2f", current, amount)
	}
	s.budget[agentID] = current - amount
	return nil
}

// CloseTicket marks a ticket as closed
func (s *Store) CloseTicket(ctx context.Context, ticketID string) error {
	return s.UpdateTicketStatus(ctx, ticketID, "closed")
}
