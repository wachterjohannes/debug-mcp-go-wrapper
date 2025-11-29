# debug-mcp-go-wrapper - Process Manager & Proxy

## Project Overview

This Go application provides process lifecycle management and transparent proxying for the debug-mcp PHP MCP server. It solves the PHP memory leak problem by periodically restarting the PHP process while maintaining persistent client connections.

**Role in Ecosystem**: Process manager and proxy layer between clients and PHP server

**Key Responsibility**: PHP process lifecycle management with transparent restart capability

## Architecture

### Component Structure

```
main.go (Entry Point)
└── Proxy (Coordinator)
    ├── Process Manager
    │   ├── PHP subprocess control
    │   ├── Stdin/stdout pipes
    │   └── Restart logic
    ├── Message Buffer
    │   ├── Circular buffer
    │   └── Replay mechanism
    └── MCP SDK Integration
        └── StdioTransport
```

### Process Lifecycle

```
[Start]
  ↓
Initialize Proxy
  ↓
Spawn PHP Process
  ↓
┌─────────────────────┐
│  Normal Operation   │←──┐
│  (Proxy messages)   │   │
└─────────────────────┘   │
  ↓ (60s timer)           │
Buffer Incoming Messages  │
  ↓                       │
Stop PHP Process          │
  ↓                       │
Spawn New PHP Process     │
  ↓                       │
Replay Buffered Messages  │
  ↓                       │
└─────────────────────────┘
```

## Key Implementation Patterns

### Process Management (process.go)

**Responsibilities**:
- Start PHP process with proper working directory
- Manage stdin/stdout/stderr pipes
- Detect process exit
- Graceful shutdown with SIGTERM → SIGKILL

**Implementation Pattern**:
```go
type PHPProcess struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
    stderr io.ReadCloser
}

func (p *PHPProcess) Start(workingDir string) error {
    p.cmd = exec.Command("php", "bin/debug-mcp")
    p.cmd.Dir = workingDir

    var err error
    p.stdin, err = p.cmd.StdinPipe()
    if err != nil {
        return err
    }

    p.stdout, err = p.cmd.StdoutPipe()
    if err != nil {
        return err
    }

    p.stderr, err = p.cmd.StderrPipe()
    if err != nil {
        return err
    }

    return p.cmd.Start()
}

func (p *PHPProcess) Stop() error {
    // Send SIGTERM for graceful shutdown
    if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
        return err
    }

    // Wait up to 5 seconds
    done := make(chan error)
    go func() {
        done <- p.cmd.Wait()
    }()

    select {
    case <-time.After(5 * time.Second):
        // Force kill if not stopped
        return p.cmd.Process.Kill()
    case err := <-done:
        return err
    }
}
```

### Message Buffering (buffer.go)

**Responsibilities**:
- Store messages during restart window
- Thread-safe access
- Circular buffer with bounded size
- Message replay to new process

**Implementation Pattern**:
```go
type MessageBuffer struct {
    mu       sync.Mutex
    messages [][]byte
    maxSize  int
}

func NewMessageBuffer(size int) *MessageBuffer {
    return &MessageBuffer{
        messages: make([][]byte, 0, size),
        maxSize:  size,
    }
}

func (b *MessageBuffer) Add(message []byte) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if len(b.messages) >= b.maxSize {
        // Remove oldest message (circular buffer)
        b.messages = b.messages[1:]
    }

    b.messages = append(b.messages, message)
}

func (b *MessageBuffer) Replay(writer io.Writer) error {
    b.mu.Lock()
    defer b.mu.Unlock()

    for _, msg := range b.messages {
        if _, err := writer.Write(msg); err != nil {
            return err
        }
    }

    b.messages = b.messages[:0] // Clear buffer
    return nil
}
```

### Proxy Coordinator (proxy.go)

**Responsibilities**:
- Coordinate PHP process and buffer
- Implement restart timer
- Proxy stdio bidirectionally
- Use go-sdk for MCP protocol

**Implementation Pattern**:
```go
type Proxy struct {
    process    *PHPProcess
    buffer     *MessageBuffer
    restarting bool
    workingDir string
}

func (p *Proxy) Run(ctx context.Context) error {
    // Start PHP process
    if err := p.startPHP(); err != nil {
        return err
    }

    // Setup restart timer
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()

    // Main loop
    for {
        select {
        case <-ticker.C:
            p.restartPHP()
        case <-ctx.Done():
            p.stopPHP()
            return nil
        }
    }
}

func (p *Proxy) restartPHP() error {
    p.restarting = true
    defer func() { p.restarting = false }()

    // Stop old process
    if err := p.process.Stop(); err != nil {
        log.Printf("Error stopping PHP: %v", err)
    }

    // Start new process
    if err := p.startPHP(); err != nil {
        return err
    }

    // Replay buffered messages
    return p.buffer.Replay(p.process.stdin)
}
```

### Main Entry Point (main.go)

**Responsibilities**:
- Parse command-line arguments
- Initialize proxy
- Handle OS signals
- Start proxy execution

**Implementation Pattern**:
```go
func main() {
    // Parse flags
    workingDir := flag.String("cwd", "", "Working directory")
    flag.Parse()

    if *workingDir == "" {
        log.Fatal("--cwd flag is required")
    }

    // Create context with signal handling
    ctx, stop := signal.NotifyContext(
        context.Background(),
        os.Interrupt,
        syscall.SIGTERM,
    )
    defer stop()

    // Initialize proxy
    proxy := NewProxy(*workingDir)

    // Run proxy
    if err := proxy.Run(ctx); err != nil {
        log.Fatalf("Proxy error: %v", err)
    }
}
```

## Go SDK Integration

### Using modelcontextprotocol/go-sdk

The official Go SDK provides:
- MCP protocol compliance
- StdioTransport for stdin/stdout
- Automatic message framing
- JSON-RPC handling

**Integration Pattern**:
```go
import (
    "github.com/modelcontextprotocol/go-sdk/pkg/server"
    "github.com/modelcontextprotocol/go-sdk/pkg/transport"
)

// Use SDK's stdio transport
transport := transport.NewStdioTransport()

// Proxy between client and PHP process
go io.Copy(phpProcess.stdin, transport.Reader())
go io.Copy(transport.Writer(), phpProcess.stdout)
```

## Configuration (config.go)

**Configuration Sources** (priority order):
1. Command-line flags
2. Environment variables
3. Default values

**Configuration Structure**:
```go
type Config struct {
    WorkingDir      string
    PHPBinary       string
    RestartInterval time.Duration
    BufferSize      int
}

func LoadConfig() (*Config, error) {
    cfg := &Config{
        PHPBinary:       getEnv("PHP_BINARY", "php"),
        RestartInterval: 60 * time.Second,
        BufferSize:      100,
    }

    // Parse command-line flags
    flag.StringVar(&cfg.WorkingDir, "cwd", "", "Working directory")
    flag.Parse()

    // Validate required fields
    if cfg.WorkingDir == "" {
        cfg.WorkingDir = os.Getenv("DEBUG_MCP_DIR")
    }

    if cfg.WorkingDir == "" {
        return nil, errors.New("working directory not specified")
    }

    return cfg, nil
}
```

## Error Handling

### Error Recovery Strategies

**PHP Process Crashes**:
```go
func (p *Proxy) monitorProcess() {
    for {
        err := p.process.Wait()
        if err != nil {
            log.Printf("PHP process died: %v", err)
        }

        // Immediate restart on crash
        if err := p.startPHP(); err != nil {
            log.Printf("Failed to restart PHP: %v", err)
            time.Sleep(time.Second) // Backoff
        }
    }
}
```

**Buffer Overflow**:
```go
func (b *MessageBuffer) Add(message []byte) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if len(b.messages) >= b.maxSize {
        log.Printf("WARNING: Buffer full, dropping oldest message")
        b.messages = b.messages[1:]
    }

    b.messages = append(b.messages, message)
}
```

**Restart Failures**:
```go
func (p *Proxy) restartPHP() error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        if err := p.startPHP(); err != nil {
            log.Printf("Restart attempt %d failed: %v", i+1, err)
            time.Sleep(time.Duration(i+1) * time.Second)
            continue
        }
        return nil
    }
    return errors.New("max restart retries exceeded")
}
```

## Signal Handling

### Graceful Shutdown

```go
func main() {
    ctx, stop := signal.NotifyContext(
        context.Background(),
        os.Interrupt,      // Ctrl+C
        syscall.SIGTERM,   // kill command
    )
    defer stop()

    proxy := NewProxy(workingDir)

    // Run proxy until signal received
    if err := proxy.Run(ctx); err != nil {
        log.Fatalf("Proxy error: %v", err)
    }

    log.Println("Shutdown complete")
}
```

## Testing Strategies

### Unit Tests

Test individual components:
```go
func TestMessageBuffer(t *testing.T) {
    buffer := NewMessageBuffer(10)

    // Test add
    buffer.Add([]byte("message1"))
    buffer.Add([]byte("message2"))

    // Test replay
    var buf bytes.Buffer
    if err := buffer.Replay(&buf); err != nil {
        t.Errorf("Replay failed: %v", err)
    }

    if buf.String() != "message1message2" {
        t.Errorf("Unexpected replay: %s", buf.String())
    }
}
```

### Integration Tests

Test with mock PHP process:
```go
func TestProxyRestart(t *testing.T) {
    // Create mock PHP process
    mockPHP := exec.Command("cat") // Echo stdin to stdout

    proxy := &Proxy{
        process: mockPHP,
        buffer:  NewMessageBuffer(100),
    }

    // Test restart
    if err := proxy.restartPHP(); err != nil {
        t.Errorf("Restart failed: %v", err)
    }
}
```

## Performance Considerations

### Memory Usage

- **Go Wrapper**: ~10-20MB (minimal)
- **Message Buffer**: ~1KB per message × 100 = ~100KB
- **Total**: <50MB for wrapper itself

### CPU Usage

- **Idle**: <1% CPU (mostly I/O waiting)
- **During Restart**: Brief spike during process spawn

### Restart Time

- PHP spawn: ~500-800ms
- Buffer replay: ~10-50ms per message
- Total: ~1 second for typical restart

### Optimizations

1. **Preallocate Buffers**: Reduce allocation overhead
2. **Goroutine Pooling**: Limit concurrent operations
3. **Lazy Loading**: Load config only when needed

## Development Workflow

### Setup

```bash
# Clone repository
git clone https://github.com/wachterjohannes/debug-mcp-go-wrapper.git
cd debug-mcp-go-wrapper

# Install dependencies
make install

# Run tests
make test
```

### Build

```bash
# Development build
make build

# Production build (all platforms)
make build-all
```

### Run

```bash
# Direct run
go run cmd/debug-mcp-wrapper/main.go --cwd=/path/to/debug-mcp

# Or use built binary
./debug-mcp-wrapper --cwd=/path/to/debug-mcp
```

## Quick Implementation Checklist

- [ ] `cmd/debug-mcp-wrapper/main.go` - Entry point with signal handling
- [ ] `internal/proxy/proxy.go` - Main proxy coordinator
- [ ] `internal/proxy/process.go` - PHP process management
- [ ] `internal/proxy/buffer.go` - Message buffering
- [ ] `internal/config/config.go` - Configuration loading
- [ ] `go.mod` - Module definition
- [ ] `Makefile` - Build automation
- [ ] `README.md` - User documentation
- [ ] Integration test with debug-mcp
- [ ] Test restart behavior
- [ ] Verify message buffering

## Future Enhancements

1. **Metrics Endpoint**: HTTP endpoint for monitoring
2. **Configurable Restart**: Allow restart interval configuration
3. **Health Checks**: Ping PHP process to detect hangs
4. **Log Rotation**: Automatic log file management
5. **Multiple PHP Processes**: Load balancing across multiple PHP instances

## Repository Information

- **GitHub**: https://github.com/wachterjohannes/debug-mcp-go-wrapper
- **License**: MIT
