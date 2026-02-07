package tools

import "fmt"

// Tool represents an agent tool.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(args map[string]interface{}) (string, error)
	ToSchema() map[string]interface{}
}

// BaseTool provides common functionality for tools.
type BaseTool struct{}

// GenerateSchema converts the tool to OpenAI function schema format.
func GenerateSchema(tool Tool) map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		},
	}
}

// Registry manages the available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// Execute executes a tool by name with arguments.
func (r *Registry) Execute(name string, args map[string]interface{}) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	return tool.Execute(args)
}

// GetDefinitions returns the schema definitions for all registered tools.
func (r *Registry) GetDefinitions() []interface{} {
	defs := make([]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		defs = append(defs, tool.ToSchema())
	}
	return defs
}
