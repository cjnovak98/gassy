package a2a

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Server handles A2A requests
type Server struct {
	Tasks                  map[string]*Task
	HandleMessage          func(Message) (*Task, error)
	HandleStreamingMessage func(Message) (<-chan TaskEvent, error)
	WebhookURL            string
}

// TaskEvent represents a streaming task event sent via SSE
type TaskEvent struct {
	TaskID    string     `json:"taskId"`
	SessionID string     `json:"sessionId,omitempty"`
	ContextID string     `json:"contextId,omitempty"`
	Status    *TaskStatus `json:"status,omitempty"`
	Artifact  *Artifact  `json:"artifact,omitempty"`
	TextDelta string     `json:"textDelta,omitempty"`
	Kind      string     `json:"kind"` // "statusUpdate", "artifactUpdate", "textDelta", "done"
}

// NewServer creates a new A2A server
func NewServer() *Server {
	return &Server{
		Tasks: make(map[string]*Task),
	}
}

// HandleTaskSubscribe handles SSE subscription to task updates at /tasks/{id}/subscribe.
func (s *Server) HandleTaskSubscribe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract task ID from URL path /tasks/{id}/subscribe
		path := strings.TrimPrefix(r.URL.Path, "/tasks/")
		path = strings.TrimSuffix(path, "/subscribe")
		taskID := strings.Trim(path, "/")

		if taskID == "" {
			http.Error(w, "task ID required", http.StatusBadRequest)
			return
		}

		task, ok := s.Tasks[taskID]
		if !ok {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Send current task status
		if task.Status != nil {
			event := TaskEvent{
				Kind:   "statusUpdate",
				TaskID: taskID,
				Status: task.Status,
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}

		// For demo, send completion immediately if already done
		if task.State == TaskStateCompleted || task.State == TaskStateFailed {
			doneEvent := TaskEvent{Kind: "done", TaskID: taskID}
			data, _ := json.Marshal(doneEvent)
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}

		// Keep connection open, send updates as they come
		// In a real implementation, use a channel-based approach
		for {
			// Check for updated status periodically
			if task.Status != nil {
				event := TaskEvent{
					Kind:   "statusUpdate",
					TaskID: taskID,
					Status: task.Status,
				}
				data, _ := json.Marshal(event)
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, data)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			if task.State == TaskStateCompleted || task.State == TaskStateFailed {
				break
			}
			// Simple polling - in production this would use channels
		}

		doneEvent := TaskEvent{Kind: "done", TaskID: taskID}
		data, _ := json.Marshal(doneEvent)
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// HandleAgentCard serves the agent card at /.well-known/agent.json
func (s *Server) HandleAgentCard(card *AgentCard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card.ToJSON())
	}
}

// HandleA2A handles incoming A2A JSON-RPC requests
func (s *Server) HandleA2A() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		methodVal := string(req["method"])
		// RawMessage includes JSON quotes, so unmarshal to get clean string
		if methodVal == "" {
			s.sendError(w, -32600, "method missing", nil)
			return
		}
		methodVal = strings.Trim(methodVal, "\"")

		var result interface{}
		var err error

		switch methodVal {
		case "sendMessage":
			result, err = s.handleSendMessage(req, false)
		case "sendStreamingMessage":
			// Check if client wants SSE streaming
			if strings.Contains(r.Header.Get("Accept"), "text/event-stream") && s.HandleStreamingMessage != nil {
				s.handleStreamingMessageSSE(w, r, req)
				return
			}
			result, err = s.handleSendMessage(req, false)
		case "getTask":
			result, err = s.handleGetTask(req)
		case "cancelTask":
			result, err = s.handleCancelTask(req)
		case "listTasks":
			result, err = s.handleListTasks(req)
		case "registerWebhook":
			result, err = s.handleRegisterWebhook(req)
		default:
			s.sendError(w, -32601, "Method not found", nil)
			return
		}

		if err != nil {
			s.sendError(w, 0, err.Error(), nil)
			return
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req["id"],
			Result:  result,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleStreamingMessageSSE handles streaming messages via Server-Sent Events
func (s *Server) handleStreamingMessageSSE(w http.ResponseWriter, r *http.Request, req map[string]json.RawMessage) {
	paramsRaw, ok := req["params"]
	if !ok {
		s.sendError(w, -32600, "params missing", nil)
		return
	}

	var params SendMessageParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		s.sendError(w, -32600, fmt.Sprintf("invalid params: %v", err), nil)
		return
	}

	if s.HandleStreamingMessage == nil {
		// Fall back to non-streaming if no streaming handler configured
		result, err := s.handleSendMessage(req, true)
		if err != nil {
			s.sendError(w, 0, err.Error(), nil)
			return
		}
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req["id"],
			Result:  result,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	events, err := s.HandleStreamingMessage(params.Message)
	if err != nil {
		s.sendError(w, 0, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Flush headers immediately so client sees events as they arrive
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	// Send done event
	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) handleSendMessage(req map[string]json.RawMessage, streaming bool) (interface{}, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return nil, fmt.Errorf("params missing")
	}

	var params SendMessageParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if s.HandleMessage == nil {
		return nil, fmt.Errorf("no message handler configured")
	}

	task, err := s.HandleMessage(params.Message)
	if err != nil {
		return nil, err
	}

	s.Tasks[task.ID] = task

	// For streaming requests without a streaming handler, simulate a stream
	// by sending a single taskStatusUpdate event
	if streaming && s.HandleStreamingMessage == nil {
		return task, nil
	}

	return task, nil
}

func (s *Server) handleGetTask(req map[string]json.RawMessage) (interface{}, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return nil, fmt.Errorf("params missing")
	}

	var params GetTaskParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	task, ok := s.Tasks[params.TaskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", params.TaskID)
	}

	return task, nil
}

func (s *Server) handleCancelTask(req map[string]json.RawMessage) (interface{}, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return nil, fmt.Errorf("params missing")
	}

	var params CancelTaskParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	task, ok := s.Tasks[params.TaskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", params.TaskID)
	}

	task.State = TaskStateCanceled
	return task, nil
}

func (s *Server) handleRegisterWebhook(req map[string]json.RawMessage) (interface{}, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return nil, fmt.Errorf("params missing")
	}

	var params RegisterWebhookParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if params.URL == "" {
		return nil, fmt.Errorf("webhook URL cannot be empty")
	}

	s.WebhookURL = params.URL
	return map[string]string{"status": "registered", "url": params.URL}, nil
}

func (s *Server) handleListTasks(req map[string]json.RawMessage) (interface{}, error) {
	paramsRaw, ok := req["params"]
	if !ok {
		return nil, fmt.Errorf("params missing")
	}

	var params ListTasksParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	var tasks []*Task
	for _, task := range s.Tasks {
		// Filter by contextId if provided
		if params.ContextID != "" && task.ContextID != params.ContextID {
			continue
		}
		// Filter by sessionId if provided
		if params.SessionID != "" && task.SessionID != params.SessionID {
			continue
		}
		tasks = append(tasks, task)
	}

	// Sort for deterministic ordering
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	// Apply maxTasks limit if specified
	if params.MaxTasks > 0 && len(tasks) > params.MaxTasks {
		tasks = tasks[:params.MaxTasks]
	}

	return tasks, nil
}

func (s *Server) sendError(w http.ResponseWriter, code int, msg string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      nil,
		Error: &JSONRPCError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
