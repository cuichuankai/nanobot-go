package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/config"
	"github.com/HKUDS/nanobot-go/pkg/providers"
	"github.com/HKUDS/nanobot-go/pkg/tools"
)

// SubagentManager manages background subagent execution.
type SubagentManager struct {
	Provider      providers.LLMProvider
	Workspace     string
	Bus           *bus.MessageBus
	Model         string
	BraveAPIKey   string
	ExecConfig    *config.ExecToolConfig
	running       map[string]bool // Simplified tracking
}

// NewSubagentManager creates a new SubagentManager.
func NewSubagentManager(
	provider providers.LLMProvider,
	workspace string,
	messageBus *bus.MessageBus,
	model string,
	braveAPIKey string,
	execConfig *config.ExecToolConfig,
) *SubagentManager {
	if model == "" {
		model = provider.GetDefaultModel()
	}
	if execConfig == nil {
		execConfig = &config.ExecToolConfig{Timeout: 60, RestrictToWorkspace: true}
	}
	return &SubagentManager{
		Provider:      provider,
		Workspace:     workspace,
		Bus:           messageBus,
		Model:         model,
		BraveAPIKey:   braveAPIKey,
		ExecConfig:    execConfig,
		running:       make(map[string]bool),
	}
}

// Spawn spawns a subagent to execute a task in the background.
func (m *SubagentManager) Spawn(
	task string,
	label string,
	originChannel string,
	originChatID string,
) string {
	taskID := fmt.Sprintf("%d", time.Now().UnixNano()) // Simple ID
	if label == "" {
		if len(task) > 30 {
			label = task[:30] + "..."
		} else {
			label = task
		}
	}

	m.running[taskID] = true
	go m.runSubagent(taskID, task, label, originChannel, originChatID)

	log.Printf("Spawned subagent [%s]: %s", taskID, label)
	return fmt.Sprintf("Subagent [%s] started (id: %s). I'll notify you when it completes.", label, taskID)
}

func (m *SubagentManager) runSubagent(
	taskID string,
	task string,
	label string,
	originChannel string,
	originChatID string,
) {
	defer delete(m.running, taskID)

	log.Printf("Subagent [%s] starting task: %s", taskID, label)

	// Build subagent tools
	reg := tools.NewRegistry()
	reg.Register(&tools.ReadFileTool{})
	reg.Register(&tools.WriteFileTool{})
	reg.Register(&tools.ListDirTool{})
	reg.Register(&tools.EditFileTool{})
	
	// Add ExecTool
	reg.Register(tools.NewExecTool(m.ExecConfig.Timeout, m.Workspace, m.ExecConfig.RestrictToWorkspace))
	
	// Add Web Tools
	reg.Register(tools.NewWebSearchTool(m.BraveAPIKey, 5))
	reg.Register(tools.NewWebFetchTool(50000))

	systemPrompt := m.buildSubagentPrompt(task)
	messages := []interface{}{
		map[string]interface{}{"role": "system", "content": systemPrompt},
		map[string]interface{}{"role": "user", "content": task},
	}

	maxIterations := 15
	iteration := 0
	var finalResult string

	for iteration < maxIterations {
		iteration++

		ctx := context.Background()
		response, err := m.Provider.Chat(ctx, messages, reg.GetDefinitions(), m.Model)
		if err != nil {
			log.Printf("Subagent [%s] error: %v", taskID, err)
			m.announceResult(taskID, label, task, fmt.Sprintf("Error: %v", err), originChannel, originChatID, "error")
			return
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
			
			// Add assistant message
			msg := map[string]interface{}{
				"role":       "assistant",
				"content":    response.Content,
				"tool_calls": toolCallsRaw,
			}
			messages = append(messages, msg)

			// Execute tools
			for _, tc := range response.ToolCalls {
				log.Printf("Subagent [%s] executing: %s", taskID, tc.Name)
				result, err := reg.Execute(tc.Name, tc.Arguments)
				if err != nil {
					result = fmt.Sprintf("Error executing tool: %v", err)
				}
				
				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": tc.ID,
					"name":         tc.Name,
					"content":      result,
				})
			}
		} else {
			finalResult = response.Content
			break
		}
	}

	if finalResult == "" {
		finalResult = "Task completed but no final response was generated."
	}

	log.Printf("Subagent [%s] completed successfully", taskID)
	m.announceResult(taskID, label, task, finalResult, originChannel, originChatID, "ok")
}

func (m *SubagentManager) announceResult(
	taskID, label, task, result, originChannel, originChatID, status string,
) {
	statusText := "completed successfully"
	if status != "ok" {
		statusText = "failed"
	}

	content := fmt.Sprintf(`[Subagent '%s' %s]

Task: %s

Result:
%s

Summarize this naturally for the user. Keep it brief (1-2 sentences). Do not mention technical details like "subagent" or task IDs.`, label, statusText, task, result)

	msg := bus.InboundMessage{
		Channel:  "system",
		SenderID: "subagent",
		ChatID:   fmt.Sprintf("%s:%s", originChannel, originChatID),
		Content:  content,
	}
	m.Bus.PublishInbound(msg)
}

func (m *SubagentManager) buildSubagentPrompt(task string) string {
	return fmt.Sprintf(`# Subagent

You are a subagent spawned by the main agent to complete a specific task.

## Your Task
%s

## Rules
1. Stay focused - complete only the assigned task, nothing else
2. Your final response will be reported back to the main agent
3. Do not initiate conversations or take on side tasks
4. Be concise but informative in your findings

## What You Can Do
- Read and write files in the workspace
- Execute shell commands
- Search the web and fetch web pages
- Complete the task thoroughly

## What You Cannot Do
- Send messages directly to users (no message tool available)
- Spawn other subagents
- Access the main agent's conversation history

## Workspace
Your workspace is at: %s

When you have completed the task, provide a clear summary of your findings or actions.`, task, m.Workspace)
}
