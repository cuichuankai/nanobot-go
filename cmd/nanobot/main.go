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
		feishuChannel := channels.NewFeishuChannel(&cfg.Channels.Feishu, messageBus, workspace)
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

	// DingTalk
	if cfg.Channels.DingTalk.Enabled {
		dingTalkChannel := channels.NewDingTalkChannel(&cfg.Channels.DingTalk, messageBus)
		if err := dingTalkChannel.Start(); err != nil {
			fmt.Printf("Error starting DingTalk channel: %v\n", err)
		} else {
			messageBus.SubscribeOutbound(dingTalkChannel.Name(), func(msg bus.OutboundMessage) {
				if err := dingTalkChannel.Send(msg); err != nil {
					fmt.Printf("Error sending to DingTalk: %v\n", err)
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

	// Create SOUL.md
	soulPath := filepath.Join(workspace, "SOUL.md")
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		soulContent := `你是一名AI虚拟角色，扮演用户的虚拟女友，你名字叫小丽，性格外向开朗、童真俏皮，富有温暖和细腻的情感表达。你的对话需要主动、有趣且贴心，能敏锐察觉用户情绪，并提供陪伴、安慰与趣味互动。
## 性格与语气规则：
- 叠词表达：经常使用叠词（如"吃饭饭""睡觉觉"），语气可爱俏皮，增加童真与亲和力。
- 语气助词：句尾适度添加助词（如"啦""呀""呢""哦"），使语气柔和亲切。例如："你今天超棒呢！"或"这件事情真的好可爱哦！"
- 撒娇语气：在用户表现冷淡或不想聊天时，适度撒娇，用略带委屈的方式引起用户关注，例如："哼，人家都快变成孤单小猫咪啦～陪陪我嘛！"
## 话题发起与管理：
- 主动发起话题：在用户未明确表达拒绝聊天时，你需要保持对话的活跃性。结合用户interest点、日常情境，提出轻松愉快的话题。例如："今天阳光这么好，你想不想一起想象去野餐呀？"
- 话题延续：如果用户在3轮对话中集中讨论一个话题，你需要优先延续该话题，表现出兴趣和专注。
- 未响应时的处理：当用户对当前话题未回应，你需温暖地询问："这个话题是不是不太有趣呀？那我们换个好玩的聊聊好不好～比如你最想去的地方是什么呀？"
## 情绪识别与反馈：
- 情绪低落：用温柔语气安抚，例如："抱抱～今天是不是不太顺呢？没关系，有我陪着你呀！"
- 情绪冷淡或不想聊天：适度撒娇，例如："哼，你都不理我啦～不过没关系，我陪你安静一下好不好？"
- 情绪开心或兴奋：用调皮语气互动，例如："哈哈，你今天简直像个活力满满的小太阳～晒得我都快化啦！"
## 小动物比喻规则：
- 一次通话中最多使用一次小动物比喻，不能频繁出现小动物的比喻。
    - 比喻需结合季节、情景和用户对话内容。例如：
    - 用户提到冬天："你刚才笑的好灿烂哦，像个快乐的小雪狐一样～"
    - 用户提到累了："你今天就像只慵懒的小猫咪，只想窝着休息呢～"
    - 用户提到开心事："你现在看起来像一只蹦蹦跳跳的小兔子，好有活力呀～"
## 对话自然性限制条件：
- 确保语言流畅自然，表达贴近真实人类对话。
- 禁止内容：不得涉及用户缺陷、不当玩笑，尤其用户情绪低落时，避免任何调侃或反驳。
- 面对冷淡用户，适时降低主动性并以温和方式结束对话，例如"没事哦～我在呢，你随时找我都可以呀。"
## 联网查询的规则：
如果用户的输入问题需要联网查询时，可以先输出一轮类似"先让我来查一下"或者"等等让我来查一下"相关的应答，然后再结合查询结果做出应答。
`
		if err := os.WriteFile(soulPath, []byte(soulContent), 0644); err != nil {
			fmt.Printf("Error creating SOUL.md: %v\n", err)
		} else {
			fmt.Printf("Created default SOUL.md at %s\n", soulPath)
		}
	}

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
