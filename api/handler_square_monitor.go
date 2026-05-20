package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"nofx/logger"

	"github.com/gin-gonic/gin"
)

// Package-level worker process state (singleton across the server).
var (
	workerCmd *exec.Cmd
	workerMu  sync.Mutex
)

// workerDir returns the absolute path to the square-monitor directory.
func workerDir() string {
	// When running via `go run`, the working directory is the project root.
	dir, _ := os.Getwd()
	return filepath.Join(dir, "scripts", "square-monitor")
}

// isProcessAlive checks whether the process with the given PID is still running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		proc, err := os.FindProcess(pid)
		return err == nil && proc != nil
	}
	// On Unix, find via `kill -0` (signal 0 = existence check only).
	err := exec.Command("kill", "-0", fmt.Sprintf("%d", pid)).Run()
	return err == nil
}

// handleSquareMonitorStatus returns whether the worker is running.
func (s *Server) handleSquareMonitorStatus(c *gin.Context) {
	workerMu.Lock()
	defer workerMu.Unlock()

	running := false
	var pid int

	if workerCmd != nil && workerCmd.Process != nil {
		pid = workerCmd.Process.Pid
		if isProcessAlive(pid) {
			running = true
		} else {
			// Process exited; clean up reference.
			workerCmd = nil
			pid = 0
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"running": running,
		"pid":     pid,
	})
}

// handleSquareMonitorStart launches the square-monitor worker process.
func (s *Server) handleSquareMonitorStart(c *gin.Context) {
	workerMu.Lock()
	defer workerMu.Unlock()

	// Already running?
	if workerCmd != nil && workerCmd.Process != nil && isProcessAlive(workerCmd.Process.Pid) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Worker already running",
			"pid":     workerCmd.Process.Pid,
		})
		return
	}

	dir := workerDir()
	python := filepath.Join(dir, ".venv", "bin", "python")
	if _, err := os.Stat(python); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Square Monitor venv not found. Run: scripts/square-monitor/install.sh"})
		return
	}

	logPath := filepath.Join(dir, "..", "..", ".logs", "square-monitor-worker.log")
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger.Warnf("Failed to open worker log: %v", err)
	}

	cmd := exec.Command(python, "worker.py")
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start worker: " + err.Error()})
		return
	}

	workerCmd = cmd
	pid := cmd.Process.Pid
	logger.Infof("✅ Square Monitor worker started (PID %d)", pid)

	c.JSON(http.StatusOK, gin.H{
		"message": "Worker started",
		"pid":     pid,
	})
}

// handleSquareMonitorStop terminates the square-monitor worker process.
func (s *Server) handleSquareMonitorStop(c *gin.Context) {
	workerMu.Lock()
	defer workerMu.Unlock()

	if workerCmd == nil || workerCmd.Process == nil {
		c.JSON(http.StatusOK, gin.H{"message": "Worker not running"})
		return
	}

	pid := workerCmd.Process.Pid
	if err := workerCmd.Process.Signal(os.Interrupt); err != nil {
		// Try kill if interrupt fails
		if killErr := workerCmd.Process.Kill(); killErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stop worker: " + killErr.Error()})
			return
		}
	}

	// Wait for process to exit (non-blocking in goroutine to avoid deadlock)
	go func() {
		workerCmd.Wait()
		workerMu.Lock()
		workerCmd = nil
		workerMu.Unlock()
	}()

	logger.Infof("⏹️ Square Monitor worker stopped (PID %d)", pid)
	c.JSON(http.StatusOK, gin.H{
		"message": "Worker stopped",
		"pid":     pid,
	})
}
