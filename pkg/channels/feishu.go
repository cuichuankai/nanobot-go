package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/config"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// FeishuChannel implements the Feishu channel.
type FeishuChannel struct {
	BaseChannel
	Config    *config.FeishuConfig
	Workspace string
	client    *lark.Client
	wsClient  *larkws.Client
}

// NewFeishuChannel creates a new FeishuChannel.
func NewFeishuChannel(cfg *config.FeishuConfig, messageBus *bus.MessageBus, workspace string) *FeishuChannel {
	return &FeishuChannel{
		BaseChannel: BaseChannel{
			Config:    cfg,
			Bus:       messageBus,
			AllowFrom: cfg.AllowFrom,
		},
		Config:    cfg,
		Workspace: workspace,
	}
}

func (c *FeishuChannel) Name() string {
	return "feishu"
}

func (c *FeishuChannel) getAgentName() string {
	if c.Workspace == "" {
		return "Nanobot"
	}
	path := filepath.Join(c.Workspace, "SOUL.md")
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "Nanobot"
	}
	text := string(content)

	// Try "名字叫XX" or "名字是XX" (supports Chinese punctuation)
	re := regexp.MustCompile(`名字[叫是]([^，,。.\n]+)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try "Name: XX"
	reEn := regexp.MustCompile(`(?i)Named[:\s]+([^,\n]+)`)
	matchesEn := reEn.FindStringSubmatch(text)
	if len(matchesEn) > 1 {
		return strings.TrimSpace(matchesEn[1])
	}

	return "Nanobot"
}

func (c *FeishuChannel) Start() error {
	if !c.Config.Enabled || c.Config.AppID == "" || c.Config.AppSecret == "" {
		return nil
	}

	// API Client (for sending messages)
	c.client = lark.NewClient(c.Config.AppID, c.Config.AppSecret)

	// WebSocket Client (for receiving messages)
	// For WebSocket, we use the dispatcher but VerificationToken and EncryptKey are generally not used for signature validation
	// in the same way as Webhooks, but we pass them if available.
	handler := larkdispatcher.NewEventDispatcher(c.Config.VerificationToken, c.Config.EncryptKey).
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			// Extract message content
			content := *event.Event.Message.Content
			// msgType := *event.Event.Message.MsgType // Removed due to compilation error
			log.Printf("Received Feishu event content: %s", content)

			var textContent string

			// Try to parse as text message
			var msgContent struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(content), &msgContent); err == nil && msgContent.Text != "" {
				textContent = msgContent.Text
			} else {
				// Fallback: try to parse generic map
				var generic map[string]interface{}
				if err := json.Unmarshal([]byte(content), &generic); err == nil {
					// Check for "title" and "content" (Post message)
					if _, ok := generic["content"]; ok {
						textContent = fmt.Sprintf("[Rich Text] %s", content)
					} else {
						textContent = content
					}
				} else {
					textContent = content
				}
			}

			chatID := *event.Event.Message.ChatId
			senderID := *event.Event.Sender.SenderId.OpenId

			// Check allow list
			if !c.IsAllowed(senderID) {
				log.Printf("Feishu message from unauthorized user: %s", senderID)
				return nil
			}

			// Publish to bus
			c.Bus.PublishInbound(bus.InboundMessage{
				Channel:  c.Name(),
				SenderID: senderID,
				ChatID:   chatID,
				Content:  textContent,
			})

			return nil
		})

	c.wsClient = larkws.NewClient(
		c.Config.AppID,
		c.Config.AppSecret,
		larkws.WithEventHandler(handler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	go func() {
		log.Println("Starting Feishu WebSocket client...")
		if err := c.wsClient.Start(context.Background()); err != nil {
			log.Printf("Feishu WebSocket error: %v", err)
		}
	}()

	log.Println("Feishu bot started")
	return nil
}

func (c *FeishuChannel) sendStream(msg bus.OutboundMessage, receiveIDType string) error {
	ctx := context.Background()

	// 1. Create Card Entity
	elementID := "markdown_1"
	cardData := map[string]interface{}{
		"schema": "2.0",
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": c.getAgentName(),
			},
			"template": "blue",
		},
		"config": map[string]interface{}{
			"streaming_mode": true,
			"update_multi":   true,
			"summary": map[string]interface{}{
				"content": "[Generating...]",
			},
			"streaming_config": map[string]interface{}{
				"print_frequency_ms": map[string]interface{}{
					"default": 80,
					"android": 80,
					"ios":     80,
					"pc":      80,
				},
				"print_step": map[string]interface{}{
					"default": 2,
					"android": 2,
					"ios":     2,
					"pc":      2,
				},
				"print_strategy": "fast",
			},
		},
		"body": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{
					"tag":        "markdown",
					"element_id": elementID,
					"content":    "...", // Initial placeholder
				},
			},
		},
	}
	cardDataBytes, _ := json.Marshal(cardData)

	createCardReqBody := map[string]interface{}{
		"type": "card_json",
		"data": string(cardDataBytes),
	}

	req := &larkcore.ApiReq{
		HttpMethod:                "POST",
		ApiPath:                   "https://open.feishu.cn/open-apis/cardkit/v1/cards",
		Body:                      createCardReqBody,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	}

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create card entity: %w", err)
	}

	// Parse response
	var createCardResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			CardID string `json:"card_id"`
		} `json:"data"`
	}
	// Try to decode from RawBody if available, or Body
	if resp.RawBody != nil {
		if err := json.Unmarshal(resp.RawBody, &createCardResp); err != nil {
			return fmt.Errorf("failed to parse create card response: %w", err)
		}
	} else {
		return fmt.Errorf("response body is empty")
	}

	if createCardResp.Code != 0 {
		return fmt.Errorf("create card failed: %d %s", createCardResp.Code, createCardResp.Msg)
	}
	cardID := createCardResp.Data.CardID

	// 2. Send Message with card_id
	msgContent := map[string]interface{}{
		"type": "card",
		"data": map[string]interface{}{
			"card_id": cardID,
		},
	}
	msgContentBytes, _ := json.Marshal(msgContent)

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(string(msgContentBytes)).
			Build()).
		Build()

	msgResp, err := c.client.Im.Message.Create(ctx, msgReq)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	if !msgResp.Success() {
		return fmt.Errorf("feishu send message failed: %d %s", msgResp.Code, msgResp.Msg)
	}

	// 3. Loop stream updates
	sequence := 1
	var contentBuilder strings.Builder
	ticker := time.NewTicker(120 * time.Millisecond) // Limit updates to ~8 times/second (safe below 10/s limit)
	defer ticker.Stop()

	var hasPending bool

	for {
		select {
		case chunk, ok := <-msg.Stream:
			if !ok {
				// Stream closed, send remaining content if any
				if hasPending {
					fullContent := contentBuilder.String()

					updateReqBody := map[string]interface{}{
						"content":  fullContent,
						"sequence": sequence,
					}
					sequence++

					updateReq := &larkcore.ApiReq{
						HttpMethod:                "PUT",
						ApiPath:                   fmt.Sprintf("https://open.feishu.cn/open-apis/cardkit/v1/cards/%s/elements/%s/content", cardID, elementID),
						Body:                      updateReqBody,
						SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
					}
					c.client.Do(ctx, updateReq)
				}
				goto StreamDone
			}
			contentBuilder.WriteString(chunk)
			hasPending = true

		case <-ticker.C:
			if hasPending {
				fullContent := contentBuilder.String()

				updateReqBody := map[string]interface{}{
					"content":  fullContent,
					"sequence": sequence,
				}
				sequence++

				updateReq := &larkcore.ApiReq{
					HttpMethod:                "PUT",
					ApiPath:                   fmt.Sprintf("https://open.feishu.cn/open-apis/cardkit/v1/cards/%s/elements/%s/content", cardID, elementID),
					Body:                      updateReqBody,
					SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
				}

				// Log request for debugging
				// log.Printf("Sending update seq=%d len=%d", sequence-1, len(fullContent))

				updateResp, err := c.client.Do(ctx, updateReq)
				if err != nil {
					log.Printf("Failed to update stream: %v", err)
					continue
				}
				if updateResp.StatusCode != 200 {
					log.Printf("Update stream failed status: %d", updateResp.StatusCode)
				}
				hasPending = false
			}
		}
	}

StreamDone:
	// 4. Close streaming mode
	// If no content was received, update the card to indicate that.
	if contentBuilder.Len() == 0 {
		updateReqBody := map[string]interface{}{
			"content":  "No content generated.",
			"sequence": sequence,
		}
		updateReq := &larkcore.ApiReq{
			HttpMethod:                "PUT",
			ApiPath:                   fmt.Sprintf("https://open.feishu.cn/open-apis/cardkit/v1/cards/%s/elements/%s/content", cardID, elementID),
			Body:                      updateReqBody,
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
		}
		c.client.Do(ctx, updateReq)
	}

	closeReqBody := map[string]interface{}{
		"config": map[string]interface{}{
			"streaming_mode": false,
		},
	}
	closeReq := &larkcore.ApiReq{
		HttpMethod:                "PATCH",
		ApiPath:                   fmt.Sprintf("https://open.feishu.cn/open-apis/cardkit/v1/cards/%s/settings", cardID),
		Body:                      closeReqBody,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	}
	// We don't strictly care if this fails, but good to try
	c.client.Do(ctx, closeReq)

	return nil
}

func (c *FeishuChannel) Stop() error {
	// larkws.Client doesn't seem to have a Stop method exposed in some versions,
	// but usually context cancellation is used.
	// The current SDK version's Start takes a context, but we passed Background().
	// Ideally we should store the context cancel function.
	// However, looking at the struct, let's see if we can just leave it for now
	// as the program exit handles it.
	return nil
}

func (c *FeishuChannel) Send(msg bus.OutboundMessage) error {
	if c.client == nil {
		return fmt.Errorf("feishu client not initialized")
	}

	receiveIDType := larkim.ReceiveIdTypeOpenId
	if len(msg.ChatID) > 3 && msg.ChatID[:3] == "oc_" {
		receiveIDType = larkim.ReceiveIdTypeChatId
	}

	if msg.Stream != nil {
		return c.sendStream(msg, receiveIDType)
	}

	// Construct Interactive Card
	cardContent := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": c.getAgentName(),
			},
			"template": "blue",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": msg.Content,
				},
			},
		},
	}
	contentJSON, _ := json.Marshal(cardContent)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(string(contentJSON)).
			Build()).
		Build()

	resp, err := c.client.Im.Message.Create(context.Background(), req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu error: %d %s", resp.Code, resp.Msg)
	}

	return nil
}
