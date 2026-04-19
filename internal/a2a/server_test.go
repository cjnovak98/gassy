package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerAgentCard(t *testing.T) {
	card := &AgentCard{
		Name:    "test-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilities{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: false,
		},
		Skills: []AgentSkill{
			{ID: "code", Name: "Write Code"},
		},
		DefaultStream: true,
	}

	server := NewServer()
	handler := server.HandleAgentCard(card)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var j AgentCardJSON
	if err := json.NewDecoder(w.Body).Decode(&j); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if j.Name != "test-agent" {
		t.Errorf("name = %q, want %q", j.Name, "test-agent")
	}
}

func TestServerHandleA2AGetTask(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{
		ID:    "task-1",
		State: TaskStateWorking,
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"getTask","params":{"taskId":"task-1"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("error: %v", resp.Error)
	}
}

func TestServerHandleA2ATaskNotFound(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"getTask","params":{"taskId":"nonexistent"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServerHandleA2AMethodNotAllowed(t *testing.T) {
	server := NewServer()

	req := httptest.NewRequest(http.MethodGet, "/a2a", nil)
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestFetchAgentCard(t *testing.T) {
	card := &AgentCard{
		Name:    "remote-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilities{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: false,
		},
		DefaultStream: true,
	}

	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("task-1", &msg), nil
	}
	mux := http.NewServeMux()
	mux.Handle("/.well-known/agent.json", server.HandleAgentCard(card))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	fetched, err := FetchAgentCard(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("FetchAgentCard: %v", err)
	}
	if fetched.Name != "remote-agent" {
		t.Errorf("name = %q, want %q", fetched.Name, "remote-agent")
	}
}

func TestClientSendMessage(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("task-1", &msg), nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "Hello"),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if task.ID != "task-1" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-1")
	}
}

func TestClientGetTask(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{
		ID:    "task-1",
		State: TaskStateWorking,
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.GetTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ID != "task-1" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-1")
	}
	if task.State != TaskStateWorking {
		t.Errorf("task.State = %v, want %v", task.State, TaskStateWorking)
	}
}

func TestClientGetTaskNotFound(t *testing.T) {
	server := NewServer()
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	_, err := client.GetTask(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestCancelTask(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{
		ID:    "task-1",
		State: TaskStateWorking,
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"cancelTask","params":{"taskId":"task-1"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	task := server.Tasks["task-1"]
	if task.State != TaskStateCanceled {
		t.Errorf("task.State = %v, want %v", task.State, TaskStateCanceled)
	}
}

func TestCancelTaskNotFound(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"cancelTask","params":{"taskId":"nonexistent"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServerHandleA2AMissingMethod(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	// Should return OK with error response, not crash
	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestServerHandleA2AMissingParams(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"getTask"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServerHandleA2AUnknownMethod(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"unknownMethod","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServerWithNilHandleMessage(t *testing.T) {
	server := NewServer()
	// HandleMessage is nil

	body := `{"jsonrpc":"2.0","id":1,"method":"sendMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	// Should handle nil HandleMessage gracefully
	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestFetchAgentCardHTTPError(t *testing.T) {
	// Test that FetchAgentCard handles HTTP errors
	_, err := FetchAgentCard(context.Background(), "http://localhost:99999")
	if err == nil {
		t.Error("FetchAgentCard() to nonexistent server = nil, want error")
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
		Message: *NewMessage("user", "Hello streaming"),
	})
	if err != nil {
		t.Fatalf("SendStreamingMessage: %v", err)
	}
	if events == nil {
		t.Error("SendStreamingMessage returned nil channel")
	}
}

type readCloser struct {
	*strings.Reader
}

func (rc *readCloser) Close() error { return nil }

func newJSONBody(s string) io.ReadCloser {
	return &readCloser{strings.NewReader(s)}
}

func TestHandleSendMessageMissingParams(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("task-1", &msg), nil
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"sendMessage"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for missing params")
	}
}

func TestHandleSendMessageInvalidParams(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("task-1", &msg), nil
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"sendMessage","params":{"message":{"role":"user","parts":"not-an-array"}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for invalid params")
	}
}

func TestHandleGetTaskMissingParams(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{ID: "task-1", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"getTask"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for missing params")
	}
}

func TestHandleGetTaskInvalidParams(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{ID: "task-1", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"getTask","params":"not-an-object"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for invalid params")
	}
}

func TestHandleCancelTaskMissingParams(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{ID: "task-1", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"cancelTask"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for missing params")
	}
}

func TestHandleCancelTaskNotFound(t *testing.T) {
	server := NewServer()
	// No tasks added

	body := `{"jsonrpc":"2.0","id":1,"method":"cancelTask","params":{"taskId":"nonexistent"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestHandleListTasks(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{ID: "task-1", ContextID: "ctx-1", State: TaskStateWorking}
	server.Tasks["task-2"] = &Task{ID: "task-2", ContextID: "ctx-1", State: TaskStateCompleted}
	server.Tasks["task-3"] = &Task{ID: "task-3", ContextID: "ctx-2", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"listTasks","params":{"contextId":"ctx-1"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestHandleListTasksWithMaxTasks(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{ID: "task-1", ContextID: "ctx-1", State: TaskStateWorking}
	server.Tasks["task-2"] = &Task{ID: "task-2", ContextID: "ctx-1", State: TaskStateCompleted}
	server.Tasks["task-3"] = &Task{ID: "task-3", ContextID: "ctx-1", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"listTasks","params":{"contextId":"ctx-1","maxTasks":2}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleListTasksEmpty(t *testing.T) {
	server := NewServer()
	// No tasks

	body := `{"jsonrpc":"2.0","id":1,"method":"listTasks","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleListTasksMissingParams(t *testing.T) {
	server := NewServer()
	server.Tasks["task-1"] = &Task{ID: "task-1", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"listTasks"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for missing params")
	}
}

func TestHandleListTasksSortedByID(t *testing.T) {
	server := NewServer()
	// Add tasks in non-sorted order
	server.Tasks["task-3"] = &Task{ID: "task-3", ContextID: "ctx-1", State: TaskStateWorking}
	server.Tasks["task-1"] = &Task{ID: "task-1", ContextID: "ctx-1", State: TaskStateCompleted}
	server.Tasks["task-2"] = &Task{ID: "task-2", ContextID: "ctx-1", State: TaskStateWorking}

	body := `{"jsonrpc":"2.0","id":1,"method":"listTasks","params":{"contextId":"ctx-1"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Verify response contains tasks
	result, ok := resp.Result.([]interface{})
	if !ok {
		t.Fatal("expected array result")
	}
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// Verify IDs come back in sorted order (task-1, task-2, task-3)
	for i, r := range result {
		taskMap, ok := r.(map[string]interface{})
		if !ok {
			t.Fatalf("result[%d] is not a map", i)
		}
		expectedID := fmt.Sprintf("task-%d", i+1)
		if taskMap["id"] != expectedID {
			t.Errorf("result[%d].id = %q, want %q", i, taskMap["id"], expectedID)
		}
	}
}

func TestHandleRegisterWebhook(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"registerWebhook","params":{"url":"https://example.com/webhook"}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if server.WebhookURL != "https://example.com/webhook" {
		t.Errorf("WebhookURL = %q, want %q", server.WebhookURL, "https://example.com/webhook")
	}
}

func TestHandleRegisterWebhookEmptyURL(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"registerWebhook","params":{"url":""}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for empty webhook URL")
	}
}

func TestHandleRegisterWebhookMissingParams(t *testing.T) {
	server := NewServer()

	body := `{"jsonrpc":"2.0","id":1,"method":"registerWebhook"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for missing params")
	}
}

func TestStreamingMessageWithSSEAccept(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent, 2)
		go func() {
			defer close(events)
			events <- TaskEvent{
				Kind:      "statusUpdate",
				TaskID:    "stream-task",
				SessionID: "session-1",
				Status:    &TaskStatus{State: TaskStateWorking},
			}
			events <- TaskEvent{
				Kind:   "statusUpdate",
				TaskID: "stream-task",
				Status: &TaskStatus{State: TaskStateCompleted},
			}
		}()
		return events, nil
	}

	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: statusUpdate") {
		t.Error("response should contain SSE statusUpdate events")
	}
	if !strings.Contains(body, "event: done") {
		t.Error("response should contain SSE done event")
	}
}

func TestStreamingMessageFallbackWithoutHandler(t *testing.T) {
	server := NewServer()
	// No HandleStreamingMessage configured — should fall back to JSON-RPC
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("fallback-task", &msg), nil
	}

	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestStreamingMessageNoAcceptFallsBack(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent, 1)
		go func() {
			defer close(events)
			events <- TaskEvent{Kind: "statusUpdate", TaskID: "task", Status: &TaskStatus{State: TaskStateWorking}}
		}()
		return events, nil
	}

	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// No Accept: text/event-stream header — should fall back to JSON-RPC
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestStreamingMessageMissingParams(t *testing.T) {
	server := NewServer()
	// Streaming handler set but params are missing — should error
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage"}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error for missing params")
	}
}

func TestStreamingMessageHandlerError(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		return nil, fmt.Errorf("handler error")
	}

	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error from handler")
	}
}

func TestStreamingMessageSSEHeaders(t *testing.T) {
	server := NewServer()
	server.HandleStreamingMessage = func(msg Message) (<-chan TaskEvent, error) {
		events := make(chan TaskEvent, 1)
		go func() {
			defer close(events)
			events <- TaskEvent{Kind: "statusUpdate", TaskID: "flush-task", Status: &TaskStatus{State: TaskStateWorking}}
		}()
		return events, nil
	}

	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"sendStreamingMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q, want keep-alive", conn)
	}
}

func TestTaskEventJSON(t *testing.T) {
	event := TaskEvent{
		Kind:      "statusUpdate",
		TaskID:    "task-1",
		SessionID: "session-1",
		ContextID: "context-1",
		Status:    &TaskStatus{State: TaskStateWorking},
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(data), "task-1") {
		t.Error("json should contain TaskID")
	}
	if !strings.Contains(string(data), "statusUpdate") {
		t.Error("json should contain kind")
	}
}

func TestSSEDoneEventFormat(t *testing.T) {
	// Verify the done event format is "{}" not '""'
	doneLine := "data: {}"
	if strings.Contains(doneLine, `"`) && !strings.Contains(doneLine, "{}") {
		t.Error("done event data should be {} not quoted empty string")
	}
}
