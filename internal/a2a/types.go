package a2a

import "time"

// TaskState represents the state of an A2A task
type TaskState string

const (
	TaskStateWorking   TaskState = "working"
	TaskStateInputReq  TaskState = "input-required"
	TaskStateAuthReq   TaskState = "auth-required"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCanceled  TaskState = "canceled"
	TaskStateRejected  TaskState = "rejected"
)

// Task represents an A2A task
type Task struct {
	ID        string      `json:"id"`
	State     TaskState   `json:"state"`
	SessionID string      `json:"sessionId,omitempty"`
	ContextID string      `json:"contextId,omitempty"`
	Message   *Message    `json:"message,omitempty"`
	Artifacts []Artifact  `json:"artifacts,omitempty"`
	History   []Event     `json:"history,omitempty"`
	Status    *TaskStatus `json:"status,omitempty"`
}

// TaskStatus represents task status information
type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// Message represents an A2A message
type Message struct {
	ID        string    `json:"id,omitempty"`
	Role      string    `json:"role,omitempty"`
	Parts     []Part    `json:"parts"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// Part represents a part of a message
type Part interface{}

// TextPart represents a text part
type TextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// DataPart represents a data part
type DataPart struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// Artifact represents a task artifact
type Artifact struct {
	Resource *ArtifactResource `json:"resource,omitempty"`
	Parts    []Part            `json:"parts,omitempty"`
}

// ArtifactResource represents an artifact resource
type ArtifactResource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
}

// Event represents a task event
type Event struct {
	Kind      string      `json:"kind"`
	Timestamp time.Time   `json:"timestamp,omitempty"`
	Actor     string      `json:"actor,omitempty"`
	Content   interface{} `json:"content,omitempty"`
}

// AgentProvider represents agent provider info
type AgentProvider struct {
	Organization string `json:"organization"`
	Url          string `json:"url"`
}

// AgentCapabilities represents agent capabilities
type AgentCapabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
	ExtendedAgentCard bool `json:"extendedAgentCard"`
}

// AgentSkill represents an agent skill
type AgentSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SecurityScheme represents a security scheme
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

// AgentCard represents the agent metadata endpoint
type AgentCard struct {
	Name            string                    `json:"name"`
	Version         string                    `json:"version"`
	Url             string                    `json:"url"`
	Capabilities    AgentCapabilities         `json:"capabilities"`
	Skills          []AgentSkill              `json:"skills,omitempty"`
	Provider        *AgentProvider            `json:"provider,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
	DefaultStream   bool                      `json:"defaultStream"`
}

// SendMessageRequest is a JSON-RPC request to send a message
type SendMessageRequest struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      interface{}       `json:"id"`
	Method  string            `json:"method"`
	Params  SendMessageParams `json:"params"`
}

// SendMessageParams contains params for SendMessage
type SendMessageParams struct {
	TaskID    string  `json:"taskId,omitempty"`
	SessionID string  `json:"sessionId,omitempty"`
	ContextID string  `json:"contextId,omitempty"`
	Message   Message `json:"message"`
	Stream    bool    `json:"stream,omitempty"`
}

// SendStreamingMessageParams contains params for SendStreamingMessage
type SendStreamingMessageParams struct {
	SendMessageParams
}

// GetTaskRequest is a JSON-RPC request to get a task
type GetTaskRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Method  string        `json:"method"`
	Params  GetTaskParams `json:"params"`
}

// GetTaskParams contains params for GetTask
type GetTaskParams struct {
	TaskID string `json:"taskId"`
}

// ListTasksParams contains params for ListTasks
type ListTasksParams struct {
	SessionID string `json:"sessionId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	MaxTasks  int    `json:"maxTasks,omitempty"`
}

// CancelTaskParams contains params for CancelTask
type CancelTaskParams struct {
	TaskID string `json:"taskId"`
}

// RegisterWebhookParams contains params for RegisterWebhook
type RegisterWebhookParams struct {
	URL string `json:"url"`
}

// JSONRPCError represents a JSON-RPC error
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// JSONRPCResponse is a generic JSON-RPC response
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

// TaskStatusUpdateEvent represents a task status update
type TaskStatusUpdateEvent struct {
	TaskID string     `json:"taskId"`
	Status TaskStatus `json:"status"`
}

// TaskArtifactUpdateEvent represents a task artifact update
type TaskArtifactUpdateEvent struct {
	TaskID   string   `json:"taskId"`
	Artifact Artifact `json:"artifact"`
}
