package tools

import (
	"fmt"

	"github.com/HKUDS/nanobot-go/pkg/bus"
)

// MessageTool allows the agent to send messages.
type MessageTool struct {
	BaseTool
	Bus            *bus.MessageBus
	DefaultChannel string
	DefaultChatID  string
}

// NewMessageTool creates a new MessageTool.
func NewMessageTool(messageBus *bus.MessageBus) *MessageTool {
	return &MessageTool{
		Bus: messageBus,
	}
}

// SetContext sets the default channel and chat ID.
func (t *MessageTool) SetContext(channel, chatID string) {
	t.DefaultChannel = channel
	t.DefaultChatID = chatID
}

func (t *MessageTool) Name() string {
	return "message"
}

func (t *MessageTool) Description() string {
	return "Send a message to the user. Supports text, image, audio, and video. Use this to send files or communicate."
}

func (t *MessageTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *MessageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The message content (text body or caption)",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Message type: text, image, audio, video",
				"enum":        []string{"text", "image", "audio", "video"},
			},
			"media": map[string]interface{}{
				"type":        "string",
				"description": "Path or URL to the media file (required for image/audio/video)",
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target channel (telegram, feishu, etc.)",
			},
			"chat_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target chat/user ID",
			},
		},
		"required": []string{},
	}
}

func (t *MessageTool) Execute(args map[string]interface{}) (string, error) {
	content, _ := args["content"].(string)
	msgType, _ := args["type"].(string)
	media, _ := args["media"].(string)

	if msgType == "" {
		msgType = "text"
	}

	if (msgType == "image" || msgType == "audio" || msgType == "video") && media == "" {
		return "", fmt.Errorf("media path/url is required for %s message", msgType)
	}
	
	if msgType == "text" && content == "" {
		return "", fmt.Errorf("content is required for text message")
	}

	channel := t.DefaultChannel
	if c, ok := args["channel"].(string); ok && c != "" {
		channel = c
	}

	chatID := t.DefaultChatID
	if c, ok := args["chat_id"].(string); ok && c != "" {
		chatID = c
	}

	if channel == "" || chatID == "" {
		return "Error: No target channel/chat specified", nil
	}

	if t.Bus == nil {
		return "Error: Message bus not configured", nil
	}

	msg := bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Content: content,
		Type:    bus.MessageType(msgType),
		Media:   media,
	}

	// We publish directly to outbound
	t.Bus.PublishOutbound(msg)

	return fmt.Sprintf("Message (%s) sent to %s:%s", msgType, channel, chatID), nil
}
