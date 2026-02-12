package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/config"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dingtalkim "github.com/alibabacloud-go/dingtalk/im_1_0"
	dingtalkoauth2 "github.com/alibabacloud-go/dingtalk/oauth2_1_0"
	dingtalkrobot "github.com/alibabacloud-go/dingtalk/robot_1_0"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/google/uuid"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/logger"
)

type DingTalkChannel struct {
	BaseChannel
	Config       *config.DingTalkConfig
	streamClient *client.StreamClient
	robotClient  *dingtalkrobot.Client
	imClient     *dingtalkim.Client
	oauthClient  *dingtalkoauth2.Client

	tokenMu       sync.RWMutex
	accessToken   string
	tokenExpireAt time.Time
}

func NewDingTalkChannel(cfg *config.DingTalkConfig, messageBus *bus.MessageBus) *DingTalkChannel {
	return &DingTalkChannel{
		BaseChannel: BaseChannel{
			Config:    cfg,
			Bus:       messageBus,
			AllowFrom: cfg.AllowFrom,
		},
		Config: cfg,
	}
}

func (c *DingTalkChannel) Name() string {
	return "dingtalk"
}

func (c *DingTalkChannel) Start() error {
	if !c.Config.Enabled || c.Config.ClientID == "" || c.Config.AppSecret == "" {
		return nil
	}

	apiConfig := &openapi.Config{
		Protocol: tea.String("https"),
		RegionId: tea.String("central"),
	}

	// Robot Client
	robotClient, err := dingtalkrobot.NewClient(apiConfig)
	if err != nil {
		return fmt.Errorf("failed to init dingtalk robot client: %v", err)
	}
	c.robotClient = robotClient

	// IM Client
	imClient, err := dingtalkim.NewClient(apiConfig)
	if err != nil {
		return fmt.Errorf("failed to init dingtalk im client: %v", err)
	}
	c.imClient = imClient

	// OAuth Client
	oauthClient, err := dingtalkoauth2.NewClient(apiConfig)
	if err != nil {
		return fmt.Errorf("failed to init dingtalk oauth client: %v", err)
	}
	c.oauthClient = oauthClient

	// Initialize Stream Client
	logger.SetLogger(logger.NewStdTestLogger())
	c.streamClient = client.NewStreamClient(client.WithAppCredential(client.NewAppCredentialConfig(c.Config.ClientID, c.Config.AppSecret)))
	c.streamClient.RegisterChatBotCallbackRouter(c.onChatReceive)

	go func() {
		log.Println("Starting DingTalk Stream Client...")
		// Start is blocking, so run in goroutine
		if err := c.streamClient.Start(context.Background()); err != nil {
			log.Printf("DingTalk Stream Client error: %v", err)
		}
	}()

	log.Println("DingTalk bot started")
	return nil
}

func (c *DingTalkChannel) Stop() error {
	if c.streamClient != nil {
		c.streamClient.Close()
	}
	return nil
}

func (c *DingTalkChannel) getAccessToken() (string, error) {
	c.tokenMu.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpireAt) {
		defer c.tokenMu.RUnlock()
		return c.accessToken, nil
	}
	c.tokenMu.RUnlock()

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Double check
	if c.accessToken != "" && time.Now().Before(c.tokenExpireAt) {
		return c.accessToken, nil
	}

	req := &dingtalkoauth2.GetAccessTokenRequest{
		AppKey:    tea.String(c.Config.ClientID),
		AppSecret: tea.String(c.Config.AppSecret),
	}
	resp, err := c.oauthClient.GetAccessToken(req)
	if err != nil {
		return "", err
	}

	if resp.Body == nil || resp.Body.AccessToken == nil {
		return "", fmt.Errorf("failed to get access token, response body is empty")
	}

	c.accessToken = *resp.Body.AccessToken
	// ExpireIn is seconds. Buffer it by 60s
	expireIn := *resp.Body.ExpireIn
	c.tokenExpireAt = time.Now().Add(time.Duration(expireIn-60) * time.Second)

	return c.accessToken, nil
}

func (c *DingTalkChannel) onChatReceive(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
	content := strings.TrimSpace(data.Text.Content)
	if content == "" {
		log.Printf("[DingTalk] Empty content received")
		return nil, nil
	}

	senderStaffId := data.SenderStaffId
	if senderStaffId == "" {
		senderStaffId = data.SenderId
	}

	if senderStaffId == "" {
		log.Printf("[DingTalk] Message missing senderStaffId/senderId")
		return nil, nil
	}

	if !c.IsAllowed(senderStaffId) {
		log.Printf("[DingTalk] Message from unauthorized user: %s (Allowed: %v)", senderStaffId, c.Config.AllowFrom)
		return nil, nil
	}

	// Determine ChatID based on conversation type
	// conversationType: "1" for single chat, "2" for group chat
	conversationType := data.ConversationType
	conversationId := data.ConversationId

	targetId := senderStaffId
	if conversationType == "2" && conversationId != "" {
		targetId = conversationId
	}

	log.Printf("[DingTalk] Processing message from %s (Type=%s, ConvID=%s) -> ChatID: %s", senderStaffId, conversationType, conversationId, targetId)

	c.Bus.PublishInbound(bus.InboundMessage{
		Channel:  c.Name(),
		SenderID: senderStaffId,
		ChatID:   targetId,
		Content:  content,
		Metadata: map[string]interface{}{
			"sender_name": data.SenderNick,
		},
	})

	return nil, nil
}

type dingTalkSampleTextParam struct {
	Content string `json:"content"`
}

func (c *DingTalkChannel) Send(msg bus.OutboundMessage) error {
	token, err := c.getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %v", err)
	}

	// 优先使用互动卡片流式输出（需配置 TemplateID）
	if msg.Stream != nil && c.Config.TemplateID != "" {
		return c.sendStream(msg, token)
	}

	// 降级处理：消费流并拼接为普通文本
	if msg.Stream != nil {
		var builder strings.Builder
		for chunk := range msg.Stream {
			builder.WriteString(chunk)
		}
		msg.Content = builder.String()
	}

	log.Printf("[DingTalk] Sending message to %s (len=%d)", msg.ChatID, len(msg.Content))

	if msg.Content == "" {
		log.Printf("[DingTalk] Skipping empty message")
		return nil
	}

	// Heuristic: if ID starts with "cid", it is likely a conversation ID (group chat).
	// Skip OTO and try Group send directly to avoid "staffId.notExisted" errors.
	if strings.HasPrefix(msg.ChatID, "cid") {
		if errGroup := c.sendGroup(token, msg); errGroup != nil {
			return fmt.Errorf("failed to send dingtalk group message: %v", errGroup)
		}
		log.Printf("[DingTalk] Group send success (CID)")
		return nil
	}

	// Try OTO first (works for StaffID)
	if err := c.sendOTO(token, msg); err != nil {
		return fmt.Errorf("failed to send dingtalk message (OTO): %v", err)
	}

	log.Printf("[DingTalk] OTO send success")
	return nil
}

func (c *DingTalkChannel) sendStream(msg bus.OutboundMessage, token string) error {
	outTrackId := uuid.New().String()
	isGroup := strings.HasPrefix(msg.ChatID, "cid")

	// 1. 创建卡片（初始状态）
	// 使用 "..." 或其他 Loading 字符占位
	currentContent := "Thinking..."
	log.Printf("[DingTalk] Creating interactive card (TemplateID=%s, OutTrackID=%s)...", c.Config.TemplateID, outTrackId)
	if err := c.createInteractiveCard(token, outTrackId, msg.ChatID, isGroup, currentContent); err != nil {
		log.Printf("[DingTalk] Failed to create interactive card: %v. Fallback to text.", err)

		// 如果创建卡片失败，降级为普通文本发送
		var builder strings.Builder
		for chunk := range msg.Stream {
			builder.WriteString(chunk)
		}
		msg.Content = builder.String()
		if isGroup {
			return c.sendGroup(token, msg)
		}
		return c.sendOTO(token, msg)
	}

	// 2. 开启流式更新循环
	// 钉钉接口有频率限制，建议控制在 200ms 以上
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var contentBuilder strings.Builder
	var hasPending bool

	log.Printf("[DingTalk] Stream loop started. Waiting for chunks...")

	for {
		select {
		case chunk, ok := <-msg.Stream:
			if !ok {
				// Stream closed, send final update
				log.Printf("[DingTalk] Stream closed. Total len=%d. Pending=%v", contentBuilder.Len(), hasPending)
				if hasPending || contentBuilder.Len() > 0 {
					if err := c.updateInteractiveCard(token, outTrackId, contentBuilder.String()); err != nil {
						log.Printf("[DingTalk] Final card update failed: %v", err)
					} else {
						log.Printf("[DingTalk] Final card update success")
					}
				}
				return nil
			}
			contentBuilder.WriteString(chunk)
			hasPending = true

		case <-ticker.C:
			if hasPending {
				log.Printf("[DingTalk] Ticker update. Len=%d", contentBuilder.Len())
				if err := c.updateInteractiveCard(token, outTrackId, contentBuilder.String()); err != nil {
					log.Printf("[DingTalk] Update card failed: %v", err)
				}
				hasPending = false
			}
		}
	}
}

// createInteractiveCard 创建互动卡片实例
func (c *DingTalkChannel) createInteractiveCard(token, outTrackId, targetId string, isGroup bool, content string) error {
	headers := &dingtalkim.SendInteractiveCardHeaders{
		XAcsDingtalkAccessToken: tea.String(token),
	}

	req := &dingtalkim.SendInteractiveCardRequest{
		OutTrackId:     tea.String(outTrackId),
		CardTemplateId: tea.String(c.Config.TemplateID),
		CardData: &dingtalkim.SendInteractiveCardRequestCardData{
			CardParamMap: map[string]*string{
				"content":         tea.String(content),
				"text":            tea.String(content),
				"markdown":        tea.String(content),
				"body":            tea.String(content),
				"message":         tea.String(content),
				"description":     tea.String(content),
				"title":           tea.String(content),
				"header":          tea.String(content),
				"markdownContent": tea.String(content),
			},
		},
		RobotCode: tea.String(c.Config.RobotCode),
	}

	if isGroup {
		req.ConversationType = tea.Int32(1)
		req.OpenConversationId = tea.String(targetId)
	} else {
		req.ConversationType = tea.Int32(0)
		req.ReceiverUserIdList = []*string{tea.String(targetId)}
	}

	_, err := c.imClient.SendInteractiveCardWithOptions(req, headers, &util.RuntimeOptions{})
	return err
}

// updateInteractiveCard 更新互动卡片内容
func (c *DingTalkChannel) updateInteractiveCard(token, outTrackId, content string) error {
	headers := &dingtalkim.UpdateInteractiveCardHeaders{
		XAcsDingtalkAccessToken: tea.String(token),
	}

	req := &dingtalkim.UpdateInteractiveCardRequest{
		OutTrackId: tea.String(outTrackId),
		CardData: &dingtalkim.UpdateInteractiveCardRequestCardData{
			CardParamMap: map[string]*string{
				"content":     tea.String(content),
				"lastMessage": tea.String(content),
			},
		},
		CardOptions: &dingtalkim.UpdateInteractiveCardRequestCardOptions{
			UpdateCardDataByKey: tea.Bool(false),
		},
		RobotCode: tea.String(c.Config.RobotCode),
	}

	_, err := c.imClient.UpdateInteractiveCardWithOptions(req, headers, &util.RuntimeOptions{})
	return err
}

func (c *DingTalkChannel) sendOTO(token string, msg bus.OutboundMessage) error {
	headers := &dingtalkrobot.BatchSendOTOHeaders{
		XAcsDingtalkAccessToken: tea.String(token),
	}

	param := dingTalkSampleTextParam{Content: msg.Content}
	msgParamBytes, _ := json.Marshal(param)

	req := &dingtalkrobot.BatchSendOTORequest{
		RobotCode: tea.String(c.Config.RobotCode),
		UserIds:   []*string{tea.String(msg.ChatID)},
		MsgKey:    tea.String("sampleText"),
		MsgParam:  tea.String(string(msgParamBytes)),
	}

	_, err := c.robotClient.BatchSendOTOWithOptions(req, headers, &util.RuntimeOptions{})
	return err
}

func (c *DingTalkChannel) sendGroup(token string, msg bus.OutboundMessage) error {
	headers := &dingtalkrobot.OrgGroupSendHeaders{
		XAcsDingtalkAccessToken: tea.String(token),
	}

	param := dingTalkSampleTextParam{Content: msg.Content}
	msgParamBytes, _ := json.Marshal(param)

	req := &dingtalkrobot.OrgGroupSendRequest{
		RobotCode:          tea.String(c.Config.RobotCode),
		OpenConversationId: tea.String(msg.ChatID),
		MsgKey:             tea.String("sampleText"),
		MsgParam:           tea.String(string(msgParamBytes)),
	}

	_, err := c.robotClient.OrgGroupSendWithOptions(req, headers, &util.RuntimeOptions{})
	return err
}
