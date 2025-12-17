package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Ensure implementations satisfy interface
var (
	_ core.Output = (*TextOutput)(nil)
	_ core.Output = (*JSONOutput)(nil)
	_ core.Output = (*BufferOutput)(nil)
)

// TextOutput writes human-readable messages
type TextOutput struct {
	w io.Writer
}

// NewTextOutput creates output adapter for human-readable text
func NewTextOutput(w io.Writer) *TextOutput {
	return &TextOutput{w: w}
}

func (o *TextOutput) Write(msg core.Message) {
	prefix := ""
	switch msg.Type {
	case core.MsgSuccess:
		prefix = "✓ "
	case core.MsgWarning:
		prefix = "⚠ "
	case core.MsgError:
		prefix = "✗ "
	}
	fmt.Fprintf(o.w, "%s%s\n", prefix, msg.Content)
}

// JSONOutput writes JSON-formatted messages
type JSONOutput struct {
	w   io.Writer
	enc *json.Encoder
}

// NewJSONOutput creates output adapter for JSON
func NewJSONOutput(w io.Writer) *JSONOutput {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return &JSONOutput{w: w, enc: enc}
}

func (o *JSONOutput) Write(msg core.Message) {
	out := map[string]any{
		"type":    msgTypeName(msg.Type),
		"message": msg.Content,
	}
	if msg.Data != nil {
		out["data"] = msg.Data
	}
	o.enc.Encode(out)
}

// BufferOutput collects messages for testing
type BufferOutput struct {
	mu       sync.Mutex
	Messages []core.Message
}

// NewBufferOutput creates output adapter that buffers messages for testing
func NewBufferOutput() *BufferOutput {
	return &BufferOutput{
		Messages: make([]core.Message, 0),
	}
}

func (o *BufferOutput) Write(msg core.Message) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Messages = append(o.Messages, msg)
}

// Last returns the last message or nil
func (o *BufferOutput) Last() *core.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.Messages) == 0 {
		return nil
	}
	return &o.Messages[len(o.Messages)-1]
}

// HasSuccess returns true if any success message was written
func (o *BufferOutput) HasSuccess() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, m := range o.Messages {
		if m.Type == core.MsgSuccess {
			return true
		}
	}
	return false
}

func msgTypeName(t core.MessageType) string {
	switch t {
	case core.MsgInfo:
		return "info"
	case core.MsgSuccess:
		return "success"
	case core.MsgWarning:
		return "warning"
	case core.MsgError:
		return "error"
	default:
		return "unknown"
	}
}
