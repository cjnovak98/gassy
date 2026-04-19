// Package main runs the A2A PoC demo.
// It starts an A2A server, then runs a client that connects to it,
// demonstrating JSON-RPC 2.0 messaging with SSE streaming.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/cjnovak98/gassy/internal/a2a"
)

// A2A Server Implementation (inline for self-contained demo)

type jsonRPCRouter struct {
	server *a2a.Server
}

func newJSONRPCRouter(s *a2a.Server) *jsonRPCRouter {
	return &jsonRPCRouter{server: s}
}

func (r *jsonRPCRouter) serveRPC(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rpcReq map[string]json.RawMessage
	if err := json.NewDecoder(req.Body).Decode(&rpcReq); err != nil {
		r.sendError(w, -32700, "Parse error", nil)
		return
	}

	methodRaw, ok := rpcReq["method"]
	if !ok {
		r.sendError(w, -32600, "method missing", nil)
		return
	}
	method := strings.Trim(string(methodRaw), "\"")

	var result interface{}
	var err error

	switch method {
	case "sendMessage":
		result, err = r.handleSendMessage(rpcReq)
	case "sendStreamingMessage":
		if strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
			r.handleStreamingSSE(rpcReq, w)
			return
		}
		result, err = r.handleSendMessage(rpcReq)
	case "getTask":
		result, err = r.handleGetTask(rpcReq)
	case "cancelTask":
		result, err = r.handleCancelTask(rpcReq)
	case "listTasks":
		result, err = r.handleListTasks(rpcReq)
	default:
		r.sendError(w, -32601, fmt.Sprintf("Method not found: %s", method), nil)
		return
	}

	if err != nil {
		r.sendError(w, 0, err.Error(), nil)
		return
	}

	r.sendResult(w, rpcReq["id"], result)
}

func (r *jsonRPCRouter) handleStreamingSSE(req map[string]json.RawMessage, w http.ResponseWriter) {
	params, err := r.extractParams(req)
	if err != nil {
		r.sendError(w, -32600, err.Error(), nil)
		return
	}

	if r.server.HandleStreamingMessage == nil {
		r.sendError(w, 0, "streaming not supported", nil)
		return
	}

	events, err := r.server.HandleStreamingMessage(params.Message)
	if err != nil {
		r.sendError(w, 0, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for event := range events {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *jsonRPCRouter) handleSendMessage(req map[string]json.RawMessage) (interface{}, error) {
	params, err := r.extractParams(req)
	if err != nil {
		return nil, err
	}

	task, err := r.server.HandleMessage(params.Message)
	if err != nil {
		return nil, err
	}

	r.server.Tasks[task.ID] = task
	return task, nil
}

func (r *jsonRPCRouter) handleGetTask(req map[string]json.RawMessage) (interface{}, error) {
	params, err := r.extractParams(req)
	if err != nil {
		return nil, err
	}

	task, ok := r.server.Tasks[params.TaskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", params.TaskID)
	}

	return task, nil
}

func (r *jsonRPCRouter) handleCancelTask(req map[string]json.RawMessage) (interface{}, error) {
	params, err := r.extractParams(req)
	if err != nil {
		return nil, err
	}

	task, ok := r.server.Tasks[params.TaskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", params.TaskID)
	}

	task.State = a2a.TaskStateCanceled
	task.Status = &a2a.TaskStatus{
		State:     a2a.TaskStateCanceled,
		Timestamp: task.Status.Timestamp,
	}

	return task, nil
}

func (r *jsonRPCRouter) handleListTasks(req map[string]json.RawMessage) (interface{}, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return nil, fmt.Errorf("params missing")
	}

	var params a2a.ListTasksParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return nil, err
	}

	var tasks []*a2a.Task
	for _, task := range r.server.Tasks {
		if params.ContextID != "" && task.ContextID != params.ContextID {
			continue
		}
		if params.SessionID != "" && task.SessionID != params.SessionID {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (r *jsonRPCRouter) extractParams(req map[string]json.RawMessage) (a2a.SendMessageParams, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return a2a.SendMessageParams{}, fmt.Errorf("params missing")
	}

	var params a2a.SendMessageParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return a2a.SendMessageParams{}, fmt.Errorf("invalid params: %w", err)
	}

	return params, nil
}

func (r *jsonRPCRouter) sendResult(w http.ResponseWriter, id json.RawMessage, result interface{}) {
	resp := a2a.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (r *jsonRPCRouter) sendError(w http.ResponseWriter, code int, message string, data interface{}) {
	resp := a2a.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      nil,
		Error: &a2a.JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Agent Card

type agentCard struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	URL             string                 `json:"url"`
	Capabilities    agentCapabilities      `json:"capabilities"`
	Skills          []agentSkill           `json:"skills,omitempty"`
	Provider        *agentProvider         `json:"provider,omitempty"`
	DefaultStream   bool                   `json:"defaultStream"`
}

type agentCapabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
}

type agentSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type agentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url"`
}

func newAgentCard(baseURL string) *agentCard {
	return &agentCard{
		Name:    "poc-agent",
		Version: "1.0.0",
		URL:     baseURL,
		Capabilities: agentCapabilities{
			Streaming:         true,
			PushNotifications: false,
		},
		Skills: []agentSkill{
			{ID: "echo", Name: "Echo", Description: "Echoes back the received message"},
		},
		Provider: &agentProvider{
			Organization: "gassy",
			URL:          "https://gassy.example.com",
		},
		DefaultStream: true,
	}
}

// Demo Handlers

func messageHandler(msg a2a.Message) (*a2a.Task, error) {
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	task := a2a.NewTask(taskID, &msg)
	task.State = a2a.TaskStateCompleted
	task.Status = &a2a.TaskStatus{
		State:     a2a.TaskStateCompleted,
		Timestamp: time.Now(),
	}
	return task, nil
}

func streamingMessageHandler(msg a2a.Message) (<-chan a2a.TaskEvent, error) {
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	events := make(chan a2a.TaskEvent, 10)

	go func() {
		defer close(events)

		// Initial working status
		events <- a2a.TaskEvent{
			Kind:   "statusUpdate",
			TaskID: taskID,
			Status: &a2a.TaskStatus{State: a2a.TaskStateWorking, Timestamp: time.Now()},
		}

		// Extract text from message
		var inputText string
		for _, part := range msg.Parts {
			if tp, ok := part.(a2a.TextPart); ok {
				inputText = tp.Text
				break
			}
		}

		// Stream text deltas
		if inputText != "" {
			response := fmt.Sprintf("Received: %q", inputText)
			for _, ch := range response {
				events <- a2a.TaskEvent{
					Kind:      "textDelta",
					TaskID:    taskID,
					TextDelta: string(ch),
				}
				time.Sleep(10 * time.Millisecond)
			}
		}

		// Completed status
		events <- a2a.TaskEvent{
			Kind:   "statusUpdate",
			TaskID: taskID,
			Status: &a2a.TaskStatus{State: a2a.TaskStateCompleted, Timestamp: time.Now()},
		}
	}()

	return events, nil
}

// Main

func main() {
	ctx := context.Background()
	fmt.Println("=== A2A PoC Demo ===")
	fmt.Println()

	// Create A2A server
	server := a2a.NewServer()
	server.HandleMessage = messageHandler
	server.HandleStreamingMessage = streamingMessageHandler

	// Build HTTP router
	mux := http.NewServeMux()

	router := newJSONRPCRouter(server)
	mux.HandleFunc("/a2a", router.serveRPC)

	card := newAgentCard("http://localhost:8080")
	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	})

	// Use httptest for demo (no port conflicts)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	fmt.Printf("[1] A2A Server started at %s\n", ts.URL)
	fmt.Println()

	// Step 1: Fetch Agent Card
	fmt.Println("[2] Fetching Agent Card (discovery)...")
	card2, err := a2a.FetchAgentCard(ctx, ts.URL)
	if err != nil {
		log.Fatalf("   ERROR: Failed to fetch agent card: %v", err)
	}
	fmt.Printf("    Agent: %s v%s\n", card2.Name, card2.Version)
	fmt.Printf("    Capabilities: streaming=%v\n", card2.Capabilities.Streaming)
	fmt.Println()

	// Step 2: Send a message (non-streaming)
	fmt.Println("[3] Sending message via JSON-RPC 2.0...")
	client := a2a.NewClient(ts.URL)
	task, err := client.SendMessage(ctx, a2a.SendMessageParams{
		Message: *a2a.NewMessage("user", "Hello, agent!"),
	})
	if err != nil {
		log.Fatalf("   ERROR: SendMessage failed: %v", err)
	}
	fmt.Printf("    Task created: ID=%s State=%s\n", task.ID, task.State)
	fmt.Println()

	// Step 3: GetTask (verify task exists)
	fmt.Println("[4] Getting task status (JSON-RPC getTask)...")
	fetched, err := client.GetTask(ctx, task.ID)
	if err != nil {
		log.Fatalf("   ERROR: GetTask failed: %v", err)
	}
	fmt.Printf("    Task: ID=%s State=%s\n", fetched.ID, fetched.State)
	fmt.Println()

	// Step 4: Streaming message (SSE)
	fmt.Println("[5] Sending streaming message (SSE)...")
	streamChan, err := client.SendStreamingMessage(ctx, a2a.SendMessageParams{
		Message: *a2a.NewMessage("user", "Stream this"),
	})
	if err != nil {
		log.Fatalf("   ERROR: SendStreamingMessage failed: %v", err)
	}

	fmt.Println("    SSE Events:")
	var eventCount int
	for event := range streamChan {
		eventCount++
		var te a2a.TaskEvent
		if err := json.Unmarshal([]byte(event.Data), &te); err == nil {
			if te.TextDelta != "" {
				fmt.Printf("      [%d] textDelta: %s\n", eventCount, te.TextDelta)
			}
			if te.Status != nil {
				fmt.Printf("      [%d] statusUpdate: %s\n", eventCount, te.Status.State)
			}
		} else {
			fmt.Printf("      [%d] %s: %s\n", eventCount, event.Event, event.Data)
		}
	}

	fmt.Println()
	fmt.Printf("[6] Demo complete. Total SSE events: %d\n", eventCount)
}
