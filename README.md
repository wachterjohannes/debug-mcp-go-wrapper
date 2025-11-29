# debug-mcp-go-wrapper

**⚠️ PROTOTYPE - FOR TESTING AND DISCUSSION PURPOSES ONLY**

---

Go process manager and proxy for the debug-mcp PHP MCP server, providing automatic process restarts with memory isolation.

## Purpose

This Go application acts as a transparent proxy between MCP clients (like Claude Desktop) and the PHP debug-mcp server, providing:
- **Process Management**: Automatic PHP process lifecycle management
- **Memory Isolation**: PHP process gets its own memory space, separate from Go
- **Periodic Restarts**: PHP process restarts every 60 seconds to prevent memory leaks
- **Transparent Proxying**: Clients don't experience disconnections during PHP restarts
- **Message Buffering**: Messages are buffered during restart windows to prevent loss

## Features

- **Persistent Connection**: Maintains continuous connection to MCP clients
- **Automatic Restart**: PHP process restarts every 60 seconds
- **Message Buffering**: In-memory buffer (last 100 messages) prevents message loss
- **Graceful Shutdown**: Handles SIGINT/SIGTERM signals cleanly
- **Error Recovery**: Automatic restart on PHP process crashes
- **Stdio Proxying**: Transparent stdin/stdout proxying using official go-sdk

## Installation

### Build from Source

```bash
make build
```

This creates the `debug-mcp-wrapper` binary in the current directory.

### Build for Multiple Platforms

```bash
make build-all
```

Creates binaries for:
- Linux (amd64)
- macOS (amd64, arm64)
- Windows (amd64)

## Usage

### Basic Usage

```bash
./debug-mcp-wrapper --cwd=/path/to/debug-mcp
```

The wrapper will:
1. Change to the specified working directory
2. Start the PHP MCP server (`php bin/debug-mcp`)
3. Proxy stdin/stdout between client and PHP process
4. Restart PHP every 60 seconds
5. Buffer messages during restart window

### Command-Line Arguments

- `--cwd`: Working directory (where debug-mcp is installed) - **Required**

Example:
```bash
./debug-mcp-wrapper --cwd=/Users/username/projects/my-mcp-server
```

### Environment Variables

- `DEBUG_MCP_DIR`: Alternative to --cwd flag
- `PHP_BINARY`: PHP binary path (default: `php`)

## Claude Desktop Configuration

Add to Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "debug-mcp": {
      "command": "/absolute/path/to/debug-mcp-wrapper",
      "args": ["--cwd", "/absolute/path/to/debug-mcp"]
    }
  }
}
```

**Important**: Use absolute paths, not relative paths or `~`.

## Architecture

```
Claude Desktop
    ↓ stdin/stdout
Go Wrapper (this program)
├─ Main Process (persistent)
│  ├─ MCP Protocol Handling (via go-sdk)
│  ├─ Message Buffer (100 messages)
│  └─ 60-second Restart Timer
│
└─ PHP Process (restarted every 60s)
   └─ debug-mcp Server
```

### Process Flow

1. **Initialization**: Go wrapper starts, spawns PHP process
2. **Normal Operation**: Proxies messages bidirectionally
3. **Restart Trigger**: Every 60 seconds, timer fires
4. **Buffering Phase**: Incoming messages buffered in memory
5. **Process Restart**: Old PHP killed, new PHP spawned (~1s)
6. **Replay Phase**: Buffered messages sent to new PHP
7. **Resume**: Normal operation continues

### Message Buffering

During PHP restart (~1 second):
- Incoming messages stored in circular buffer
- Buffer size: 100 messages (configurable)
- Old messages dropped if buffer full
- Messages replayed to new process after restart
- Client experiences brief latency, no disconnection

## Building

### Prerequisites

- Go 1.21 or higher
- Make (optional, for Makefile usage)

### Build Commands

```bash
# Install dependencies
make install

# Build binary
make build

# Run tests
make test

# Format code
make fmt

# Run linter
make lint

# Clean artifacts
make clean
```

## Development

### Project Structure

```
debug-mcp-go-wrapper/
├── cmd/
│   └── debug-mcp-wrapper/
│       └── main.go              # Entry point
├── internal/
│   ├── proxy/
│   │   ├── proxy.go             # Main proxy coordinator
│   │   ├── process.go           # PHP process management
│   │   └── buffer.go            # Message buffering
│   └── config/
│       └── config.go            # Configuration handling
├── go.mod
├── Makefile
└── README.md
```

### Key Components

**main.go**: Entry point
- Parse command-line arguments
- Initialize proxy
- Handle signals (SIGINT/SIGTERM)
- Start proxy in main goroutine

**proxy.go**: Main coordinator
- Uses official go-sdk for MCP protocol
- Manages PHP subprocess lifecycle
- Implements 60-second restart timer
- Coordinates message buffering
- Proxies stdio between client and PHP

**process.go**: Process management
- Starts PHP process: `php bin/debug-mcp`
- Manages stdin/stdout/stderr pipes
- Detects process exit
- Graceful stop (SIGTERM then SIGKILL)

**buffer.go**: Message buffer
- Thread-safe circular buffer
- Configurable size (default: 100)
- Prevents message loss during restart
- Bounded memory usage

### Implementation Notes

**Go SDK Integration**:
Use `modelcontextprotocol/go-sdk` for MCP protocol handling:
- StdioTransport for stdin/stdout communication
- Automatic JSON-RPC message framing
- Protocol compliance guaranteed by SDK

**Restart Logic**:
```go
ticker := time.NewTicker(60 * time.Second)
for range ticker.C {
    proxy.RestartPHP()
}
```

**Error Handling**:
- PHP crash → Immediate restart
- Repeated crashes → Log error, keep trying
- Go wrapper shutdown → Graceful PHP termination

## Troubleshooting

### PHP Process Won't Start

Check:
- Working directory is correct (`--cwd` points to debug-mcp installation)
- PHP binary is in PATH or set `PHP_BINARY` environment variable
- `bin/debug-mcp` exists and is executable
- composer install has been run in debug-mcp directory

### Messages Lost During Restart

- Buffer size may be too small
- Check logs for buffer overflow warnings
- Increase buffer size in code if needed

### High Memory Usage

- Normal: Go wrapper uses minimal memory
- PHP process memory is isolated and reset every 60s
- If Go memory grows, check for buffer leaks

### Connection Drops

- Should not happen - Go maintains persistent connection
- Check Claude Desktop logs for errors
- Verify wrapper is running (not killed/crashed)

## Monitoring

### Logs

The wrapper logs to stderr:
- PHP process starts/stops
- Restart events
- Buffer statistics
- Error conditions

Example:
```
2024-11-29T15:30:00Z [INFO] Starting PHP process
2024-11-29T15:30:00Z [INFO] PHP process started (PID: 12345)
2024-11-29T15:31:00Z [INFO] Restart timer triggered
2024-11-29T15:31:00Z [INFO] Buffering messages during restart
2024-11-29T15:31:01Z [INFO] PHP process restarted (PID: 12346)
2024-11-29T15:31:01Z [INFO] Replayed 3 buffered messages
```

### Metrics

Monitor these metrics:
- Restart frequency (should be every 60s)
- Buffer usage (should stay below 100 messages)
- Restart duration (should be ~1 second)
- PHP process crashes (should be rare)

## Performance

- **Memory**: Go wrapper ~10-20MB, PHP process ~50-100MB
- **Restart Time**: ~1 second for process spawn and buffer replay
- **Message Latency**: <10ms during normal operation, ~1s during restart
- **CPU**: Minimal, mostly idle waiting for I/O

## Requirements

- Go 1.21 or higher
- PHP 8.1 or higher
- debug-mcp server installed

## Repository

GitHub: https://github.com/wachterjohannes/debug-mcp-go-wrapper

## License

MIT
