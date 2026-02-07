package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/HKUDS/nanobot-go/pkg/cron"
)

// CronTool for scheduling reminders and tasks.
type CronTool struct {
	BaseTool
	Service *cron.Service
	Channel string
	ChatID  string
}

// NewCronTool creates a new CronTool.
func NewCronTool(service *cron.Service) *CronTool {
	return &CronTool{
		Service: service,
	}
}

// SetContext sets the current session context.
func (t *CronTool) SetContext(channel, chatID string) {
	t.Channel = channel
	t.ChatID = chatID
}

func (t *CronTool) Name() string {
	return "cron"
}

func (t *CronTool) Description() string {
	return "Schedule reminders and recurring tasks. Actions: add, list, remove."
}

func (t *CronTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *CronTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"add", "list", "remove"},
				"description": "Action to perform",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Reminder message (for add)",
			},
			"every_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Interval in seconds (for recurring tasks)",
			},
			"run_in_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Run once after N seconds (for one-time tasks)",
			},
			"cron_expr": map[string]interface{}{
				"type":        "string",
				"description": "Cron expression like '0 9 * * *' (for scheduled tasks)",
			},
			"job_id": map[string]interface{}{
				"type":        "string",
				"description": "Job ID (for remove)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *CronTool) Execute(args map[string]interface{}) (string, error) {
	action, ok := args["action"].(string)
	if !ok {
		return "", fmt.Errorf("action must be a string")
	}

	message, _ := args["message"].(string)
	everySeconds, _ := args["every_seconds"].(float64)
	runInSeconds, _ := args["run_in_seconds"].(float64)
	cronExpr, _ := args["cron_expr"].(string)
	jobID, _ := args["job_id"].(string)

	switch action {
	case "add":
		return t.addJob(message, int(everySeconds), int(runInSeconds), cronExpr)
	case "list":
		return t.listJobs()
	case "remove":
		return t.removeJob(jobID)
	default:
		return fmt.Sprintf("Unknown action: %s", action), nil
	}
}

func (t *CronTool) addJob(message string, everySeconds int, runInSeconds int, cronExpr string) (string, error) {
	if message == "" {
		return "Error: message is required for add", nil
	}
	if t.Channel == "" || t.ChatID == "" {
		return "Error: no session context (channel/chat_id)", nil
	}

	var schedule cron.CronSchedule
	deleteAfterRun := false

	if runInSeconds > 0 {
		schedule = cron.CronSchedule{
			Kind: "at",
			AtMs: (time.Now().UnixNano() / int64(time.Millisecond)) + int64(runInSeconds*1000),
		}
		deleteAfterRun = true
	} else if everySeconds > 0 {
		schedule = cron.CronSchedule{Kind: "every", EveryMs: int64(everySeconds * 1000)}
	} else if cronExpr != "" {
		schedule = cron.CronSchedule{Kind: "cron", Expr: cronExpr}
	} else {
		return "Error: either every_seconds, run_in_seconds, or cron_expr is required", nil
	}

	name := message
	if len(name) > 30 {
		name = name[:30]
	}

	job := t.Service.AddJob(name, schedule, message, true, t.Channel, t.ChatID, deleteAfterRun)
	return fmt.Sprintf("Created job '%s' (id: %s)", job.Name, job.ID), nil
}

func (t *CronTool) listJobs() (string, error) {
	jobs := t.Service.ListJobs()
	if len(jobs) == 0 {
		return "No scheduled jobs.", nil
	}

	var sb strings.Builder
	sb.WriteString("Scheduled jobs:\n")
	for _, j := range jobs {
		sb.WriteString(fmt.Sprintf("- %s (id: %s, %s)\n", j.Name, j.ID, j.Schedule.Kind))
	}
	return sb.String(), nil
}

func (t *CronTool) removeJob(jobID string) (string, error) {
	if jobID == "" {
		return "Error: job_id is required for remove", nil
	}
	if t.Service.RemoveJob(jobID) {
		return fmt.Sprintf("Removed job %s", jobID), nil
	}
	return fmt.Sprintf("Job %s not found", jobID), nil
}
