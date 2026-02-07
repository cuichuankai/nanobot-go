package bus

import (
	"time"
)

// InboundMessage represents a message received from a chat channel.
type InboundMessage struct {
	Channel   string                 `json:"channel"`
	SenderID  string                 `json:"sender_id"`
	ChatID    string                 `json:"chat_id"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Media     []string               `json:"media"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// SessionKey returns a unique key for session identification.
func (m *InboundMessage) SessionKey() string {
	return m.Channel + ":" + m.ChatID
}

// OutboundMessage represents a message to send to a chat channel.
type OutboundMessage struct {
	Channel  string                 `json:"channel"`
	ChatID   string                 `json:"chat_id"`
	Content  string                 `json:"content"`
	ReplyTo  string                 `json:"reply_to,omitempty"`
	Media    []string               `json:"media"`
	Metadata map[string]interface{} `json:"metadata"`
	Stream   <-chan string          `json:"-"`
}
