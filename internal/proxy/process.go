package proxy

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"syscall"
	"time"
)

// PHPProcess manages the lifecycle of a PHP debug-mcp server process
type PHPProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

// NewPHPProcess creates a new PHPProcess instance
func NewPHPProcess() *PHPProcess {
	return &PHPProcess{}
}

// Start launches the PHP process with the specified working directory and PHP binary
func (p *PHPProcess) Start(workingDir, phpBinary string) error {
	// Create command
	p.cmd = exec.Command(phpBinary, "bin/debug-mcp")
	p.cmd.Dir = workingDir

	// Setup stdin pipe
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Setup stdout pipe
	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Setup stderr pipe
	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start PHP process: %w", err)
	}

	log.Printf("PHP process started (PID: %d)", p.cmd.Process.Pid)

	// Start stderr logger
	go p.logStderr()

	return nil
}

// Stop terminates the PHP process gracefully
// First attempts SIGTERM, then SIGKILL after timeout
func (p *PHPProcess) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	pid := p.cmd.Process.Pid
	log.Printf("Stopping PHP process (PID: %d)", pid)

	// Send SIGTERM for graceful shutdown
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Printf("Failed to send SIGTERM: %v", err)
		return p.cmd.Process.Kill()
	}

	// Wait up to 5 seconds for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case <-time.After(5 * time.Second):
		// Timeout - force kill
		log.Printf("PHP process did not stop gracefully, sending SIGKILL")
		if err := p.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		<-done // Wait for process to fully exit
		return nil
	case err := <-done:
		if err != nil {
			log.Printf("PHP process exited with error: %v", err)
		} else {
			log.Printf("PHP process stopped gracefully")
		}
		return nil
	}
}

// Stdin returns the stdin writer for the PHP process
func (p *PHPProcess) Stdin() io.WriteCloser {
	return p.stdin
}

// Stdout returns the stdout reader for the PHP process
func (p *PHPProcess) Stdout() io.ReadCloser {
	return p.stdout
}

// Wait waits for the process to exit and returns any error
func (p *PHPProcess) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

// logStderr continuously reads and logs stderr from the PHP process
func (p *PHPProcess) logStderr() {
	buf := make([]byte, 4096)
	for {
		n, err := p.stderr.Read(buf)
		if n > 0 {
			log.Printf("[PHP stderr] %s", string(buf[:n]))
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading PHP stderr: %v", err)
			}
			return
		}
	}
}
