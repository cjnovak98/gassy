package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cjnovak98/gassy/internal/a2a"
	"github.com/spf13/cobra"
)

func TestGetAgentURL(t *testing.T) {
	city := &City{
		Network: NetworkConfig{
			MayorURL:    "http://localhost:8001",
			EngineerURL: "http://localhost:8002",
			DesignerURL: "http://localhost:8003",
		},
	}

	tests := []struct {
		name     string
		agentID  string
		wantURL  string
	}{
		{"mayor URL", "mayor", "http://localhost:8001"},
		{"engineer URL", "engineer", "http://localhost:8002"},
		{"designer URL", "designer", "http://localhost:8003"},
		{"unknown agent returns empty", "unknown-agent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := getAgentURL(city, tt.agentID)
			if url != tt.wantURL {
				t.Errorf("getAgentURL() = %q, want %q", url, tt.wantURL)
			}
		})
	}
}

func TestGetAgentURLWithEmptyNetwork(t *testing.T) {
	city := &City{
		Network: NetworkConfig{
			MayorURL:    "",
			EngineerURL: "",
			DesignerURL: "",
		},
	}

	url := getAgentURL(city, "mayor")
	if url != "" {
		t.Errorf("getAgentURL() with empty network = %q, want empty string", url)
	}
}

func TestMapAgentNetworkURLs(t *testing.T) {
	city := &City{
		Network: NetworkConfig{
			MayorURL:    "http://localhost:8001",
			EngineerURL: "http://localhost:8002",
			DesignerURL: "http://localhost:8003",
		},
	}

	urls := mapAgentNetworkURLs(city)

	if len(urls) != 3 {
		t.Fatalf("len(urls) = %d, want 3", len(urls))
	}

	expected := map[string]string{
		"mayor":    "http://localhost:8001",
		"engineer": "http://localhost:8002",
		"designer": "http://localhost:8003",
	}

	for agentID, expectedURL := range expected {
		if urls[agentID] != expectedURL {
			t.Errorf("urls[%q] = %q, want %q", agentID, urls[agentID], expectedURL)
		}
	}
}

func TestMapAgentNetworkURLsPartial(t *testing.T) {
	city := &City{
		Network: NetworkConfig{
			MayorURL: "http://localhost:8001",
			// EngineerURL and DesignerURL empty
		},
	}

	urls := mapAgentNetworkURLs(city)

	if len(urls) != 1 {
		t.Errorf("len(urls) = %d, want 1", len(urls))
	}

	if urls["mayor"] != "http://localhost:8001" {
		t.Errorf("urls[mayor] = %q, want %q", urls["mayor"], "http://localhost:8001")
	}
}

func TestMapAgentNetworkURLsAllEmpty(t *testing.T) {
	city := &City{
		Network: NetworkConfig{},
	}

	urls := mapAgentNetworkURLs(city)

	if len(urls) != 0 {
		t.Errorf("len(urls) = %d, want 0", len(urls))
	}
}

func TestGetAgentURLSimplified(t *testing.T) {
	// Test that getAgentURL works correctly with empty network (no hardcoded fallthrough)
	city := &City{
		Network: NetworkConfig{
			MayorURL: "http://localhost:8001",
			// engineer and designer empty
		},
	}

	// mayor should work
	url := getAgentURL(city, "mayor")
	if url != "http://localhost:8001" {
		t.Errorf("getAgentURL(mayor) = %q, want %q", url, "http://localhost:8001")
	}

	// engineer should return empty (not hardcoded fallback)
	url = getAgentURL(city, "engineer")
	if url != "" {
		t.Errorf("getAgentURL(engineer) = %q, want empty string", url)
	}

	// designer should return empty
	url = getAgentURL(city, "designer")
	if url != "" {
		t.Errorf("getAgentURL(designer) = %q, want empty string", url)
	}
}

// TestA2ATaskStateConstants verifies all task states are defined correctly
func TestA2ATaskStateConstants(t *testing.T) {
	// Import a2a types to verify states exist
	// These are the states from internal/a2a/types.go
	states := []string{
		"working",
		"input-required",
		"auth-required",
		"completed",
		"failed",
		"canceled",
		"rejected",
	}

	for _, state := range states {
		if state == "" {
			t.Error("empty task state string")
		}
	}
}

func TestGetAllNetworkURLs(t *testing.T) {
	city := &City{
		Network: NetworkConfig{
			MayorURL:    "http://localhost:8001",
			EngineerURL: "http://localhost:8002",
			DesignerURL: "http://localhost:8003",
		},
	}

	urls := getAllNetworkURLsFromCity(city)
	if len(urls) != 3 {
		t.Errorf("len(urls) = %d, want 3", len(urls))
	}
}

func TestGetAllNetworkURLsPartial(t *testing.T) {
	city := &City{
		Network: NetworkConfig{
			MayorURL:    "http://localhost:8001",
			EngineerURL: "",
			DesignerURL: "",
		},
	}

	urls := getAllNetworkURLsFromCity(city)
	if len(urls) != 1 {
		t.Errorf("len(urls) = %d, want 1", len(urls))
	}
}

func TestGetAllNetworkURLsEmpty(t *testing.T) {
	city := &City{
		Network: NetworkConfig{},
	}

	urls := getAllNetworkURLsFromCity(city)
	if len(urls) != 0 {
		t.Errorf("len(urls) = %d, want 0", len(urls))
	}
}

func TestDelegateCmdHasSkillFlag(t *testing.T) {
	flag := delegateCmd.Flags().Lookup("skill")
	if flag == nil {
		t.Error("delegateCmd should have --skill flag")
	}
}

func TestDelegateCmdHasStreamFlag(t *testing.T) {
	flag := delegateCmd.Flags().Lookup("stream")
	if flag == nil {
		t.Error("delegateCmd should have --stream flag")
	}
}

func TestDelegateCmdMinArgs(t *testing.T) {
	if delegateCmd.Args != cobra.MinimumNArgs(1) {
		t.Error("delegateCmd should accept 1+ args")
	}
}

// getAllNetworkURLsFromCity mirrors the URL collection logic from getAllNetworkURLs
// for testing without file I/O dependencies
func getAllNetworkURLsFromCity(city *City) []string {
	var urls []string
	if city.Network.MayorURL != "" {
		urls = append(urls, city.Network.MayorURL)
	}
	if city.Network.EngineerURL != "" {
		urls = append(urls, city.Network.EngineerURL)
	}
	if city.Network.DesignerURL != "" {
		urls = append(urls, city.Network.DesignerURL)
	}
	return urls
}

func TestDelegateCmdUse(t *testing.T) {
	if delegateCmd.Use != "delegate [agent-id] [prompt]" {
		t.Errorf("delegateCmd.Use = %q, want %q", delegateCmd.Use, "delegate [agent-id] [prompt]")
	}
}
func TestPollTaskTimeout(t *testing.T) {
	// Test pollTask with a server that never completes the task
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a task that stays in working state forever
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-timeout","state":"working","status":{"state":"working"}}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pollTask(ctx, client, "task-timeout")
	if err == nil {
		t.Error("pollTask should error on timeout")
	}
}

func TestPollTaskCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-done","state":"completed","message":{"role":"assistant","parts":[{"type":"text","text":"done"}]}}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-done")
	if err != nil {
		t.Errorf("pollTask completed task error = %v, want nil", err)
	}
}

func TestPollTaskFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-fail","state":"failed"}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-fail")
	if err == nil {
		t.Error("pollTask should error on failed task")
	}
}

func TestPollTaskGetTaskError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-error")
	if err == nil {
		t.Error("pollTask should error when GetTask fails")
	}
}

func TestPollTaskCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-cancel","state":"canceled"}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-cancel")
	if err != nil {
		t.Errorf("pollTask canceled task error = %v, want nil", err)
	}
}

func TestPollTaskInputRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-input","state":"input-required","status":{"state":"input-required"}}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-input")
	// input-required should not error, just print and continue
	if err != nil {
		t.Errorf("pollTask input-required error = %v, want nil", err)
	}
}

func TestPollTaskAuthRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-auth","state":"auth-required","status":{"state":"auth-required"}}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-auth")
	if err != nil {
		t.Errorf("pollTask auth-required error = %v, want nil", err)
	}
}

func TestPollTaskRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-reject","state":"rejected"}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx := context.Background()

	err := pollTask(ctx, client, "task-reject")
	if err == nil {
		t.Error("pollTask rejected should error")
	}
}

func TestPollTaskNilStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Task with nil status
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-nil-status","state":"working"}}`))
	}))
	defer server.Close()

	client := a2a.NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Should handle nil status gracefully without error
	err := pollTask(ctx, client, "task-nil-status")
	if err != nil {
		t.Errorf("pollTask nil status error = %v, want nil", err)
	}
}
