package channels

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramChannel implements the Telegram channel.
type TelegramChannel struct {
	BaseChannel
	Config *config.TelegramConfig
	bot    *tgbotapi.BotAPI
	running bool
}

// NewTelegramChannel creates a new TelegramChannel.
func NewTelegramChannel(cfg *config.TelegramConfig, messageBus *bus.MessageBus) *TelegramChannel {
	return &TelegramChannel{
		BaseChannel: BaseChannel{
			Config:    cfg,
			Bus:       messageBus,
			AllowFrom: cfg.AllowFrom,
		},
		Config: cfg,
	}
}

func (c *TelegramChannel) Name() string {
	return "telegram"
}

func (c *TelegramChannel) Start() error {
	if !c.Config.Enabled || c.Config.Token == "" {
		return nil
	}

	var err error
	c.bot, err = tgbotapi.NewBotAPI(c.Config.Token)
	if err != nil {
		return fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	log.Printf("Telegram bot authorized on account %s", c.bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := c.bot.GetUpdatesChan(u)
	c.running = true

	go func() {
		for update := range updates {
			if !c.running {
				break
			}
			if update.Message == nil {
				continue
			}

			c.handleUpdate(update)
		}
	}()

	return nil
}

func (c *TelegramChannel) Stop() error {
	c.running = false
	if c.bot != nil {
		c.bot.StopReceivingUpdates()
	}
	return nil
}

func (c *TelegramChannel) Send(msg bus.OutboundMessage) error {
	if c.bot == nil {
		return fmt.Errorf("telegram bot not initialized")
	}

	chatID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %s", msg.ChatID)
	}

	content := msg.Content
	if msg.Stream != nil {
		var sb strings.Builder
		for chunk := range msg.Stream {
			sb.WriteString(chunk)
		}
		content = sb.String()
	}

	if content == "" {
		return nil
	}

	// Simple text message for now, HTML parsing is complex
	// TODO: Implement Markdown/HTML conversion
	reply := tgbotapi.NewMessage(chatID, content)
	_, err = c.bot.Send(reply)
	return err
}

func (c *TelegramChannel) handleUpdate(update tgbotapi.Update) {
	msg := update.Message
	senderID := strconv.FormatInt(msg.From.ID, 10)
	if msg.From.UserName != "" {
		senderID = fmt.Sprintf("%s|%s", senderID, msg.From.UserName)
	}

	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	content := msg.Text

	if msg.Caption != "" {
		content = msg.Caption
	}

	// Handle /start
	if msg.IsCommand() && msg.Command() == "start" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "👋 Hi! I'm nanobot.\n\nSend me a message and I'll respond!")
		c.bot.Send(reply)
		return
	}

	// Basic media handling (placeholders)
	var media []string
	if msg.Photo != nil {
		content = "[Photo received]" // Download logic omitted for brevity
	} else if msg.Voice != nil {
		content = "[Voice received]"
	}

	if content == "" {
		content = "[Empty message]"
	}

	metadata := map[string]interface{}{
		"message_id": msg.MessageID,
		"username":   msg.From.UserName,
		"first_name": msg.From.FirstName,
	}

	c.HandleMessage(c.Name(), senderID, chatID, content, media, metadata)
}
