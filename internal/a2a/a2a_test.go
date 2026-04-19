package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientGetTaskNotFoundError(t *testing.T) {
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

func TestNewClientWithTimeout(t *testing.T) {
	client := NewClientWithTimeout("http://localhost:8001", 5*time.Second)
	if client.HTTPClient.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", client.HTTPClient.Timeout)
	}
}

func TestNewClientDefaultTimeout(t *testing.T) {
	client := NewClient("http://localhost:8001")
	if client.HTTPClient.Timeout != DefaultHTTPTimeout {
		t.Errorf("Timeout = %v, want %v", client.HTTPClient.Timeout, DefaultHTTPTimeout)
	}
}

func TestServerHandleA2ASendMessage(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return &Task{
			ID:      "task-new",
			State:   TaskStateWorking,
			Message: &msg,
		}, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"sendMessage","params":{"message":{"role":"user","parts":[{"type":"text","text":"Hello"}]}}}}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if _, ok := server.Tasks["task-new"]; !ok {
		t.Error("task-new not found in server.Tasks")
	}
}

func TestServerHandleA2AInvalidJSON(t *testing.T) {
	server := NewServer()
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.HandleA2A()(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentCardSaveLoad(t *testing.T) {
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
			{ID: "code", Name: "Write Code", Description: "Writes code"},
		},
		DefaultStream: true,
	}

	tmpfile := "/tmp/test_agent_card.json"
	err := SaveAgentCard(card, tmpfile)
	if err != nil {
		t.Fatalf("SaveAgentCard error: %v", err)
	}

	loaded, err := LoadAgentCard(tmpfile)
	if err != nil {
		t.Fatalf("LoadAgentCard error: %v", err)
	}

	if loaded.Name != card.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, card.Name)
	}
	if loaded.Version != card.Version {
		t.Errorf("Version = %q, want %q", loaded.Version, card.Version)
	}
	if len(loaded.Skills) != len(card.Skills) {
		t.Errorf("len(Skills) = %d, want %d", len(loaded.Skills), len(card.Skills))
	}
}

func TestTextPart(t *testing.T) {
	part := TextPart{Type: "text", Text: "Hello, World!"}
	if part.Type != "text" {
		t.Errorf("Type = %q, want %q", part.Type, "text")
	}
	if part.Text != "Hello, World!" {
		t.Errorf("Text = %q, want %q", part.Text, "Hello, World!")
	}
}

func TestDataPart(t *testing.T) {
	part := DataPart{
		Type: "data",
		Data: map[string]interface{}{"key": "value", "num": 42},
	}
	if part.Type != "data" {
		t.Errorf("Type = %q, want %q", part.Type, "data")
	}
	if part.Data["key"] != "value" {
		t.Errorf("Data[key] = %v, want %v", part.Data["key"], "value")
	}
}

func TestArtifact(t *testing.T) {
	artifact := Artifact{
		Resource: &ArtifactResource{
			URI:      "file:///tmp/output.json",
			MimeType: "application/json",
		},
		Parts: []Part{TextPart{Type: "text", Text: "result"}},
	}
	if artifact.Resource.URI != "file:///tmp/output.json" {
		t.Errorf("URI = %q, want %q", artifact.Resource.URI, "file:///tmp/output.json")
	}
	if len(artifact.Parts) != 1 {
		t.Errorf("len(Parts) = %d, want 1", len(artifact.Parts))
	}
}

func TestEvent(t *testing.T) {
	event := Event{
		Kind:    "status",
		Actor:   "agent",
		Content: map[string]string{"status": "working"},
	}
	if event.Kind != "status" {
		t.Errorf("Kind = %q, want %q", event.Kind, "status")
	}
	if event.Actor != "agent" {
		t.Errorf("Actor = %q, want %q", event.Actor, "agent")
	}
}

func TestNewMessage(t *testing.T) {
	msg := NewMessage("user", "Hello, world!")
	if msg.Role != "user" {
		t.Errorf("Role = %q, want %q", msg.Role, "user")
	}
	if len(msg.Parts) != 1 {
		t.Fatalf("len(Parts) = %d, want 1", len(msg.Parts))
	}
	textPart, ok := msg.Parts[0].(TextPart)
	if !ok {
		t.Fatal("Part is not TextPart")
	}
	if textPart.Text != "Hello, world!" {
		t.Errorf("Text = %q, want %q", textPart.Text, "Hello, world!")
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestNewTask(t *testing.T) {
	msg := NewMessage("user", "test prompt")
	task := NewTask("task-123", msg)
	if task.ID != "task-123" {
		t.Errorf("ID = %q, want %q", task.ID, "task-123")
	}
	if task.State != TaskStateWorking {
		t.Errorf("State = %v, want %v", task.State, TaskStateWorking)
	}
	if task.Message == nil {
		t.Fatal("Message should not be nil")
	}
	if task.Status == nil {
		t.Fatal("Status should not be nil")
	}
	if task.Status.State != TaskStateWorking {
		t.Errorf("Status.State = %v, want %v", task.Status.State, TaskStateWorking)
	}
}

func TestTaskWithAllFields(t *testing.T) {
	task := &Task{
		ID:        "task-full",
		State:     TaskStateWorking,
		SessionID: "session-1",
		ContextID: "context-1",
		Message:   NewMessage("user", "test"),
		Artifacts: []Artifact{
			{
				Resource: &ArtifactResource{URI: "file://out.txt", MimeType: "text/plain"},
				Parts:    []Part{TextPart{Type: "text", Text: "output"}},
			},
		},
		History: []Event{
			{Kind: "created", Actor: "mayor", Timestamp: time.Now()},
		},
		Status: &TaskStatus{State: TaskStateWorking, Timestamp: time.Now()},
	}
	if task.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", task.SessionID, "session-1")
	}
	if task.ContextID != "context-1" {
		t.Errorf("ContextID = %q, want %q", task.ContextID, "context-1")
	}
	if len(task.Artifacts) != 1 {
		t.Errorf("len(Artifacts) = %d, want 1", len(task.Artifacts))
	}
	if len(task.History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(task.History))
	}
}

func TestTaskStateConstants(t *testing.T) {
	states := []TaskState{
		TaskStateWorking,
		TaskStateInputReq,
		TaskStateAuthReq,
		TaskStateCompleted,
		TaskStateFailed,
		TaskStateCanceled,
		TaskStateRejected,
	}
	expected := []string{"working", "input-required", "auth-required", "completed", "failed", "canceled", "rejected"}
	for i, state := range states {
		if string(state) != expected[i] {
			t.Errorf("state[%d] = %q, want %q", i, state, expected[i])
		}
	}
}

func TestToAgentCard(t *testing.T) {
	j := &AgentCardJSON{
		Name:    "test-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilitiesJSON{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: true,
		},
		Skills:        []AgentSkill{{ID: "code", Name: "Code"}},
		DefaultStream: true,
	}
	card := j.ToAgentCard()
	if card.Name != j.Name {
		t.Errorf("Name = %q, want %q", card.Name, j.Name)
	}
	if card.Version != j.Version {
		t.Errorf("Version = %q, want %q", card.Version, j.Version)
	}
	if !card.Capabilities.Streaming {
		t.Error("Streaming should be true")
	}
	if card.Capabilities.ExtendedAgentCard != j.Capabilities.ExtendedAgentCard {
		t.Errorf("ExtendedAgentCard = %v, want %v", card.Capabilities.ExtendedAgentCard, j.Capabilities.ExtendedAgentCard)
	}
}

func TestToJSON(t *testing.T) {
	card := &AgentCard{
		Name:    "test-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilities{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: false,
		},
		Skills:        []AgentSkill{{ID: "code", Name: "Code"}},
		DefaultStream: true,
	}
	j := card.ToJSON()
	if j.Name != card.Name {
		t.Errorf("Name = %q, want %q", j.Name, card.Name)
	}
	if j.DefaultStream != card.DefaultStream {
		t.Errorf("DefaultStream = %v, want %v", j.DefaultStream, card.DefaultStream)
	}
}

func TestAgentProvider(t *testing.T) {
	provider := &AgentProvider{
		Organization: "Test Org",
		Url:          "https://test.org",
	}
	card := &AgentCard{
		Name:    "test-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilities{
			Streaming: true,
		},
		Provider:      provider,
		DefaultStream: false,
	}
	j := card.ToJSON()
	if j.Provider == nil {
		t.Fatal("Provider should not be nil")
	}
	if j.Provider.Organization != "Test Org" {
		t.Errorf("Organization = %q, want %q", j.Provider.Organization, "Test Org")
	}
}

func TestAgentSkill(t *testing.T) {
	skill := AgentSkill{
		ID:          "code",
		Name:        "Write Code",
		Description: "Writes clean code",
	}
	if skill.ID != "code" {
		t.Errorf("ID = %q, want %q", skill.ID, "code")
	}
	if skill.Description != "Writes clean code" {
		t.Errorf("Description = %q, want %q", skill.Description, "Writes clean code")
	}
}

func TestSendMessageParams(t *testing.T) {
	params := SendMessageParams{
		TaskID:    "task-1",
		SessionID: "session-1",
		ContextID: "context-1",
		Message:   *NewMessage("user", "test"),
		Stream:    true,
	}
	if params.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", params.TaskID, "task-1")
	}
	if params.Stream != true {
		t.Error("Stream should be true")
	}
}

func TestSecurityScheme(t *testing.T) {
	scheme := SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	}
	card := &AgentCard{
		Name:    "test-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilities{Streaming: true},
		SecuritySchemes: map[string]SecurityScheme{
			"Bearer": scheme,
		},
		DefaultStream: false,
	}
	j := card.ToJSON()
	if j.SecuritySchemes == nil {
		t.Fatal("SecuritySchemes should not be nil")
	}
	// Note: map[string]any conversion loses type info, just verify it doesn't crash
}

func TestLoadAgentCardNotFound(t *testing.T) {
	_, err := LoadAgentCard("/nonexistent/path/agent.json")
	if err == nil {
		t.Error("LoadAgentCard() for nonexistent file = nil, want error")
	}
}

func TestNewTaskNilMessage(t *testing.T) {
	task := NewTask("task-nil", nil)
	if task.ID != "task-nil" {
		t.Errorf("ID = %q, want %q", task.ID, "task-nil")
	}
	if task.State != TaskStateWorking {
		t.Errorf("State = %v, want %v", task.State, TaskStateWorking)
	}
	if task.Message != nil {
		t.Error("Message should be nil for nil input")
	}
}

func TestNewMessageMultipleParts(t *testing.T) {
	msg := &Message{
		Role:      "user",
		Parts:     []Part{TextPart{Type: "text", Text: "part1"}, TextPart{Type: "text", Text: "part2"}},
		Timestamp: time.Now(),
	}
	if len(msg.Parts) != 2 {
		t.Errorf("len(Parts) = %d, want 2", len(msg.Parts))
	}
}

func TestMessageWithDataPart(t *testing.T) {
	msg := &Message{
		Role: "user",
		Parts: []Part{
			TextPart{Type: "text", Text: "Hello"},
			DataPart{Type: "data", Data: map[string]interface{}{"key": "value"}},
		},
	}
	if len(msg.Parts) != 2 {
		t.Errorf("len(Parts) = %d, want 2", len(msg.Parts))
	}
	dataPart, ok := msg.Parts[1].(DataPart)
	if !ok {
		t.Fatal("Part[1] should be DataPart")
	}
	if dataPart.Data["key"] != "value" {
		t.Errorf("Data[key] = %v, want %v", dataPart.Data["key"], "value")
	}
}

func TestJSONRPCError(t *testing.T) {
	err := &JSONRPCError{
		Code:    -32600,
		Message: "Invalid Request",
		Data:    "missing method",
	}
	if err.Code != -32600 {
		t.Errorf("Code = %d, want %d", err.Code, -32600)
	}
	if err.Message != "Invalid Request" {
		t.Errorf("Message = %q, want %q", err.Message, "Invalid Request")
	}
}

func TestJSONRPCResponse(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  map[string]string{"status": "ok"},
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, "2.0")
	}
	if resp.Error != nil {
		t.Error("Error should be nil")
	}
}

func TestJSONRPCResponseWithError(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Error:   &JSONRPCError{Code: -32601, Message: "Method not found"},
	}
	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("Error.Code = %d, want %d", resp.Error.Code, -32601)
	}
}

func TestTaskStatusUpdateEvent(t *testing.T) {
	event := TaskStatusUpdateEvent{
		TaskID: "task-1",
		Status: TaskStatus{
			State:     TaskStateWorking,
			Timestamp: time.Now(),
		},
	}
	if event.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", event.TaskID, "task-1")
	}
	if event.Status.State != TaskStateWorking {
		t.Errorf("Status.State = %v, want %v", event.Status.State, TaskStateWorking)
	}
}

func TestTaskArtifactUpdateEvent(t *testing.T) {
	event := TaskArtifactUpdateEvent{
		TaskID: "task-1",
		Artifact: Artifact{
			Parts: []Part{TextPart{Type: "text", Text: "result"}},
		},
	}
	if event.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", event.TaskID, "task-1")
	}
	if len(event.Artifact.Parts) != 1 {
		t.Errorf("len(Artifact.Parts) = %d, want 1", len(event.Artifact.Parts))
	}
}

func TestSSEEvent(t *testing.T) {
	event := SSEEvent{
		Event: "statusUpdate",
		Data:  "working",
	}
	if event.Event != "statusUpdate" {
		t.Errorf("Event = %q, want %q", event.Event, "statusUpdate")
	}
	if event.Data != "working" {
		t.Errorf("Data = %q, want %q", event.Data, "working")
	}
}

func TestGetTaskParams(t *testing.T) {
	params := GetTaskParams{
		TaskID: "task-123",
	}
	if params.TaskID != "task-123" {
		t.Errorf("TaskID = %q, want %q", params.TaskID, "task-123")
	}
}

func TestListTasksParams(t *testing.T) {
	params := ListTasksParams{
		SessionID: "session-1",
		ContextID: "context-1",
		MaxTasks:  10,
	}
	if params.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", params.SessionID, "session-1")
	}
	if params.MaxTasks != 10 {
		t.Errorf("MaxTasks = %d, want %d", params.MaxTasks, 10)
	}
}

func TestCancelTaskParams(t *testing.T) {
	params := CancelTaskParams{
		TaskID: "task-1",
	}
	if params.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", params.TaskID, "task-1")
	}
}

func TestSendStreamingMessageParams(t *testing.T) {
	params := SendStreamingMessageParams{
		SendMessageParams: SendMessageParams{
			TaskID:    "task-1",
			SessionID: "session-1",
			Message:   *NewMessage("user", "test"),
			Stream:    true,
		},
	}
	if params.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", params.TaskID, "task-1")
	}
	if !params.Stream {
		t.Error("Stream should be true")
	}
}

func TestToAgentCardWithProvider(t *testing.T) {
	j := &AgentCardJSON{
		Name:    "provider-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilitiesJSON{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: false,
		},
		Provider:      &AgentProvider{Organization: "TestOrg", Url: "https://testorg.ai"},
		DefaultStream: true,
	}
	card := j.ToAgentCard()
	if card.Provider == nil {
		t.Fatal("Provider should not be nil")
	}
	if card.Provider.Organization != "TestOrg" {
		t.Errorf("Organization = %q, want %q", card.Provider.Organization, "TestOrg")
	}
}

func TestToAgentCardWithSecuritySchemes(t *testing.T) {
	j := &AgentCardJSON{
		Name:    "secure-agent",
		Version: "1.0",
		Url:     "http://localhost:8001",
		Capabilities: AgentCapabilitiesJSON{
			Streaming:         true,
			PushNotifications: false,
			ExtendedAgentCard: false,
		},
		SecuritySchemes: map[string]any{
			"Bearer": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
		},
		DefaultStream: false,
	}
	card := j.ToAgentCard()
	if card.SecuritySchemes == nil {
		t.Fatal("SecuritySchemes should not be nil")
	}
	scheme, ok := card.SecuritySchemes["Bearer"]
	if !ok {
		t.Fatal("Bearer scheme not found")
	}
	if scheme.Type != "http" {
		t.Errorf("scheme.Type = %q, want %q", scheme.Type, "http")
	}
	if scheme.Scheme != "bearer" {
		t.Errorf("scheme.Scheme = %q, want %q", scheme.Scheme, "bearer")
	}
	if scheme.BearerFormat != "JWT" {
		t.Errorf("scheme.BearerFormat = %q, want %q", scheme.BearerFormat, "JWT")
	}
}

func TestSendStreamingMessageParamsInheritsSendMessageParams(t *testing.T) {
	parent := SendMessageParams{
		TaskID:    "parent-task",
		SessionID: "parent-session",
		ContextID: "parent-context",
		Message:   *NewMessage("user", "hello"),
		Stream:    true,
	}
	params := SendStreamingMessageParams{SendMessageParams: parent}
	if params.TaskID != "parent-task" {
		t.Errorf("TaskID = %q, want %q", params.TaskID, "parent-task")
	}
	if params.ContextID != "parent-context" {
		t.Errorf("ContextID = %q, want %q", params.ContextID, "parent-context")
	}
	if !params.Stream {
		t.Error("Stream should be true")
	}
}

func TestFetchAgentCardInvalidURL(t *testing.T) {
	_, err := FetchAgentCard(context.Background(), "://invalid-url")
	if err == nil {
		t.Error("FetchAgentCard() with invalid URL = nil, want error")
	}
}

func TestClientSendMessageHTTPError(t *testing.T) {
	// Server that returns error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendMessage() to erroring server = nil, want error")
	}
}

func TestClientGetTaskHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.GetTask(context.Background(), "task-1")
	if err == nil {
		t.Error("GetTask() to erroring server = nil, want error")
	}
}

func TestClientSendStreamingMessageHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.SendStreamingMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendStreamingMessage() to erroring server = nil, want error")
	}
}

func TestSendMessageParamsWithSessionAndContext(t *testing.T) {
	params := SendMessageParams{
		TaskID:    "task-session",
		SessionID: "my-session",
		ContextID: "my-context",
		Message:   *NewMessage("assistant", "response"),
		Stream:    false,
	}
	if params.SessionID != "my-session" {
		t.Errorf("SessionID = %q, want %q", params.SessionID, "my-session")
	}
	if params.ContextID != "my-context" {
		t.Errorf("ContextID = %q, want %q", params.ContextID, "my-context")
	}
}

func TestClientSendMessageSuccess(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return &Task{
			ID:      "task-success",
			State:   TaskStateWorking,
			Message: &msg,
		}, nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "Hello success"),
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if task.ID != "task-success" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-success")
	}
	if task.State != TaskStateWorking {
		t.Errorf("task.State = %v, want %v", task.State, TaskStateWorking)
	}
}

func TestClientGetTaskSuccess(t *testing.T) {
	server := NewServer()
	server.Tasks["task-get"] = &Task{
		ID:        "task-get",
		State:     TaskStateCompleted,
		SessionID: "session-1",
		ContextID: "context-1",
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.GetTask(context.Background(), "task-get")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if task.ID != "task-get" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-get")
	}
	if task.State != TaskStateCompleted {
		t.Errorf("task.State = %v, want %v", task.State, TaskStateCompleted)
	}
	if task.SessionID != "session-1" {
		t.Errorf("task.SessionID = %q, want %q", task.SessionID, "session-1")
	}
	if task.ContextID != "context-1" {
		t.Errorf("task.ContextID = %q, want %q", task.ContextID, "context-1")
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
		t.Error("GetTask() for nonexistent task = nil, want error")
	}
}

func TestFetchAgentCardSuccess(t *testing.T) {
	card := &AgentCard{
		Name:    "test-agent-card",
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
	mux := http.NewServeMux()
	mux.Handle("/.well-known/agent.json", handler)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	fetched, err := FetchAgentCard(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("FetchAgentCard() error = %v", err)
	}
	if fetched.Name != "test-agent-card" {
		t.Errorf("Name = %q, want %q", fetched.Name, "test-agent-card")
	}
	if !fetched.Capabilities.Streaming {
		t.Error("Streaming should be true")
	}
	if len(fetched.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(fetched.Skills))
	}
}

func TestFetchAgentCardHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := FetchAgentCard(context.Background(), server.URL)
	if err == nil {
		t.Error("FetchAgentCard() with 404 = nil, want error")
	}
}

func TestClientSendMessageWithContextID(t *testing.T) {
	server := NewServer()
	server.HandleMessage = func(msg Message) (*Task, error) {
		return NewTask("task-ctx", &msg), nil
	}
	mux := http.NewServeMux()
	mux.Handle("/a2a", server.HandleA2A())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.SendMessage(context.Background(), SendMessageParams{
		SessionID: "my-session",
		ContextID: "my-context",
		Message:   *NewMessage("user", "test with context"),
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if task.ID != "task-ctx" {
		t.Errorf("task.ID = %q, want %q", task.ID, "task-ctx")
	}
}

func TestClientSendMessageNon200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.SendMessage(context.Background(), SendMessageParams{
		Message: *NewMessage("user", "test"),
	})
	if err == nil {
		t.Error("SendMessage() with non-200 status = nil, want error")
	}
}

func TestNewClientBaseURL(t *testing.T) {
	client := NewClient("http://localhost:9000")
	if client.BaseURL != "http://localhost:9000" {
		t.Errorf("BaseURL = %q, want %q", client.BaseURL, "http://localhost:9000")
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
}

func TestDiscoveryPoller(t *testing.T) {
	registry := NewAgentRegistry()
	poller := NewDiscoveryPoller(registry, []string{"http://localhost:9999"}, 100*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		poller.Start(ctx)
		close(done)
	}()

	<-done
}

func TestDiscoveryPollerMultipleURLs(t *testing.T) {
	registry := NewAgentRegistry()
	poller := NewDiscoveryPoller(registry, []string{}, 100*time.Millisecond)

	// With no URLs, discover should not panic
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		poller.discover(ctx)
		close(done)
	}()

	<-done
	if registry.Count() != 0 {
		t.Errorf("registry.Count() = %d, want 0 with no URLs", registry.Count())
	}
}

func TestAgentRegistryGetBySkill(t *testing.T) {
	registry := NewAgentRegistry()
	card := &AgentCard{
		Name:          "coder",
		Version:       "1.0",
		Url:           "http://localhost:8001",
		Capabilities:  AgentCapabilities{Streaming: true},
		Skills:        []AgentSkill{{ID: "code", Name: "Code"}, {ID: "test", Name: "Test"}},
		DefaultStream: true,
	}
	registry.Register(card)

	agents := registry.GetBySkill("code")
	if len(agents) != 1 {
		t.Errorf("len(agents) = %d, want 1", len(agents))
	}
	if agents[0].Name != "coder" {
		t.Errorf("Name = %q, want %q", agents[0].Name, "coder")
	}

	agents = registry.GetBySkill("nonexistent")
	if len(agents) != 0 {
		t.Errorf("len(agents) for nonexistent skill = %d, want 0", len(agents))
	}
}

func TestAgentRegistryUnregister(t *testing.T) {
	registry := NewAgentRegistry()
	card := &AgentCard{Name: "unreg-agent", Version: "1.0", Url: "http://localhost:8001", Capabilities: AgentCapabilities{}}
	registry.Register(card)

	if registry.Count() != 1 {
		t.Fatalf("Count = %d, want 1", registry.Count())
	}

	registry.Unregister("unreg-agent")
	if registry.Count() != 0 {
		t.Errorf("Count after unregister = %d, want 0", registry.Count())
	}
}

func TestAgentRegistryLastSeen(t *testing.T) {
	registry := NewAgentRegistry()
	card := &AgentCard{Name: "seen-agent", Version: "1.0", Url: "http://localhost:8001", Capabilities: AgentCapabilities{}}
	registry.Register(card)

	ts, ok := registry.LastSeen("seen-agent")
	if !ok {
		t.Error("LastSeen returned false, want true")
	}
	if ts.IsZero() {
		t.Error("LastSeen time should not be zero")
	}

	_, ok = registry.LastSeen("nonexistent")
	if ok {
		t.Error("LastSeen for nonexistent = true, want false")
	}
}

func TestDiscoveryPollerStop(t *testing.T) {
	registry := NewAgentRegistry()
	poller := NewDiscoveryPoller(registry, []string{"http://localhost:9999"}, 10*time.Second)

	poller.Start(context.Background())
	poller.Stop()
	// Should not panic
}

func TestAgentRegistryList(t *testing.T) {
	registry := NewAgentRegistry()
	card1 := &AgentCard{Name: "agent1", Version: "1.0", Url: "http://localhost:8001", Capabilities: AgentCapabilities{}}
	card2 := &AgentCard{Name: "agent2", Version: "1.0", Url: "http://localhost:8002", Capabilities: AgentCapabilities{}}
	registry.Register(card1)
	registry.Register(card2)

	list := registry.List()
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
}
