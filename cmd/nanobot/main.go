package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/agent"
	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/channels"
	"github.com/HKUDS/nanobot-go/pkg/config"
	"github.com/HKUDS/nanobot-go/pkg/cron"
	"github.com/HKUDS/nanobot-go/pkg/providers"
	"github.com/HKUDS/nanobot-go/pkg/utils"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: nanobot <command> [args]")
		fmt.Println("Commands: agent, onboard, gateway")
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "agent":
		runAgent(os.Args[2:])
	case "onboard":
		runOnboard()
	case "gateway":
		fmt.Println("Gateway not implemented yet")
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func runAgent(args []string) {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	message := fs.String("m", "", "Message to send")
	configPath := fs.String("c", "", "Path to config file")
	fs.Parse(args)

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	workspace := expandPath(cfg.Agents.Defaults.Workspace)
	logDir := filepath.Join(workspace, "logs")
	utils.SetupLogger(logDir)

	// Initialize components
	messageBus := bus.NewMessageBus()

	// Initialize Cron
	cronStorePath := filepath.Join(workspace, "cron.json")
	cronService := cron.NewService(cronStorePath, func(job cron.CronJob) {
		content := job.Payload.Message
		if job.Payload.Kind == "agent_turn" {
			// Inject message to bus to trigger agent
			// We use "cron" as channel and job.Payload.Channel/To as origin if available
			channel := "cron"
			chatID := job.ID

			if job.Payload.Channel != "" {
				channel = job.Payload.Channel
			}
			if job.Payload.To != "" {
				chatID = job.Payload.To
			}

			// If it's an agent turn, we want the agent to process it.
			// We can send it as a system message or user message.
			// Ideally, we treat it as an inbound message.
			messageBus.PublishInbound(bus.InboundMessage{
				Channel:  channel,
				SenderID: "cron",
				ChatID:   chatID,
				Content:  content,
			})
		}
	})
	cronService.Start()
	defer cronService.Stop()

	// Initialize Channels
	// Telegram
	if cfg.Channels.Telegram.Enabled {
		tgChannel := channels.NewTelegramChannel(&cfg.Channels.Telegram, messageBus)
		if err := tgChannel.Start(); err != nil {
			fmt.Printf("Error starting Telegram channel: %v\n", err)
		} else {
			messageBus.SubscribeOutbound(tgChannel.Name(), func(msg bus.OutboundMessage) {
				if err := tgChannel.Send(msg); err != nil {
					fmt.Printf("Error sending to Telegram: %v\n", err)
				}
			})
		}
	}

	// Feishu
	if cfg.Channels.Feishu.Enabled {
		feishuChannel := channels.NewFeishuChannel(&cfg.Channels.Feishu, messageBus)
		if err := feishuChannel.Start(); err != nil {
			fmt.Printf("Error starting Feishu channel: %v\n", err)
		} else {
			messageBus.SubscribeOutbound(feishuChannel.Name(), func(msg bus.OutboundMessage) {
				if err := feishuChannel.Send(msg); err != nil {
					fmt.Printf("Error sending to Feishu: %v\n", err)
				}
			})
		}
	}

	// Select provider
	provider, err := providers.NewProvider(cfg)
	if err != nil {
		fmt.Printf("Error initializing provider: %v\n", err)
		fmt.Println("Please run 'nanobot onboard' or edit ~/.nanobot/config.json")
		os.Exit(1)
	}

	loop := agent.NewAgentLoop(messageBus, provider, workspace, cfg, cronService)

	go messageBus.DispatchOutbound()
	go loop.Run()

	if *message != "" {
		messageBus.PublishInbound(bus.InboundMessage{
			Channel:  "cli",
			SenderID: "user",
			ChatID:   "direct",
			Content:  *message,
		})

		// Wait for response
		done := make(chan struct{})
		messageBus.SubscribeOutbound("cli", func(msg bus.OutboundMessage) {
			if msg.Stream != nil {
				for chunk := range msg.Stream {
					fmt.Print(chunk)
				}
				fmt.Println()
			} else {
				fmt.Println(msg.Content)
			}
			close(done)
		})

		<-done
		loop.Stop()
	} else {
		// Server mode
		fmt.Println("Agent running in server mode. Press Ctrl+C to stop.")
		select {}
	}
}

func runOnboard() {
	configDir := ".nanobot"
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	configFile := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		// Expand workspace for default config
		if abs, err := filepath.Abs(filepath.Join(configDir, "workspace")); err == nil {
			cfg.Agents.Defaults.Workspace = abs
		} else {
			cfg.Agents.Defaults.Workspace = filepath.Join(configDir, "workspace")
		}

		file, err := os.Create(configFile)
		if err != nil {
			fmt.Printf("Warning: Could not create config file: %v\n", err)
		} else {
			defer file.Close()
			// Pretty print JSON
			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(cfg); err != nil {
				fmt.Printf("Error writing config file: %v\n", err)
			}
			fmt.Printf("Created config file at %s\n", configFile)
		}
	} else {
		fmt.Printf("Config file already exists at %s\n", configFile)
	}

	// Create workspace
	workspace := filepath.Join(configDir, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		fmt.Printf("Error creating workspace: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created workspace at %s\n", workspace)

	// Create memory dir
	memoryDir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		fmt.Printf("Error creating memory directory: %v\n", err)
	}

	// Create skills dir
	skillsDir := filepath.Join(workspace, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil && !os.IsExist(err) {
		fmt.Printf("Error creating skills directory: %v\n", err)
	}

	// Create a README in skills dir if not exists
	readmePath := filepath.Join(skillsDir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		readmeContent := `# Skills

Add your skills here. Each skill should be in its own directory with a ` + "`SKILL.md`" + ` file.

Example structure:
` + "```" + `
skills/
  weather/
    SKILL.md
  github/
    SKILL.md
` + "```" + `

The ` + "`SKILL.md`" + ` file should contain YAML frontmatter defining the skill's description and requirements.
`
		os.WriteFile(readmePath, []byte(readmeContent), 0644)
	}

	// Post-process skills to replace {baseDir}
	err := filepath.Walk(skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "SKILL.md" {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			skillDir := filepath.Dir(path)
			absDir, _ := filepath.Abs(skillDir)

			fmt.Printf("Processing skill at %s (baseDir -> %s)\n", path, absDir)

			newContent := strings.ReplaceAll(string(content), "{baseDir}", absDir)
			if newContent != string(content) {
				if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
					fmt.Printf("Failed to write updated skill file: %v\n", err)
				} else {
					fmt.Printf("Updated {baseDir} in %s\n", path)
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Error walking skills dir: %v\n", err)
	}

	fmt.Println("Onboarding complete! Please edit .nanobot/config.json to add your API key.")
}
