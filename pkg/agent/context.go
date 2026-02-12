package agent

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/HKUDS/nanobot-go/pkg/memory"
	"github.com/HKUDS/nanobot-go/pkg/skills"
)

// ContextBuilder builds the context for the agent.
type ContextBuilder struct {
	Workspace string
	Memory    *memory.MemoryStore
	Skills    *skills.Loader
}

// NewContextBuilder creates a new ContextBuilder.
func NewContextBuilder(workspace string) *ContextBuilder {
	return &ContextBuilder{
		Workspace: workspace,
		Memory:    memory.NewMemoryStore(workspace),
		Skills:    skills.NewLoader(workspace),
	}
}

var BootstrapFiles = []string{"AGENTS.md", "SOUL.md", "USER.md", "TOOLS.md", "IDENTITY.md"}

// BuildSystemPrompt builds the system prompt.
func (c *ContextBuilder) BuildSystemPrompt() string {
	var parts []string

	parts = append(parts, c.getIdentity())

	bootstrap := c.loadBootstrapFiles()
	if bootstrap != "" {
		parts = append(parts, bootstrap)
	}

	memory := c.Memory.GetMemoryContext()
	if memory != "" {
		parts = append(parts, fmt.Sprintf("# Memory\n\n%s", memory))
	}

	// Always loaded skills
	alwaysSkills := c.Skills.GetAlwaysSkills()
	if len(alwaysSkills) > 0 {
		alwaysContent := c.Skills.LoadSkillsForContext(alwaysSkills)
		if alwaysContent != "" {
			parts = append(parts, fmt.Sprintf("# Active Skills\n\n%s", alwaysContent))
		}
	}

	// Basic skills summary
	skillsSummary := c.Skills.BuildSkillsSummary()
	if skillsSummary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities.
IMPORTANT: These are NOT native tools. You cannot call them directly.
To use a skill, you MUST first read its instruction file using the 'read_file' tool.
Then, follow the instructions in the file to execute the task (usually via 'exec' or 'web_search').

**Guideline**:
1. If a user request matches a skill (e.g., "weather", "summarize"), you **MUST** use the skill.
2. **Do NOT** hallucinate answers or use general knowledge for things like weather, news, or summaries if a skill is available.
3. **Actively execute** the skill instructions (e.g., run the curl command). Do not just tell the user how to do it.

%s`, skillsSummary))
	}

	return strings.Join(parts, "\n\n---\n\n")
}

func (c *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")

	// Ensure workspace path is absolute
	absWorkspace, _ := filepath.Abs(c.Workspace)

	sysInfo := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	return fmt.Sprintf(`# nanobot 🐈

You are nanobot, a helpful AI assistant. You have access to tools that allow you to:
- Read, write, append, and edit files
- Execute shell commands
- Search the web and fetch web pages
- Send messages to users on chat channels
- Spawn subagents for complex background tasks

## Current Time
%s

## Runtime
%s

## Workspace
Your workspace is at: %s
- Memory files: %s/memory/MEMORY.md
- Daily notes: %s/memory/YYYY-MM-DD.md
- Custom skills: %s/skills/{skill-name}/SKILL.md

IMPORTANT: When responding to direct questions or conversations, reply directly with your text response.
Only use the 'message' tool when you need to send a message to a specific chat channel (like WhatsApp).
For normal conversation, just respond with text - do not call the message tool.
Do NOT write content to files unless explicitly requested by the user. If the user asks for long-form content (like an essay, code explanation, or story), stream it directly in your response.

Always be helpful, accurate, and concise. When using tools, explain what you're doing.

## Memory Management
You have a long-term memory file at %s/memory/MEMORY.md.
When the user provides important personal information (e.g., name, location, preferences) or explicitly asks you to remember something, you **MUST** immediately use the 'append_file' tool to save it to this file.
Do not just say "I will remember that" — you must physically write it to the file using the 'append_file' tool.

## Identity & Behavior Management
You have a soul file at %s/SOUL.md.
When the user defines your persona, character, personality, or fundamental behavioral rules (e.g., "You are a virtual girlfriend", "Always answer in French"), you **MUST** save this definition to %s/SOUL.md using the 'write_file' (to overwrite/initialize) or 'append_file' tool.
This ensures you maintain this personality across sessions.

## Conversation Handling
In group chats, user messages may be prefixed with '[Name]:' (e.g., '[Alice]: Hello').
- This indicates the sender's name.
- You should associate this name with the user in your context.
- When replying, address the user by this name to be more personal.
- If you need to remember facts about this specific user, associate them with this name in your memory.`, now, sysInfo, absWorkspace, absWorkspace, absWorkspace, absWorkspace, absWorkspace, absWorkspace, absWorkspace)
}

func (c *ContextBuilder) loadBootstrapFiles() string {
	var parts []string
	for _, filename := range BootstrapFiles {
		path := filepath.Join(c.Workspace, filename)
		if _, err := os.Stat(path); err == nil {
			content, _ := ioutil.ReadFile(path)
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", filename, string(content)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// BuildMessages builds the complete message list for an LLM call.
func (c *ContextBuilder) BuildMessages(
	history []map[string]interface{},
	currentMessage string,
	media []string,
	channel string,
	chatID string,
) []interface{} {
	var messages []interface{}

	systemPrompt := c.BuildSystemPrompt()
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": systemPrompt,
	})

	for _, msg := range history {
		messages = append(messages, msg)
	}

	userContent := c.buildUserContent(currentMessage, media)
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": userContent,
	})

	return messages
}

func (c *ContextBuilder) buildUserContent(text string, media []string) interface{} {
	if len(media) == 0 {
		return text
	}

	var content []map[string]interface{}

	for _, path := range media {
		if _, err := os.Stat(path); err == nil {
			mimeType := mime.TypeByExtension(filepath.Ext(path))
			if strings.HasPrefix(mimeType, "image/") {
				data, _ := ioutil.ReadFile(path)
				b64 := base64.StdEncoding.EncodeToString(data)
				content = append(content, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]interface{}{
						"url": fmt.Sprintf("data:%s;base64,%s", mimeType, b64),
					},
				})
			}
		}
	}

	if len(content) == 0 {
		return text
	}

	content = append(content, map[string]interface{}{
		"type": "text",
		"text": text,
	})

	return content
}

// AddToolResult adds a tool result to the message list.
func (c *ContextBuilder) AddToolResult(
	messages []interface{},
	toolCallID string,
	toolName string,
	result string,
) []interface{} {
	messages = append(messages, map[string]interface{}{
		"role":         "tool",
		"tool_call_id": toolCallID,
		"name":         toolName,
		"content":      result,
	})
	return messages
}

// AddAssistantMessage adds an assistant message to the message list.
func (c *ContextBuilder) AddAssistantMessage(
	messages []interface{},
	content string,
	toolCalls []interface{},
) []interface{} {
	msg := map[string]interface{}{
		"role":    "assistant",
		"content": content,
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	messages = append(messages, msg)
	return messages
}
