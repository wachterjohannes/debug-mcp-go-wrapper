package config

import (
	"errors"
	"flag"
	"os"
	"time"
)

// Config holds the application configuration
type Config struct {
	WorkingDir      string
	PHPBinary       string
	RestartInterval time.Duration
	BufferSize      int
}

// LoadConfig loads configuration from command-line flags and environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		PHPBinary:       getEnv("PHP_BINARY", "php"),
		RestartInterval: 60 * time.Second,
		BufferSize:      100,
	}

	// Parse command-line flags
	flag.StringVar(&cfg.WorkingDir, "cwd", "", "Working directory (where debug-mcp is installed)")
	flag.Parse()

	// Fall back to environment variable if flag not set
	if cfg.WorkingDir == "" {
		cfg.WorkingDir = os.Getenv("DEBUG_MCP_DIR")
	}

	// Validate required fields
	if cfg.WorkingDir == "" {
		return nil, errors.New("working directory not specified (use --cwd flag or DEBUG_MCP_DIR environment variable)")
	}

	// Verify working directory exists
	if _, err := os.Stat(cfg.WorkingDir); os.IsNotExist(err) {
		return nil, errors.New("working directory does not exist: " + cfg.WorkingDir)
	}

	return cfg, nil
}

// getEnv retrieves an environment variable with a default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
