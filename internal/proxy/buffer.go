package proxy

import (
	"io"
	"sync"
)

// MessageBuffer provides thread-safe circular buffering for MCP messages
type MessageBuffer struct {
	mu       sync.Mutex
	messages [][]byte
	maxSize  int
}

// NewMessageBuffer creates a new message buffer with the specified maximum size
func NewMessageBuffer(size int) *MessageBuffer {
	return &MessageBuffer{
		messages: make([][]byte, 0, size),
		maxSize:  size,
	}
}

// Add appends a message to the buffer
// If the buffer is full, the oldest message is removed (circular buffer behavior)
func (b *MessageBuffer) Add(message []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.messages) >= b.maxSize {
		// Remove oldest message (circular buffer)
		b.messages = b.messages[1:]
	}

	// Make a copy of the message to avoid issues with slice reuse
	msgCopy := make([]byte, len(message))
	copy(msgCopy, message)
	b.messages = append(b.messages, msgCopy)
}

// Replay writes all buffered messages to the provided writer
// After replaying, the buffer is cleared
func (b *MessageBuffer) Replay(writer io.Writer) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, msg := range b.messages {
		if _, err := writer.Write(msg); err != nil {
			return err
		}
	}

	// Clear the buffer after successful replay
	b.messages = b.messages[:0]
	return nil
}

// Len returns the current number of messages in the buffer
func (b *MessageBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.messages)
}

// Clear removes all messages from the buffer
func (b *MessageBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = b.messages[:0]
}
