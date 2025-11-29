package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wachterjohannes/debug-mcp-go-wrapper/internal/config"
	"github.com/wachterjohannes/debug-mcp-go-wrapper/internal/proxy"
)

func main() {
	// Setup logging
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[debug-mcp-wrapper] ")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Starting debug-mcp-wrapper")
	log.Printf("Working directory: %s", cfg.WorkingDir)
	log.Printf("PHP binary: %s", cfg.PHPBinary)
	log.Printf("Restart interval: %s", cfg.RestartInterval)
	log.Printf("Buffer size: %d messages", cfg.BufferSize)

	// Create context with signal handling
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,      // Ctrl+C
		syscall.SIGTERM,   // kill command
	)
	defer stop()

	// Initialize proxy
	p := proxy.NewProxy(
		cfg.WorkingDir,
		cfg.PHPBinary,
		cfg.RestartInterval,
		cfg.BufferSize,
	)

	// Run proxy
	if err := p.Run(ctx); err != nil {
		log.Fatalf("Proxy error: %v", err)
	}

	log.Println("Shutdown complete")
}
