package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// version is set via ldflags at build time (e.g. -X main.version=v1.2.3).
var version = "dev"

func main() {
	token := flag.String("token", "", "Agent registration token (required)")
	controlPlane := flag.String("control-plane", "wss://maverick.maniak.io/tunnel", "Control plane WebSocket URL")
	gatewayPath := flag.String("agentgateway-path", "", "Path to agentgateway binary (default: auto-download)")
	configDir := flag.String("config-dir", defaultConfigDir(), "Directory for agentgateway config files")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("maverick-agent", version)
		os.Exit(0)
	}

	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: --token is required")
		flag.Usage()
		os.Exit(1)
	}

	// Ensure config directory exists
	if err := os.MkdirAll(*configDir, 0750); err != nil {
		log.Fatalf("failed to create config dir %s: %v", *configDir, err)
	}

	log.Printf("maverick-agent %s starting", version)
	log.Printf("control plane: %s", *controlPlane)
	log.Printf("config dir: %s", *configDir)

	// Auto-download agentgateway if no explicit path given
	if *gatewayPath == "" {
		gwPath, err := EnsureAgentgateway(*configDir)
		if err != nil {
			log.Printf("warning: failed to auto-download agentgateway: %v", err)
			log.Println("falling back to agentgateway on PATH")
		} else {
			*gatewayPath = gwPath
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize process manager
	proc := NewProcessManager()

	// Initialize tunnel client
	client := &TunnelClient{
		URL:       *controlPlane,
		Token:     *token,
		ConfigDir: *configDir,
		GWPath:    *gatewayPath,
		Proc:      proc,
	}

	// Start tunnel connection (handles reconnection internally)
	go client.Run(ctx)

	// Wait for shutdown signal
	sig := <-sigCh
	log.Printf("received signal %v, shutting down...", sig)
	cancel()

	// Stop agentgateway process
	proc.Stop()

	// Give a moment for cleanup
	time.Sleep(500 * time.Millisecond)
	log.Println("maverick-agent stopped")
}

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".maverick-agent")
	}
	return filepath.Join(home, ".maverick-agent")
}
