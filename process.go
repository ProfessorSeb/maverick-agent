package main

import (
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ProcessManager manages the agentgateway subprocess lifecycle.
type ProcessManager struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

// NewProcessManager creates a new process manager.
func NewProcessManager() *ProcessManager {
	return &ProcessManager{}
}

// Start launches the agentgateway binary with the given config path.
func (p *ProcessManager) Start(binaryPath, configPath string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil && p.cmd.Process != nil {
		log.Println("process: agentgateway already running, stop first")
		return
	}

	cmd := exec.Command(binaryPath, "-f", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("process: failed to start agentgateway: %v", err)
		return
	}

	p.cmd = cmd
	log.Printf("process: agentgateway started (pid %d)", cmd.Process.Pid)

	// Wait for process in background
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("process: agentgateway exited: %v", err)
		} else {
			log.Println("process: agentgateway exited cleanly")
		}
		p.mu.Lock()
		if p.cmd == cmd {
			p.cmd = nil
		}
		p.mu.Unlock()
	}()
}

// Stop gracefully shuts down the agentgateway process.
func (p *ProcessManager) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return
	}

	log.Println("process: stopping agentgateway...")

	// Send SIGTERM
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Printf("process: failed to send SIGTERM: %v", err)
		p.cmd.Process.Kill()
		return
	}

	// Wait up to 5 seconds for graceful shutdown
	done := make(chan struct{})
	go func() {
		p.cmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("process: agentgateway stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("process: agentgateway did not stop in time, sending SIGKILL")
		p.cmd.Process.Kill()
	}

	p.cmd = nil
}

// Restart stops and starts the agentgateway process.
func (p *ProcessManager) Restart(binaryPath, configPath string) {
	p.Stop()
	p.Start(binaryPath, configPath)
}

// IsRunning returns whether the agentgateway process is currently running.
func (p *ProcessManager) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.cmd.Process != nil
}
