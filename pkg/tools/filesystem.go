package tools

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// ReadFileTool reads file contents.
type ReadFileTool struct {
	BaseTool
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file at the given path."
}

func (t *ReadFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *ReadFileTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	expandedPath := expandPath(path)
	data, err := ioutil.ReadFile(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: File not found: %s", path), nil
		}
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied: %s", path), nil
		}
		return "", fmt.Errorf("error reading file: %w", err)
	}

	return string(data), nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct {
	BaseTool
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file at the given path. Creates parent directories if needed."
}

func (t *WriteFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to write to",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *WriteFileTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content must be a string")
	}

	expandedPath := expandPath(path)
	if err := os.MkdirAll(filepath.Dir(expandedPath), 0755); err != nil {
		return "", fmt.Errorf("error creating directories: %w", err)
	}

	if err := ioutil.WriteFile(expandedPath, []byte(content), 0644); err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied: %s", path), nil
		}
		return "", fmt.Errorf("error writing file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// EditFileTool edits a file by replacing text.
type EditFileTool struct {
	BaseTool
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "Edit a file by replacing old_text with new_text. The old_text must exist exactly in the file."
}

func (t *EditFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to edit",
			},
			"old_text": map[string]interface{}{
				"type":        "string",
				"description": "The exact text to find and replace",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace with",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *EditFileTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}
	oldText, ok := args["old_text"].(string)
	if !ok {
		return "", fmt.Errorf("old_text must be a string")
	}
	newText, ok := args["new_text"].(string)
	if !ok {
		return "", fmt.Errorf("new_text must be a string")
	}

	expandedPath := expandPath(path)
	data, err := ioutil.ReadFile(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: File not found: %s", path), nil
		}
		return "", fmt.Errorf("error reading file: %w", err)
	}

	content := string(data)
	if !strings.Contains(content, oldText) {
		return "Error: old_text not found in file. Make sure it matches exactly.", nil
	}

	count := strings.Count(content, oldText)
	if count > 1 {
		return fmt.Sprintf("Warning: old_text appears %d times. Please provide more context to make it unique.", count), nil
	}

	newContent := strings.Replace(content, oldText, newText, 1)
	if err := ioutil.WriteFile(expandedPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("error writing file: %w", err)
	}

	return fmt.Sprintf("Successfully edited %s", path), nil
}

// AppendFileTool appends content to a file.
type AppendFileTool struct {
	BaseTool
}

func (t *AppendFileTool) Name() string {
	return "append_file"
}

func (t *AppendFileTool) Description() string {
	return "Append content to the end of a file. Creates the file if it doesn't exist."
}

func (t *AppendFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to append to",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to append",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *AppendFileTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *AppendFileTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content must be a string")
	}

	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	expandedPath := expandPath(path)
	if err := os.MkdirAll(filepath.Dir(expandedPath), 0755); err != nil {
		return "", fmt.Errorf("error creating directories: %w", err)
	}

	f, err := os.OpenFile(expandedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied: %s", path), nil
		}
		return "", fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("error writing to file: %w", err)
	}

	return fmt.Sprintf("Successfully appended to %s", path), nil
}

// ListDirTool lists directory contents.
type ListDirTool struct {
	BaseTool
}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return "List the contents of a directory."
}

func (t *ListDirTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to list",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *ListDirTool) Execute(args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	expandedPath := expandPath(path)
	files, err := ioutil.ReadDir(expandedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: Directory not found: %s", path), nil
		}
		return "", fmt.Errorf("error listing directory: %w", err)
	}

	var items []string
	for _, file := range files {
		prefix := "📄 "
		if file.IsDir() {
			prefix = "📁 "
		}
		items = append(items, prefix+file.Name())
	}
	sort.Strings(items)

	if len(items) == 0 {
		return fmt.Sprintf("Directory %s is empty", path), nil
	}

	return strings.Join(items, "\n"), nil
}
