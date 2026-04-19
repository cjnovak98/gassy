package supervisor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the supervisor daemon",
	Long:  "Start the supervisor daemon binary as a background process",
	RunE:  runSupervisorStart,
}

func init() {
	SupervisorCmd.AddCommand(startCmd)
}

func runSupervisorStart(cmd *cobra.Command, args []string) error {
	// Check if already running
	conn, err := net.Dial("unix", socketPath)
	if err == nil {
		conn.Close()
		fmt.Println("Supervisor is already running")
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	supervisorBin := filepath.Join(filepath.Dir(execPath), "supervisor")
	if _, err := os.Stat(supervisorBin); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		supervisorBin = filepath.Join(cwd, "cmd", "supervisor", "supervisor")
	}

	proc, err := os.StartProcess(supervisorBin, []string{supervisorBin}, &os.ProcAttr{
		Dir:   filepath.Dir(supervisorBin),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return startSupervisorWithGoRun()
	}

	fmt.Printf("Supervisor started with PID %d\n", proc.Pid)
	return nil
}

func startSupervisorWithGoRun() error {
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