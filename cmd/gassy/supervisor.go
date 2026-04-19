package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

const supervisorHTTP = "http://localhost:9091"

var supervisorCmd = &cobra.Command{
	Use:   "supervisor",
	Short: "Supervisor agent management",
	Long:  "Manage agents via the supervisor process",
}

var supervisorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents",
	RunE:  runSupervisorList,
}

var supervisorHireCmd = &cobra.Command{
	Use:   "hire [name] [binary] [port] [skills...]",
	Short: "Hire a new agent",
	Args:  cobra.MinimumNArgs(3),
	RunE:  runSupervisorHire,
}

var supervisorFireCmd = &cobra.Command{
	Use:   "fire [name]",
	Short: "Fire an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runSupervisorFire,
}

var supervisorStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the supervisor process as a background daemon",
	RunE:  runSupervisorStart,
}

func init() {
	supervisorCmd.AddCommand(supervisorListCmd, supervisorHireCmd, supervisorFireCmd, supervisorStartCmd)
	rootCmd.AddCommand(supervisorCmd)
}

func sendHTTPRequest(path string, body map[string]interface{}) ([]map[string]interface{}, error) {
	if body == nil {
		resp, err := http.Get(supervisorHTTP + path)
		if err != nil {
			return nil, fmt.Errorf("connecting to supervisor: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("supervisor returned status %d: %s", resp.StatusCode, string(respBody))
		}

		var result []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		return result, nil
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	resp, err := http.Post(supervisorHTTP+path, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("connecting to supervisor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result, nil
}

func runSupervisorList(cmd *cobra.Command, args []string) error {
	agents, err := sendHTTPRequest("/registry/list", nil)
	if err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents registered")
		return nil
	}

	fmt.Println("Registered Agents:")
	fmt.Println("-------------------")
	for _, a := range agents {
		fmt.Printf("Agent ID: %v\n", a["agent_id"])
		fmt.Printf("Role:     %v\n", a["role"])
		fmt.Printf("A2A URL:  %v\n", a["a2a_url"])
		fmt.Println("-------------------")
	}

	return nil
}

func runSupervisorHire(cmd *cobra.Command, args []string) error {
	name := args[0]
	binary := args[1]
	portInt, err := strconv.Atoi(args[2])
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	skills := args[3:]

	_, err = sendHTTPRequest("/supervisor/hire", map[string]interface{}{
		"name":   name,
		"binary": binary,
		"port":   portInt,
		"skills": skills,
	})
	if err != nil {
		return fmt.Errorf("hire failed: %w", err)
	}

	fmt.Printf("Successfully hired agent: %s\n", name)
	return nil
}

func runSupervisorFire(cmd *cobra.Command, args []string) error {
	name := args[0]

	_, err := sendHTTPRequest("/supervisor/fire", map[string]interface{}{
		"name": name,
	})
	if err != nil {
		return fmt.Errorf("fire failed: %w", err)
	}

	fmt.Printf("Successfully fired agent: %s\n", name)
	return nil
}

func runSupervisorStart(cmd *cobra.Command, args []string) error {
	// Check if already running by trying to connect to HTTP
	resp, err := http.Get(supervisorHTTP + "/health")
	if err == nil {
		resp.Body.Close()
		fmt.Println("Supervisor is already running")
		return nil
	}

	// Find the supervisor binary
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	supervisorBin := filepath.Join(filepath.Dir(execPath), "supervisor")
	if _, err := os.Stat(supervisorBin); os.IsNotExist(err) {
		// Try looking in cmd/supervisor relative to cwd
		cwd, _ := os.Getwd()
		supervisorBin = filepath.Join(cwd, "cmd", "supervisor", "supervisor")
	}

	// Start the supervisor in the background
	proc, err := os.StartProcess(supervisorBin, []string{supervisorBin}, &os.ProcAttr{
		Dir:   filepath.Dir(supervisorBin),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		// If direct start fails, try using `go run`
		return startSupervisorWithGoRun()
	}

	fmt.Printf("Supervisor started with PID %d\n", proc.Pid)
	return nil
}

func startSupervisorWithGoRun() error {
	// Use go run to start supervisor
	cwd, _ := os.Getwd()

	cmd := exec.Command("go", "run", "./cmd/supervisor")
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting supervisor with go run: %w", err)
	}

	fmt.Printf("Supervisor started with PID %d\n", cmd.Process.Pid)
	return nil
}