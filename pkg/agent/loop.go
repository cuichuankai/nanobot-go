package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/config"
	"github.com/HKUDS/nanobot-go/pkg/cron"
	"github.com/HKUDS/nanobot-go/pkg/providers"
	"github.com/HKUDS/nanobot-go/pkg/session"
	"github.com/HKUDS/nanobot-go/pkg/tools"
)

// AgentLoop is the core processing engine.
type AgentLoop struct {
	Bus           *bus.MessageBus
	Provider      providers.LLMProvider
	Workspace     string
	Model         string
	MaxIterations int
	Config        *config.Config
	CronService   *cron.Service

	Context   *ContextBuilder
	Sessions  *session.Manager
	Tools     *tools.Registry
	Subagents *SubagentManager

	running  bool
	stopChan chan struct{}
}

// NewAgentLoop creates a new AgentLoop.
func NewAgentLoop(
	bus *bus.MessageBus,
	provider providers.LLMProvider,
	workspace string,
	cfg *config.Config,
	cronService *cron.Service,
) *AgentLoop {
	model := cfg.Agents.Defaults.Model
	maxIterations := cfg.Agents.Defaults.MaxToolIterations
	if maxIterations == 0 {
		maxIterations = 20
	}

	loop := &AgentLoop{
		Bus:           bus,
		Provider:      provider,
		Workspace:     workspace,
		Model:         model,
		MaxIterations: maxIterations,
		Config:        cfg,
		CronService:   cronService,
		Context:       NewContextBuilder(workspace),
		Sessions:      session.NewManager(workspace),
		Tools:         tools.NewRegistry(),
		Subagents:     NewSubagentManager(provider, workspace, bus, model, cfg.Tools.Web.Search.APIKey, &cfg.Tools.Exec),
		stopChan:      make(chan struct{}),
	}

	loop.registerDefaultTools()
	return loop
}

func (l *AgentLoop) registerDefaultTools() {
	l.Tools.Register(&tools.ReadFileTool{})
	l.Tools.Register(&tools.WriteFileTool{})
	l.Tools.Register(&tools.AppendFileTool{})
	l.Tools.Register(&tools.EditFileTool{})
	l.Tools.Register(&tools.ListDirTool{})

	// Exec Tool
	l.Tools.Register(tools.NewExecTool(l.Config.Tools.Exec.Timeout, l.Workspace, l.Config.Tools.Exec.RestrictToWorkspace))

	// Web Tools
	l.Tools.Register(tools.NewWebSearchTool(l.Config.Tools.Web.Search.APIKey, 5))
	l.Tools.Register(tools.NewWebFetchTool(50000))

	// Register SpawnTool
	l.Tools.Register(tools.NewSpawnTool(l.Subagents))

	// Register CronTool
	if l.CronService != nil {
		l.Tools.Register(tools.NewCronTool(l.CronService))
	}

	// Register MessageTool
	l.Tools.Register(tools.NewMessageTool(l.Bus))
}

// Run starts the agent loop.
func (l *AgentLoop) Run() {
	l.running = true
	log.Println("Agent loop started")

	inbound := l.Bus.ConsumeInbound()

	for {
		select {
		case msg := <-inbound:
			go func(m bus.InboundMessage) {
				if err := l.processMessage(m); err != nil {
					log.Printf("Error processing message: %v", err)
					l.Bus.PublishOutbound(bus.OutboundMessage{
						Channel: m.Channel,
						ChatID:  m.ChatID,
						Content: fmt.Sprintf("Sorry, I encountered an error: %v", err),
					})
				}
			}(msg)
		case <-l.stopChan:
			l.running = false
			log.Println("Agent loop stopping")
			return
		}
	}
}

// Stop stops the agent loop.
func (l *AgentLoop) Stop() {
	close(l.stopChan)
}

func (l *AgentLoop) processMessage(msg bus.InboundMessage) error {
	// Handle system messages (subagent announces)
	if msg.Channel == "system" {
		return l.processSystemMessage(msg)
	}

	log.Printf("Processing message from %s:%s", msg.Channel, msg.SenderID)

	sessionKey := msg.SessionKey()

	// Handle "New Topic" command
	if strings.TrimSpace(msg.Content) == "新话题" {
		if err := l.Sessions.Clear(sessionKey); err != nil {
			log.Printf("Error clearing session: %v", err)
		}
		l.Bus.PublishOutbound(bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: "已为您开启新话题，之前的对话记录已被清除。",
		})
		return nil
	}

	sess := l.Sessions.GetOrCreate(sessionKey)

	// Update tool contexts
	if tool, ok := l.Tools.Get("spawn"); ok {
		if spawnTool, ok := tool.(*tools.SpawnTool); ok {
			spawnTool.SetContext(msg.Channel, msg.ChatID)
		}
	}
	if tool, ok := l.Tools.Get("cron"); ok {
		if cronTool, ok := tool.(*tools.CronTool); ok {
			cronTool.SetContext(msg.Channel, msg.ChatID)
		}
	}
	if tool, ok := l.Tools.Get("message"); ok {
		if msgTool, ok := tool.(*tools.MessageTool); ok {
			msgTool.SetContext(msg.Channel, msg.ChatID)
		}
	}

	// Build initial messages
	history := sess.GetHistory(50) // Limit history
	messages := l.Context.BuildMessages(history, msg.Content, msg.Media, msg.Channel, msg.ChatID)

	iteration := 0
	var finalContent string

	for iteration < l.MaxIterations {
		iteration++

		// Call LLM with streaming
		ctx := context.Background()
		stream, err := l.Provider.Stream(ctx, messages, l.Tools.GetDefinitions(), l.Model)
		if err != nil {
			return fmt.Errorf("LLM error: %w", err)
		}

		var contentBuilder strings.Builder

		type ToolCallAcc struct {
			ID          string
			Name        string
			ArgsBuilder strings.Builder
		}
		toolCallAccumulator := make(map[int]*ToolCallAcc)

		streamOut := make(chan string, 10)
		messagePublished := false

		for chunk := range stream {
			if chunk.Error != nil {
				log.Printf("Stream error: %v", chunk.Error)
				break
			}

			if chunk.Content != "" {
				if !messagePublished {
					l.Bus.PublishOutbound(bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Stream:  streamOut,
					})
					messagePublished = true
				}
				streamOut <- chunk.Content
				contentBuilder.WriteString(chunk.Content)
			}

			if chunk.ToolCall != nil {
				tc := chunk.ToolCall
				if _, ok := toolCallAccumulator[tc.Index]; !ok {
					toolCallAccumulator[tc.Index] = &ToolCallAcc{}
				}
				acc := toolCallAccumulator[tc.Index]
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Name != "" {
					acc.Name = tc.Name
				}
				if tc.Arguments != "" {
					acc.ArgsBuilder.WriteString(tc.Arguments)
				}
			}
		}

		close(streamOut)
		finalContent = contentBuilder.String()

		// Reconstruct Tool Calls
		var toolCalls []providers.ToolCallRequest
		var indices []int
		for k := range toolCallAccumulator {
			indices = append(indices, k)
		}
		sort.Ints(indices)

		for _, idx := range indices {
			acc := toolCallAccumulator[idx]
			var args map[string]interface{}
			argsStr := acc.ArgsBuilder.String()
			if argsStr != "" {
				if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
					log.Printf("Failed to unmarshal tool arguments: %v", err)
					args = make(map[string]interface{})
				}
			} else {
				args = make(map[string]interface{})
			}

			toolCalls = append(toolCalls, providers.ToolCallRequest{
				ID:        acc.ID,
				Name:      acc.Name,
				Arguments: args,
			})
		}

		if len(toolCalls) > 0 {
			// Add assistant message with tool calls
			toolCallsRaw := make([]interface{}, len(toolCalls))
			for i, tc := range toolCalls {
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
			messages = l.Context.AddAssistantMessage(messages, finalContent, toolCallsRaw)

			// Execute tools
			for _, tc := range toolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				log.Printf("Executing tool: %s with args: %s", tc.Name, string(argsJSON))
				result, err := l.Tools.Execute(tc.Name, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf("Error executing tool: %v", err)
				}
				log.Printf("Tool result: %s", result)
				messages = l.Context.AddToolResult(messages, tc.ID, tc.Name, result)
			}
		} else {
			break
		}
	}

	if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
		if iteration == 1 {
			// If we failed to produce anything in the first iteration, send this fallback
			l.Bus.PublishOutbound(bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: finalContent,
			})
		}
	}

	// Save to session
	sess.AddMessage("user", msg.Content, nil)
	sess.AddMessage("assistant", finalContent, nil)
	l.Sessions.Save(sess)

	return nil
}
