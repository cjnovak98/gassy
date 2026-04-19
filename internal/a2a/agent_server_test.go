package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func makeTestHandler(taskID string) func(Message) (*Task, error) {
	return func(msg Message) (*Task, error) {
		return NewTask(taskID, &msg), nil
	}
}

func TestNewAgentServer(t *testing.T) {
	skills := []AgentSkill{{ID: "code", Name: "Write Code"}}
	caps := AgentCapabilities{Streaming: true}
	s := NewAgentServer("test-agent", "http://localhost:8001", skills, caps, makeTestHandler("task-1"))

	if s.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", s.Name, "test-agent")
	}
	if s.URL != "http://localhost:8001" {
		t.Errorf("URL = %q, want %q", s.URL, "http://localhost:8001")
	}
	if len(s.Skills) != 1 || s.Skills[0].ID != "code" {
		t.Errorf("Skills = %v, want [{code Write Code}]", s.Skills)
	}
	if !s.DefaultStream {
		t.Error("DefaultStream should be true by default")
	}
}

func TestAgentServerAgentCard(t *testing.T) {
	skills := []AgentSkill{{ID: "research", Name: "Research"}}
	caps := AgentCapabilities{Streaming: true, PushNotifications: false}
	s := NewAgentServer("researcher", "http://localhost:9001", skills, caps, makeTestHandler("t1"))

	card := s.AgentCard()
	if card.Name != "researcher" {
		t.Errorf("card.Name = %q, want %q", card.Name, "researcher")
	}
	if card.Version != "1.0" {
		t.Errorf("card.Version = %q, want %q", card.Version, "1.0")
	}
	if card.Url != "http://localhost:9001" {
		t.Errorf("card.Url = %q, want %q", card.Url, "http://localhost:9001")
	}
	if !card.Capabilities.Streaming {
		t.Error("card.Capabilities.Streaming should be true")
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "research" {
		t.Errorf("card.Skills = %v, want [{research Research}]", card.Skills)
	}
	if !card.DefaultStream {
		t.Error("card.DefaultStream should be true")
	}
}

func TestAgentServerStartStop(t *testing.T) {
	ctx := context.Background()
	s := NewAgentServer("test", "http://localhost", nil, AgentCapabilities{}, makeTestHandler("t1"))

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	addr := s.Address()
	if addr == "" {
		t.Fatal("Address() returned empty string after Start")
	}

	if err := s.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestAgentServerAddressBeforeStart(t *testing.T) {
	s := NewAgentServer("test", "http://localhost", nil, AgentCapabilities{}, makeTestHandler("t1"))
	if s.Address() != "" {
		t.Error("Address() should return empty string before Start")
	}
}

func TestAgentServerNoHandler(t *testing.T) {
	ctx := context.Background()
	s := &AgentServer{
		Name: "no-handler",
		URL:  "http://localhost",
	}

	if err := s.Start(ctx); err == nil {
		t.Error("Start() with nil HandleMessage should return error")
		_ = s.Stop(ctx)
	}
}

func TestAgentServerNoHandlerWithAddr(t *testing.T) {
	ctx := context.Background()
	s := &AgentServer{
		Name: "no-handler",
		URL:  "http://localhost",
	}

	if err := s.StartWithAddr(ctx, "localhost:0"); err == nil {
		t.Error("StartWithAddr() with nil HandleMessage should return error")
		_ = s.Stop(ctx)
	}
}

func TestAgentServerAgentCardEndpoint(t *testing.T) {
	ctx := context.Background()
	skills := []AgentSkill{{ID: "skill1", Name: "Skill One"}}
	caps := AgentCapabilities{Streaming: true}
	s := NewAgentServer("my-agent", "http://example.com", skills, caps, makeTestHandler("t1"))

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(ctx)

	resp, err := http.Get(fmt.Sprintf("http://%s/.well-known/agent.json", s.Address()))
	if err != nil {
		t.Fatalf("GET agent.json: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var card AgentCardJSON
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card: %v", err)
	}
	if card.Name != "my-agent" {
		t.Errorf("card.Name = %q, want %q", card.Name, "my-agent")
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "skill1" {
		t.Errorf("card.Skills = %v, want [{skill1 Skill One}]", card.Skills)
	}
}

func TestAgentServerA2AEndpoint(t *testing.T) {
	ctx := context.Background()
	s := NewAgentServer("test", "http://localhost", nil, AgentCapabilities{}, makeTestHandler("task-42"))

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(ctx)

	body := `{"jsonrpc":"2.0","id":1,"method":"sendMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}`
	resp, err := http.Post(
		fmt.Sprintf("http://%s/a2a", s.Address()),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /a2a: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var rpc JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rpc.Error != nil {
		t.Errorf("unexpected JSON-RPC error: %+v", rpc.Error)
	}
}

func TestAgentServerStartWithAddr(t *testing.T) {
	ctx := context.Background()
	s := NewAgentServer("test", "http://localhost", nil, AgentCapabilities{}, makeTestHandler("t1"))

	if err := s.StartWithAddr(ctx, "localhost:0"); err != nil {
		t.Fatalf("StartWithAddr: %v", err)
	}
	defer s.Stop(ctx)

	if s.Address() == "" {
		t.Error("Address() returned empty string after StartWithAddr")
	}
}

func TestAgentServerStopWithoutStart(t *testing.T) {
	ctx := context.Background()
	s := &AgentServer{}
	// Should not panic or error when nothing is running
	if err := s.Stop(ctx); err != nil {
		t.Errorf("Stop() before Start() returned error: %v", err)
	}
}

func TestAgentServerFetchAgentCardIntegration(t *testing.T) {
	ctx := context.Background()
	skills := []AgentSkill{{ID: "analyze", Name: "Analyze"}}
	s := NewAgentServer("analyzer", "http://localhost", skills, AgentCapabilities{Streaming: true}, makeTestHandler("t1"))

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(ctx)

	url := fmt.Sprintf("http://%s", s.Address())
	card, err := FetchAgentCard(ctx, url)
	if err != nil {
		t.Fatalf("FetchAgentCard: %v", err)
	}
	if card.Name != "analyzer" {
		t.Errorf("card.Name = %q, want %q", card.Name, "analyzer")
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "analyze" {
		t.Errorf("card.Skills = %v", card.Skills)
	}
}

func TestAgentServerStreaming(t *testing.T) {
	ctx := context.Background()
	s := NewAgentServer("stream-agent", "http://localhost", nil, AgentCapabilities{Streaming: true}, func(msg Message) (*Task, error) {
		return NewTask("stream-task", &msg), nil
	})

	s.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent)
		go func() {
			defer close(events)
			events <- TaskEvent{
				Kind:   "statusUpdate",
				TaskID: "stream-task",
				Status: &TaskStatus{State: TaskStateWorking},
			}
			events <- TaskEvent{
				Kind:      "textDelta",
				TaskID:    "stream-task",
				TextDelta: "hello",
			}
			events <- TaskEvent{
				Kind:   "done",
				TaskID: "stream-task",
			}
		}()
		return events, nil
	}

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop(ctx)

	body := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hi"}]}}}`
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("http://%s/a2a", s.Address()), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	// Run handler synchronously for testing
	s.server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestAgentServerStartWithAddrAlreadyInUse(t *testing.T) {
	ctx := context.Background()
	s1 := NewAgentServer("test1", "http://localhost", nil, AgentCapabilities{}, makeTestHandler("t1"))
	if err := s1.StartWithAddr(ctx, "localhost:0"); err != nil {
		t.Fatalf("StartWithAddr: %v", err)
	}
	defer s1.Stop(ctx)

	// Get the port s1 is using
	addr := s1.Address()

	s2 := NewAgentServer("test2", "http://localhost", nil, AgentCapabilities{}, makeTestHandler("t2"))
	if err := s2.StartWithAddr(ctx, addr); err == nil {
		t.Error("StartWithAddr on already-used address should fail")
		s2.Stop(ctx)
	}
}
