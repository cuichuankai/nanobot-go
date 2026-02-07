package channels

import (
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/bus"
)

// Channel is the interface for chat channels.
type Channel interface {
	Start() error
	Stop() error
	Send(msg bus.OutboundMessage) error
	Name() string
}

// BaseChannel provides common functionality for channels.
type BaseChannel struct {
	Config   interface{}
	Bus      *bus.MessageBus
	AllowFrom []string
}

// IsAllowed checks if a sender is allowed to use this bot.
func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.AllowFrom) == 0 {
		return true
	}

	for _, allowed := range c.AllowFrom {
		if allowed == senderID {
			return true
		}
		// Handle composite IDs like "id|username"
		if strings.Contains(senderID, "|") {
			parts := strings.Split(senderID, "|")
			for _, part := range parts {
				if part == allowed {
					return true
				}
			}
		}
	}
	return false
}

// HandleMessage handles an incoming message from the chat platform.
func (c *BaseChannel) HandleMessage(
	channelName string,
	senderID string,
	chatID string,
	content string,
	media []string,
	metadata map[string]interface{},
) {
	if !c.IsAllowed(senderID) {
		return
	}

	msg := bus.InboundMessage{
		Channel:  channelName,
		SenderID: senderID,
		ChatID:   chatID,
		Content:  content,
		Media:    media,
		Metadata: metadata,
	}

	c.Bus.PublishInbound(msg)
}
