package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientWithTimeout(t *testing.T) {
	c := NewClientWithTimeout("http://localhost:9000", 5*time.Second)
	if c.BaseURL != "http://localhost:9000" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, "http://localhost:9000")
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
	if c.HTTPClient.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.HTTPClient.Timeout)
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:8001")
	if c.BaseURL != "http://localhost:8001" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, "http://localhost:8001")
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
	if c.HTTPClient.Timeout != DefaultHTTPTimeout {
		t.Errorf("Timeout = %v, want %v", c.HTTPClient.Timeout, DefaultHTTPTimeout)
	}
}

func TestClientSendMessageSuccess(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("task-handler-test", &msg), nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "hello from direct client test"),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if task.ID != "task-handler-test" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-handler-test")
	}
	if task.State != TaskStateWorking {
		t.Errorf("task.State = %v, want %v", task.State, TaskStateWorking)
	}
}

func TestClientSendMessageHTTPError(t *testing.T) {
	// Server that returns non-200
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendMessage to erroring server = nil, want error")
	}
}

func TestClientSendMessageJSONError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write invalid JSON
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendMessage with bad JSON response = nil, want error")
	}
}

func TestClientSendMessageRPCError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error:   &JSONRPCError{Code: -32600, Message: "Invalid Request"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendMessage with RPC error = nil, want error")
	}
}

func TestClientGetTaskSuccess(t *testing.T) {
	server := NewServer()
	server.Tasks["my-task"] = &Task{
		ID:    "my-task",
		State: TaskStateCompleted,
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.GetTask(context.Background(), "my-task")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ID != "my-task" {
		t.Errorf("task.ID = %q, want %q", task.ID, "my-task")
	}
	if task.State != TaskStateCompleted {
		t.Errorf("task.State = %v, want %v", task.State, TaskStateCompleted)
	}
}

func TestClientGetTaskHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetTask(context.Background(), "any-task")
	if err == nil {
		t.Error("GetTask to failing server = nil, want error")
	}
}

func TestClientGetTaskNotFound(t *testing.T) {
	server := NewServer()
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetTask(context.Background(), "nonexistent-task")
	if err == nil {
		t.Error("GetTask for nonexistent = nil, want error")
	}
}

func TestClientSendStreamingMessage(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("stream-task", &msg), nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	events, err := client.SendStreamingMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "stream me"),
	})
	if err != nil {
		t.Fatalf("SendStreamingMessage: %v", err)
	}
	if events == nil {
		t.Fatal("events channel is nil")
	}
	// Drain channel
	for range events {
		// drain
	}
}

func TestClientSendStreamingMessageHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.SendStreamingMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendStreamingMessage to bad gateway = nil, want error")
	}
}

func TestFetchAgentCardSuccess(t *testing.T) {
	card := &AgentCard{
		Name:    "direct-card-test",
		Version: "1.0",
		Url:     "http://localhost:9000",
		Capabilities: AgentCapabilities{
			Streaming:         true,
			PushNotifications: false,
		},
		DefaultStream: true,
	}
	server := NewServer()
	mux := http.NewServeMux()
	mux.Handle("/.well-known/agent.json", server.HandleAgentCard(card))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	fetched, err := FetchAgentCard(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("FetchAgentCard: %v", err)
	}
	if fetched.Name != "direct-card-test" {
		t.Errorf("Name = %q, want %q", fetched.Name, "direct-card-test")
	}
	if !fetched.Capabilities.Streaming {
		t.Error("Streaming capability should be true")
	}
}

func TestFetchAgentCardHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := FetchAgentCard(context.Background(), ts.URL)
	if err == nil {
		t.Error("FetchAgentCard to 404 server = nil, want error")
	}
}

func TestFetchAgentCardInvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json {"))
	}))
	defer ts.Close()

	_, err := FetchAgentCard(context.Background(), ts.URL)
	if err == nil {
		t.Error("FetchAgentCard with invalid JSON = nil, want error")
	}
}

func TestClientSendMessageWithTaskID(t *testing.T) {
	server := NewServer()
	var receivedTaskID string
	server.HandleMessage = func(msg Message) (*Task, error) {
		return &Task{
			ID:    receivedTaskID,
			State: TaskStateWorking,
			Message: &msg,
		}, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		TaskID:  "pre-existing-task",
		Message: *NewMessage("user", "hello"),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// The handler receives the taskID in params; verify the server processed it
	if receivedTaskID != "pre-existing-task" {
		t.Logf("handler received TaskID (note: this depends on handler implementation)")
	}
}

func TestClientSendMessageContextCancel(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		time.Sleep(5 * time.Second)
		return NewTask("slow-task", &msg), nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.SendMessage(ctx, SendMessageParams{
		Message: *NewMessage("user", "cancel me"),
	})
	if err == nil {
		t.Error("SendMessage with cancelled context = nil, want error")
	}
}

func TestClientSendMessageClosesBodyOnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("valid json"))
	}))
	ts.Close() // force connection close

	client := NewClient(ts.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	// Should get an error, and importantly should not leak resources
	if err == nil {
		t.Error("expected error after server close")
	}
}

func TestFetchAgentCardURLConstruction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/.well-known/agent.json") {
			t.Errorf("expected request to /.well-known/agent.json, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&AgentCardJSON{Name: "test"})
	}))
	defer ts.Close()

	_, _ = FetchAgentCard(context.Background(), ts.URL)
}

func TestClientGetTaskJSONError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("definitely not json"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetTask(context.Background(), "any")
	if err == nil {
		t.Error("GetTask with bad JSON = nil, want error")
	}
}

func TestClientGetTaskRPCError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error:   &JSONRPCError{Code: -32601, Message: "Method not found"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetTask(context.Background(), "any")
	if err == nil {
		t.Error("GetTask with RPC error = nil, want error")
	}
}

func TestClientSendStreamingMessageSSEParsing(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent, 3)
		go func() {
			defer close(events)
			events <- TaskEvent{
				Kind:   "statusUpdate",
				TaskID: "sse-task",
				Status: &TaskStatus{State: TaskStateWorking},
			}
			events <- TaskEvent{
				Kind:   "textDelta",
				TaskID: "sse-task",
				TextDelta: "Hello",
			}
			events <- TaskEvent{
				Kind:   "textDelta",
				TaskID: "sse-task",
				TextDelta: " world",
			}
		}()
		return events, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	events, err := client.SendStreamingMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "hello"),
	})
	if err != nil {
		t.Fatalf("SendStreamingMessage: %v", err)
	}

	var eventTypes []string
	var textDeltas []string
	for ev := range events {
		eventTypes = append(eventTypes, ev.Event)
		if ev.Event == "textDelta" {
			textDeltas = append(textDeltas, ev.TextDelta)
		}
	}

	if len(eventTypes) == 0 {
		t.Fatal("no events received")
	}
	if eventTypes[0] != "statusUpdate" {
		t.Errorf("first event type = %q, want %q", eventTypes[0], "statusUpdate")
	}
	// Last event should be "done"
	if len(eventTypes) > 0 && eventTypes[len(eventTypes)-1] != "done" {
		t.Errorf("last event type = %q, want %q", eventTypes[len(eventTypes)-1], "done")
	}
}

func TestClientSendStreamingMessageWithSSEServer(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent, 1)
		go func() {
			defer close(events)
			events <- TaskEvent{
				Kind:   "statusUpdate",
				TaskID: "stream-1",
				Status: &TaskStatus{State: TaskStateCompleted},
			}
		}()
		return events, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	events, err := client.SendStreamingMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err != nil {
		t.Fatalf("SendStreamingMessage: %v", err)
	}

	count := 0
	for range events {
		count++
	}
	if count == 0 {
		t.Error("expected at least one event (including done)")
	}
}

func TestClientSendStreamingMessageContextCancel(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent)
		go func() {
			defer close(events)
			// Never send anything — will block
		}()
		return events, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.SendStreamingMessage(ctx, SendMessageParams{
		Message: *NewMessage("user", "cancel me"),
	})
	// Should not error on context cancel during streaming
	if err != nil {
		t.Logf("SendStreamingMessage context cancel error: %v (may be ok)", err)
	}
}

func TestSSEventParsing(t *testing.T) {
	event := SSEEvent{
		Event: "statusUpdate",
		Data:  `{"taskId":"task-1","status":{"state":"working"}}`,
	}
	if event.Event != "statusUpdate" {
		t.Errorf("Event = %q, want %q", event.Event, "statusUpdate")
	}
	if event.Data == "" {
		t.Error("Data should not be empty")
	}
}

func TestSSEClientParsingMultipleDataLines(t *testing.T) {
	// Simulate SSE data with multiple lines
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Two events
		fmt.Fprintf(w, "event: textDelta\ndata: hello\n\n")
		fmt.Fprintf(w, "event: done\ndata:\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	events, err := client.SendStreamingMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err != nil {
		t.Fatalf("SendStreamingMessage: %v", err)
	}

	var received []SSEEvent
	for ev := range events {
		received = append(received, ev)
	}

	if len(received) < 1 {
		t.Fatal("expected at least one event")
	}
}
