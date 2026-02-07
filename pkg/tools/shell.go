package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ExecTool executes shell commands.
type ExecTool struct {
	BaseTool
	Timeout             int
	WorkingDir          string
	RestrictToWorkspace bool
	DenyPatterns        []string
	AllowPatterns       []string
}

// NewExecTool creates a new ExecTool.
func NewExecTool(timeout int, workingDir string, restrict bool) *ExecTool {
	if timeout <= 0 {
		timeout = 60
	}
	return &ExecTool{
		Timeout:             timeout,
		WorkingDir:          workingDir,
		RestrictToWorkspace: restrict,
		DenyPatterns: []string{
			`\brm\s+-[rf]{1,2}\b`,
			`\bdel\s+/[fq]\b`,
			`\brmdir\s+/s\b`,
			`\b(mkfs|diskpart)\b`,
			`\bdd\s+if=`,
			`>\s*/dev/sd`,
			`\b(shutdown|reboot|poweroff)\b`,
			`:\(\)\s*\{.*\};\s*:`,
		},
	}
}

func (t *ExecTool) Name() string {
	return "exec"
}

func (t *ExecTool) Description() string {
	return "Execute a shell command and return its output. Use with caution."
}

func (t *ExecTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"working_dir": map[string]interface{}{
				"type":        "string",
				"description": "Optional working directory for the command",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("command must be a string")
	}

	workingDir := t.WorkingDir
	if wd, ok := args["working_dir"].(string); ok && wd != "" {
		workingDir = wd
	}
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	if err := t.guardCommand(command, workingDir); err != nil {
		return err.Error(), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(t.Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	
	output := stdout.String()
	errOutput := stderr.String()

	var result strings.Builder
	if output != "" {
		result.WriteString(output)
	}
	if errOutput != "" {
		if result.Len() > 0 {
			result.WriteString("\nSTDERR:\n")
		}
		result.WriteString(errOutput)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("Error: Command timed out after %d seconds", t.Timeout), nil
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.WriteString(fmt.Sprintf("\nExit code: %d", exitErr.ExitCode()))
		} else {
			return fmt.Sprintf("Error executing command: %v", err), nil
		}
	}

	resStr := result.String()
	if resStr == "" {
		resStr = "(no output)"
	}

	// Truncate
	maxLen := 10000
	if len(resStr) > maxLen {
		resStr = resStr[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(resStr)-maxLen)
	}

	return resStr, nil
}

func (t *ExecTool) guardCommand(command, cwd string) error {
	cmd := strings.TrimSpace(command)
	lower := strings.ToLower(cmd)

	for _, pattern := range t.DenyPatterns {
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			return fmt.Errorf("Error: Command blocked by safety guard (dangerous pattern detected)")
		}
	}

	if len(t.AllowPatterns) > 0 {
		allowed := false
		for _, pattern := range t.AllowPatterns {
			if matched, _ := regexp.MatchString(pattern, lower); matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("Error: Command blocked by safety guard (not in allowlist)")
		}
	}

	if t.RestrictToWorkspace {
		if strings.Contains(cmd, "..\\") || strings.Contains(cmd, "../") {
			return fmt.Errorf("Error: Command blocked by safety guard (path traversal detected)")
		}

		absCwd, err := filepath.Abs(cwd)
		if err != nil {
			return err
		}

		// Simple check for paths in command - simplistic regex
		// Real path guarding is hard without full shell parsing
		// Here we just check if CWD is safe (assuming CWD is workspace)
		// and simplistic checks for paths
		// For now, let's rely on CWD being set to workspace by AgentLoop
		_ = absCwd
	}

	return nil
}
