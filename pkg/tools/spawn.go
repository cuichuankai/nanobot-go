package tools

import (
	"fmt"
)

// SubagentManagerInterface defines the interface for subagent manager.
type SubagentManagerInterface interface {
	Spawn(task, label, originChannel, originChatID string) string
}

// SpawnTool spawns a subagent.
type SpawnTool struct {
	BaseTool
	Manager       SubagentManagerInterface
	OriginChannel string
	OriginChatID  string
}

// NewSpawnTool creates a new SpawnTool.
func NewSpawnTool(manager SubagentManagerInterface) *SpawnTool {
	return &SpawnTool{
		Manager:       manager,
		OriginChannel: "cli",
		OriginChatID:  "direct",
	}
}

// SetContext sets the origin context for subagent announcements.
func (t *SpawnTool) SetContext(channel, chatID string) {
	t.OriginChannel = channel
	t.OriginChatID = chatID
}

func (t *SpawnTool) Name() string {
	return "spawn"
}

func (t *SpawnTool) Description() string {
	return "Spawn a subagent to handle a task in the background. Use this for complex or time-consuming tasks that can run independently. The subagent will complete the task and report back when done."
}

func (t *SpawnTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *SpawnTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task for the subagent to complete",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional short label for the task (for display)",
			},
		},
		"required": []string{"task"},
	}
}

func (t *SpawnTool) Execute(args map[string]interface{}) (string, error) {
	task, ok := args["task"].(string)
	if !ok {
		return "", fmt.Errorf("task must be a string")
	}
	label, _ := args["label"].(string)

	return t.Manager.Spawn(task, label, t.OriginChannel, t.OriginChatID), nil
}
