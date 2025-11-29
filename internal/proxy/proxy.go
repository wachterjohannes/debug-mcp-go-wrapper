package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// Proxy coordinates the PHP process lifecycle and message proxying
type Proxy struct {
	process        *PHPProcess
	buffer         *MessageBuffer
	restarting     bool
	workingDir     string
	phpBinary      string
	restartInterval time.Duration
}

// NewProxy creates a new Proxy instance
func NewProxy(workingDir, phpBinary string, restartInterval time.Duration, bufferSize int) *Proxy {
	return &Proxy{
		process:         NewPHPProcess(),
		buffer:          NewMessageBuffer(bufferSize),
		workingDir:      workingDir,
		phpBinary:       phpBinary,
		restartInterval: restartInterval,
	}
}

// Run starts the proxy and manages the PHP process lifecycle
func (p *Proxy) Run(ctx context.Context) error {
	// Start initial PHP process
	if err := p.startPHP(); err != nil {
		return fmt.Errorf("failed to start PHP process: %w", err)
	}

	// Setup restart timer
	ticker := time.NewTicker(p.restartInterval)
	defer ticker.Stop()

	// Monitor PHP process for unexpected exits
	go p.monitorProcess(ctx)

	// Start proxying stdio
	go p.proxyStdio(ctx)

	// Main loop
	for {
		select {
		case <-ticker.C:
			log.Printf("Restart timer triggered")
			if err := p.restartPHP(); err != nil {
				log.Printf("Failed to restart PHP: %v", err)
			}
		case <-ctx.Done():
			log.Printf("Shutting down proxy")
			p.stopPHP()
			return nil
		}
	}
}

// startPHP starts a new PHP process
func (p *Proxy) startPHP() error {
	log.Printf("Starting PHP process")
	return p.process.Start(p.workingDir, p.phpBinary)
}

// stopPHP stops the current PHP process
func (p *Proxy) stopPHP() error {
	return p.process.Stop()
}

// restartPHP performs a restart of the PHP process
func (p *Proxy) restartPHP() error {
	p.restarting = true
	defer func() { p.restarting = false }()

	log.Printf("Buffering messages during restart")

	// Stop old process
	if err := p.stopPHP(); err != nil {
		log.Printf("Error stopping PHP: %v", err)
	}

	// Create new process instance
	p.process = NewPHPProcess()

	// Start new process
	if err := p.startPHP(); err != nil {
		return fmt.Errorf("failed to start new PHP process: %w", err)
	}

	// Replay buffered messages
	bufferLen := p.buffer.Len()
	if bufferLen > 0 {
		log.Printf("Replaying %d buffered messages", bufferLen)
		if err := p.buffer.Replay(p.process.Stdin()); err != nil {
			return fmt.Errorf("failed to replay messages: %w", err)
		}
	}

	return nil
}

// proxyStdio handles bidirectional stdio proxying
func (p *Proxy) proxyStdio(ctx context.Context) {
	// Proxy stdin from client to PHP
	go p.proxyStdin(ctx)

	// Proxy stdout from PHP to client
	go p.proxyStdout(ctx)
}

// proxyStdin proxies stdin from the client to the PHP process
func (p *Proxy) proxyStdin(ctx context.Context) {
	reader := bufio.NewReader(os.Stdin)
	buf := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := reader.Read(buf)
			if n > 0 {
				message := buf[:n]

				if p.restarting {
					// Buffer messages during restart
					p.buffer.Add(message)
				} else {
					// Send directly to PHP process
					if _, err := p.process.Stdin().Write(message); err != nil {
						log.Printf("Error writing to PHP stdin: %v", err)
						return
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from stdin: %v", err)
				}
				return
			}
		}
	}
}

// proxyStdout proxies stdout from the PHP process to the client
func (p *Proxy) proxyStdout(ctx context.Context) {
	buf := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := p.process.Stdout().Read(buf)
			if n > 0 {
				if _, err := os.Stdout.Write(buf[:n]); err != nil {
					log.Printf("Error writing to stdout: %v", err)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from PHP stdout: %v", err)
				}
				return
			}
		}
	}
}

// monitorProcess watches for unexpected PHP process exits
func (p *Proxy) monitorProcess(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := p.process.Wait()
			if err != nil && !p.restarting {
				log.Printf("PHP process died unexpectedly: %v", err)
				log.Printf("Attempting immediate restart")

				// Create new process instance
				p.process = NewPHPProcess()

				if err := p.startPHP(); err != nil {
					log.Printf("Failed to restart PHP: %v", err)
					time.Sleep(time.Second) // Backoff before retry
				}
			}
		}
	}
}
