package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/tools"
)

func (l *AgentLoop) processSystemMessage(msg bus.InboundMessage) error {
	log.Printf("Processing system message from %s", msg.SenderID)

	// Parse origin from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if strings.Contains(msg.ChatID, ":") {
		parts := strings.SplitN(msg.ChatID, ":", 2)
		originChannel = parts[0]
		originChatID = parts[1]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	sessionKey := fmt.Sprintf("%s:%s", originChannel, originChatID)
	sess := l.Sessions.GetOrCreate(sessionKey)

	// Update tool contexts
	if tool, ok := l.Tools.Get("spawn"); ok {
		if spawnTool, ok := tool.(*tools.SpawnTool); ok {
			spawnTool.SetContext(originChannel, originChatID)
		}
	}
	if tool, ok := l.Tools.Get("cron"); ok {
		if cronTool, ok := tool.(*tools.CronTool); ok {
			cronTool.SetContext(originChannel, originChatID)
		}
	}
	if tool, ok := l.Tools.Get("message"); ok {
		if msgTool, ok := tool.(*tools.MessageTool); ok {
			msgTool.SetContext(originChannel, originChatID)
		}
	}

	// Build messages with the announce content
	history := sess.GetHistory(50)
	messages := l.Context.BuildMessages(history, msg.Content, nil, originChannel, originChatID)

	// Agent loop (limited for announce handling)
	iteration := 0
	var finalContent string

	for iteration < l.MaxIterations {
		iteration++

		ctx := context.Background()
		response, err := l.Provider.Chat(ctx, messages, l.Tools.GetDefinitions(), l.Model)
		if err != nil {
			return fmt.Errorf("LLM error: %w", err)
		}

		if response.HasToolCalls() {
			toolCallsRaw := make([]interface{}, len(response.ToolCalls))
			for i, tc := range response.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				toolCallsRaw[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": string(argsJSON),
					},
				}
			}
			messages = l.Context.AddAssistantMessage(messages, response.Content, toolCallsRaw)

			for _, tc := range response.ToolCalls {
				log.Printf("Executing tool: %s", tc.Name)
				result, err := l.Tools.Execute(tc.Name, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf("Error executing tool: %v", err)
				}
				messages = l.Context.AddToolResult(messages, tc.ID, tc.Name, result)
			}
		} else {
			finalContent = response.Content
			break
		}
	}

	if finalContent == "" {
		finalContent = "Background task completed."
	}

	// Save to session (mark as system message in history)
	sess.AddMessage("user", fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content), nil)
	sess.AddMessage("assistant", finalContent, nil)
	l.Sessions.Save(sess)

	l.Bus.PublishOutbound(bus.OutboundMessage{
		Channel: originChannel,
		ChatID:  originChatID,
		Content: finalContent,
	})

	return nil
}
