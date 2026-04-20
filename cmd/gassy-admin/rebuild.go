package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cjnovak98/gassy/internal/city"
	"github.com/spf13/cobra"
)

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the agent image and restart all containers",
	Long:  "Rebuild the Docker/podman image from the Dockerfile and restart all gassy containers",
	Args:  cobra.NoArgs,
	RunE:  runRebuild,
}

func init() {
	rootCmd.AddCommand(rebuildCmd)
}

func runRebuild(cmd *cobra.Command, args []string) error {
	// Get the directory containing the gassy repo (parent of gassy-admin's working dir structure)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Build from gassy/ root where the Makefile and agent/ Dockerfile live
	// gassy-admin is installed to GOPATH/bin, so executable is in cmd/gassy-admin/
	// The Makefile is in gassy/ which is two levels up from cmd/gassy-admin/
	execPath, err := os.Executable()
	if err == nil {
		// gassy-admin is installed to GOBIN, so gassy/ is the parent of its parent dir
		// GOPATH/bin/gassy-admin -> GOPATH/src/.../cmd/gassy-admin/main.go
		// We need to find gassy/ which contains agent/Dockerfile and Makefile
		gassyRoot := findGassyRoot(filepath.Dir(filepath.Dir(execPath)))
		if gassyRoot != "" {
			cwd = gassyRoot
		}
	}

	// Check if Dockerfile exists
	dockerfile := fmt.Sprintf("%s/agent/Dockerfile", cwd)
	if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
		return fmt.Errorf("Dockerfile not found at %s", dockerfile)
	}

	fmt.Println("Building agent image...")
	if err := buildAgentImage(dockerfile, cwd); err != nil {
		return fmt.Errorf("building agent image: %w", err)
	}
	fmt.Println("Image built successfully")

	// Get list of gassy containers to restart
	containers, err := getGassyContainers()
	if err != nil {
		return fmt.Errorf("getting gassy containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No gassy containers running to restart")
		return nil
	}

	// Stop containers
	fmt.Println("Stopping containers...")
	for _, c := range containers {
		if err := stopContainer(c); err != nil {
			fmt.Printf("Warning: could not stop %s: %v\n", c, err)
		}
	}

	// Start containers
	fmt.Println("Starting containers...")
	if err := startAllContainers(); err != nil {
		return fmt.Errorf("starting containers: %w", err)
	}

	fmt.Println("Rebuild complete!")
	return nil
}

func buildAgentImage(dockerfile, context string) error {
	args := []string{
		"build",
		"-t", "localhost:5000/gassy/agent:latest",
		"-f", dockerfile,
		context,
	}
	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findGassyRoot searches upward for the gassy/ root containing agent/Dockerfile
func findGassyRoot(start string) string {
	dir := start
	for {
		// Check if we found the gassy root by looking for agent/Dockerfile
		dockerfile := filepath.Join(dir, "agent", "Dockerfile")
		if _, err := os.Stat(dockerfile); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	// If we couldn't find it via search, try common locations
	commonPaths := []string{
		"/home/cnovak/gassy",
		"/var/home/cnovak/gassy",
		filepath.Join(os.Getenv("HOME"), "gassy"),
	}
	for _, p := range commonPaths {
		dockerfile := filepath.Join(p, "agent", "Dockerfile")
		if _, err := os.Stat(dockerfile); err == nil {
			return p
		}
	}
	return ""
}

func getGassyContainers() ([]string, error) {
	cmd := exec.Command("podman", "ps", "--filter", "label=gassy", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing gassy containers: %w", err)
	}

	containers := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Filter out empty strings
	result := make([]string, 0, len(containers))
	for _, c := range containers {
		c = strings.TrimSpace(c)
		if c != "" {
			result = append(result, c)
		}
	}
	return result, nil
}

func startAllContainers() error {
	// Re-use the start logic from start.go
	c, err := city.ParseFile(cityFile)
	if err != nil {
		return fmt.Errorf("parsing city config: %w", err)
	}

	supervisorPort := "9091"

	// Start supervisor first
	if err := startSupervisor(); err != nil {
		return fmt.Errorf("starting supervisor: %w", err)
	}
	fmt.Println("Started supervisor container")

	// Start agent containers
	for _, agent := range c.Agents {
		if err := startAgentContainer(agent, supervisorPort); err != nil {
			return fmt.Errorf("starting agent %s: %w", agent.ID, err)
		}
		fmt.Printf("Started agent container: %s\n", agent.ID)
	}

	return nil
}
