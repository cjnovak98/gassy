package beads

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s := New("localhost:8080")
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.addr != "localhost:8080" {
		t.Errorf("addr = %q, want %q", s.addr, "localhost:8080")
	}
	if s.tickets == nil {
		t.Error("tickets map not initialized")
	}
	if s.budget == nil {
		t.Error("budget map not initialized")
	}
}

func TestCreateTicket(t *testing.T) {
	s := New("localhost:8080")
	ticket, err := s.CreateTicket(context.Background(), "mayor", "Test prompt")
	if err != nil {
		t.Fatalf("CreateTicket() error = %v", err)
	}
	if ticket.AgentID != "mayor" {
		t.Errorf("AgentID = %q, want %q", ticket.AgentID, "mayor")
	}
	if ticket.Prompt != "Test prompt" {
		t.Errorf("Prompt = %q, want %q", ticket.Prompt, "Test prompt")
	}
	if ticket.Status != "open" {
		t.Errorf("Status = %q, want %q", ticket.Status, "open")
	}
	if ticket.ID == "" {
		t.Error("ID is empty")
	}
	if ticket.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestCreateTicketUniqueIDs(t *testing.T) {
	s := New("localhost:8080")
	t1, _ := s.CreateTicket(context.Background(), "a", "prompt 1")
	t2, _ := s.CreateTicket(context.Background(), "a", "prompt 2")
	if t1.ID == t2.ID {
		t.Error("Each ticket should have a unique ID")
	}
}

func TestGetTicket(t *testing.T) {
	s := New("localhost:8080")
	created, _ := s.CreateTicket(context.Background(), "agent1", "test prompt")

	ticket, err := s.GetTicket(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetTicket() error = %v", err)
	}
	if ticket.ID != created.ID {
		t.Errorf("ticket.ID = %q, want %q", ticket.ID, created.ID)
	}
	if ticket.AgentID != "agent1" {
		t.Errorf("AgentID = %q, want %q", ticket.AgentID, "agent1")
	}
}

func TestGetTicketNotFound(t *testing.T) {
	s := New("localhost:8080")
	_, err := s.GetTicket(context.Background(), "nonexistent-id")
	if err == nil {
		t.Error("GetTicket() for nonexistent = nil, want error")
	}
}

func TestUpdateTicketStatus(t *testing.T) {
	s := New("localhost:8080")
	ticket, _ := s.CreateTicket(context.Background(), "agent1", "test")

	err := s.UpdateTicketStatus(context.Background(), ticket.ID, "in-progress")
	if err != nil {
		t.Fatalf("UpdateTicketStatus() error = %v", err)
	}

	updated, _ := s.GetTicket(context.Background(), ticket.ID)
	if updated.Status != "in-progress" {
		t.Errorf("Status = %q, want %q", updated.Status, "in-progress")
	}
}

func TestUpdateTicketStatusNotFound(t *testing.T) {
	s := New("localhost:8080")
	err := s.UpdateTicketStatus(context.Background(), "nonexistent-id", "closed")
	if err == nil {
		t.Error("UpdateTicketStatus() for nonexistent = nil, want error")
	}
}

func TestGetOpenTickets(t *testing.T) {
	s := New("localhost:8080")
	_, _ = s.CreateTicket(context.Background(), "agent1", "task 1")
	_, _ = s.CreateTicket(context.Background(), "agent1", "task 2")
	_, _ = s.CreateTicket(context.Background(), "agent2", "task 3")

	// Close one ticket
	ticket, _ := s.CreateTicket(context.Background(), "agent1", "task to close")
	s.CloseTicket(context.Background(), ticket.ID)

	open, err := s.GetOpenTickets(context.Background(), "agent1")
	if err != nil {
		t.Fatalf("GetOpenTickets() error = %v", err)
	}
	if len(open) != 2 {
		t.Errorf("len(open) = %d, want 2", len(open))
	}
}

func TestGetOpenTicketsNone(t *testing.T) {
	s := New("localhost:8080")
	open, err := s.GetOpenTickets(context.Background(), "nonexistent-agent")
	if err != nil {
		t.Fatalf("GetOpenTickets() error = %v", err)
	}
	if len(open) != 0 {
		t.Errorf("len(open) = %d, want 0", len(open))
	}
}

func TestGetBudget(t *testing.T) {
	s := New("localhost:8080")

	budget, err := s.GetBudget(context.Background(), "agent1")
	if err != nil {
		t.Fatalf("GetBudget() error = %v", err)
	}
	if budget != 0 {
		t.Errorf("budget = %f, want 0 for unset budget", budget)
	}

	s.SetBudget(context.Background(), "agent1", 100.0)
	budget, err = s.GetBudget(context.Background(), "agent1")
	if err != nil {
		t.Fatalf("GetBudget() error = %v", err)
	}
	if budget != 100.0 {
		t.Errorf("budget = %f, want 100.0", budget)
	}
}

func TestDeductBudget(t *testing.T) {
	s := New("localhost:8080")
	s.SetBudget(context.Background(), "agent1", 100.0)

	err := s.DeductBudget(context.Background(), "agent1", 30.0)
	if err != nil {
		t.Fatalf("DeductBudget() error = %v", err)
	}

	budget, _ := s.GetBudget(context.Background(), "agent1")
	if budget != 70.0 {
		t.Errorf("budget after deduction = %f, want 70.0", budget)
	}
}

func TestDeductBudgetInsufficient(t *testing.T) {
	s := New("localhost:8080")
	s.SetBudget(context.Background(), "agent1", 50.0)

	err := s.DeductBudget(context.Background(), "agent1", 100.0)
	if err == nil {
		t.Error("DeductBudget() with insufficient budget = nil, want error")
	}

	// Budget should be unchanged
	budget, _ := s.GetBudget(context.Background(), "agent1")
	if budget != 50.0 {
		t.Errorf("budget = %f, want 50.0 after failed deduction", budget)
	}
}

func TestDeductBudgetNoBudget(t *testing.T) {
	s := New("localhost:8080")
	err := s.DeductBudget(context.Background(), "nonexistent-agent", 10.0)
	if err == nil {
		t.Error("DeductBudget() for agent with no budget = nil, want error")
	}
}

func TestCloseTicket(t *testing.T) {
	s := New("localhost:8080")
	ticket, _ := s.CreateTicket(context.Background(), "agent1", "task")

	err := s.CloseTicket(context.Background(), ticket.ID)
	if err != nil {
		t.Fatalf("CloseTicket() error = %v", err)
	}

	closed, _ := s.GetTicket(context.Background(), ticket.ID)
	if closed.Status != "closed" {
		t.Errorf("Status = %q, want %q", closed.Status, "closed")
	}
}

func TestCloseTicketNotFound(t *testing.T) {
	s := New("localhost:8080")
	err := s.CloseTicket(context.Background(), "nonexistent-id")
	if err == nil {
		t.Error("CloseTicket() for nonexistent = nil, want error")
	}
}

func TestStoreWithEmptyAddr(t *testing.T) {
	s := New("")
	if s.addr != "" {
		t.Errorf("addr = %q, want empty string", s.addr)
	}
	ticket, err := s.CreateTicket(context.Background(), "test", "prompt")
	if err != nil {
		t.Fatalf("CreateTicket() with empty addr error = %v", err)
	}
	if ticket.AgentID != "test" {
		t.Errorf("AgentID = %q, want %q", ticket.AgentID, "test")
	}
}

func TestTicketTimestamps(t *testing.T) {
	s := New("localhost:8080")
	before := time.Now()
	ticket, _ := s.CreateTicket(context.Background(), "a", "prompt")
	after := time.Now()

	if ticket.CreatedAt.Before(before) || ticket.CreatedAt.After(after) {
		t.Errorf("CreatedAt timestamp out of expected range")
	}
}

func TestSetBudget(t *testing.T) {
	s := New("localhost:8080")

	err := s.SetBudget(context.Background(), "agent1", 200.0)
	if err != nil {
		t.Fatalf("SetBudget() error = %v", err)
	}

	budget, _ := s.GetBudget(context.Background(), "agent1")
	if budget != 200.0 {
		t.Errorf("budget = %f, want 200.0", budget)
	}
}

func TestMultipleAgentsBudget(t *testing.T) {
	s := New("localhost:8080")

	s.SetBudget(context.Background(), "agent1", 100.0)
	s.SetBudget(context.Background(), "agent2", 200.0)
	s.SetBudget(context.Background(), "agent3", 300.0)

	b1, _ := s.GetBudget(context.Background(), "agent1")
	b2, _ := s.GetBudget(context.Background(), "agent2")
	b3, _ := s.GetBudget(context.Background(), "agent3")

	if b1 != 100.0 || b2 != 200.0 || b3 != 300.0 {
		t.Errorf("budgets = %f, %f, %f, want 100, 200, 300", b1, b2, b3)
	}
}

func TestConcurrentTicketCreation(t *testing.T) {
	s := New("localhost:8080")
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			_, err := s.CreateTicket(context.Background(), fmt.Sprintf("agent-%d", idx%3), fmt.Sprintf("prompt %d", idx))
			if err != nil {
				t.Errorf("CreateTicket() error = %v", err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all tickets were created
	open, _ := s.GetOpenTickets(context.Background(), "agent-0")
	if len(open) != 4 { // agents 0, 3, 6, 9 -> 4 tickets
		t.Errorf("agent-0 has %d tickets, want 4", len(open))
	}
}

func TestConcurrentBudgetDeduction(t *testing.T) {
	s := New("localhost:8080")
	s.SetBudget(context.Background(), "agent1", 1000.0)

	errChan := make(chan error, 100)
	for i := 0; i < 100; i++ {
		go func() {
			err := s.DeductBudget(context.Background(), "agent1", 10.0)
			errChan <- err
		}()
	}

	var lastErr error
	for i := 0; i < 100; i++ {
		if err := <-errChan; err != nil {
			lastErr = err
		}
	}

	// Should have many failures due to insufficient balance
	if lastErr == nil {
		t.Error("expected some deductions to fail due to insufficient budget")
	}

	// Budget should be around 0 (100 deductions of 10 from 1000)
	budget, _ := s.GetBudget(context.Background(), "agent1")
	if budget > 100 {
		t.Errorf("budget = %f, expected near 0 after over-deduction", budget)
	}
}

func TestDeductBudgetExactAmount(t *testing.T) {
	s := New("localhost:8080")
	s.SetBudget(context.Background(), "agent1", 100.0)

	err := s.DeductBudget(context.Background(), "agent1", 100.0)
	if err != nil {
		t.Fatalf("DeductBudget(100) error = %v", err)
	}

	budget, _ := s.GetBudget(context.Background(), "agent1")
	if budget != 0 {
		t.Errorf("budget = %f, want 0", budget)
	}
}

func TestUpdateTicketStatusMultipleTimes(t *testing.T) {
	s := New("localhost:8080")
	ticket, _ := s.CreateTicket(context.Background(), "agent1", "test")

	statuses := []string{"open", "in-progress", "review", "closed"}
	for _, status := range statuses {
		err := s.UpdateTicketStatus(context.Background(), ticket.ID, status)
		if err != nil {
			t.Fatalf("UpdateTicketStatus(%s) error = %v", status, err)
		}
	}

	final, _ := s.GetTicket(context.Background(), ticket.ID)
	if final.Status != "closed" {
		t.Errorf("final Status = %q, want %q", final.Status, "closed")
	}
}

func TestGetOpenTicketsAllAgents(t *testing.T) {
	s := New("localhost:8080")

	// Create tickets for multiple agents
	_, _ = s.CreateTicket(context.Background(), "agent1", "task 1")
	_, _ = s.CreateTicket(context.Background(), "agent2", "task 2")
	_, _ = s.CreateTicket(context.Background(), "agent1", "task 3")

	// Get all open tickets for agent1 only
	open1, _ := s.GetOpenTickets(context.Background(), "agent1")
	if len(open1) != 2 {
		t.Errorf("agent1 open tickets = %d, want 2", len(open1))
	}
}