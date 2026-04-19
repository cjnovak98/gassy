package a2a

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Default timeout for HTTP requests
const DefaultHTTPTimeout = 30 * time.Second

// Client is an A2A client for communicating with agents
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new A2A client with default timeout
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: DefaultHTTPTimeout},
	}
}

// NewClientWithTimeout creates a new A2A client with custom timeout
func NewClientWithTimeout(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// SendMessage sends a message to an agent and returns the task
func (c *Client) SendMessage(ctx context.Context, params SendMessageParams) (*Task, error) {
	req := SendMessageRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "sendMessage",
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/a2a", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d body: %s", resp.StatusCode, string(body))
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("jsonrpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	resultBytes, err := json.Marshal(rpcResp.Result)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}

	var task Task
	if err := json.Unmarshal(resultBytes, &task); err != nil {
		return nil, fmt.Errorf("unmarshaling task: %w", err)
	}

	return &task, nil
}

// SendStreamingMessage sends a message and returns a channel for streaming events.
// It requests SSE from the server and parses server-sent events.
func (c *Client) SendStreamingMessage(ctx context.Context, params SendMessageParams) (<-chan SSEEvent, error) {
	params.Stream = true
	req := SendMessageRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "sendStreamingMessage",
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/a2a", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	eventChan := make(chan SSEEvent)

	go func() {
		defer close(eventChan)
		defer resp.Body.Close()

		// Parse SSE format: "event: <type>\ndata: <json>\n\n"
		var eventType, data string
		scanner := bufio.NewScanner(resp.Body)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			line := scanner.Bytes()
			line = bytes.TrimSuffix(line, []byte{'\n'})
			if len(line) == 0 || line[0] == '\r' {
				// Blank line marks end of event — emit it
				if data != "" {
					event := SSEEvent{Event: eventType, Data: data}
					select {
					case eventChan <- event:
					case <-ctx.Done():
						return
					}
					eventType = ""
					data = ""
				}
				continue
			}

			// Parse SSE line
			if bytes.HasPrefix(line, []byte("event:")) {
				eventType = string(bytes.TrimSpace(line[6:]))
			} else if bytes.HasPrefix(line, []byte("data:")) {
				data = string(bytes.TrimSpace(line[5:]))
			}
		}
	}()

	return eventChan, nil
}

// GetTask retrieves a task by ID
func (c *Client) GetTask(ctx context.Context, taskID string) (*Task, error) {
	req := GetTaskRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "getTask",
		Params: GetTaskParams{
			TaskID: taskID,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/a2a", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("jsonrpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	resultBytes, _ := json.Marshal(rpcResp.Result)
	var task Task
	if err := json.Unmarshal(resultBytes, &task); err != nil {
		return nil, fmt.Errorf("unmarshaling task: %w", err)
	}

	return &task, nil
}

// FetchAgentCard fetches the agent card from a remote agent
func FetchAgentCard(ctx context.Context, baseURL string) (*AgentCard, error) {
	url := baseURL + "/.well-known/agent.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: DefaultHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching agent card: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var card AgentCardJSON
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decoding agent card: %w", err)
	}

	return card.ToAgentCard(), nil
}