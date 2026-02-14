package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HKUDS/nanobot-go/pkg/bus"
	"github.com/HKUDS/nanobot-go/pkg/config"
	"github.com/HKUDS/nanobot-go/pkg/utils"

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
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[DingTalk] Panic recovered in stream client goroutine: %v", r)
			}
		}()

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
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DingTalk] Panic recovered in onChatReceive: %v", r)
		}
	}()

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

	// 处理流消息
	if msg.Stream != nil {
		// 1. 如果是文本消息，我们需要流内容作为最终的 Content
		if msg.Type == bus.MessageTypeText || msg.Type == "" {
			// 如果配置了 TemplateID，且是文本，尝试使用卡片流式发送
			if c.Config.TemplateID != "" {
				return c.sendStream(msg, token)
			}

			// 否则降级：同步读取流，拼接为文本
			var builder strings.Builder
			for chunk := range msg.Stream {
				builder.WriteString(chunk)
			}
			msg.Content = builder.String()
		} else {
			// 2. 如果是媒体消息（Image/Audio/Video），钉钉不支持 Caption
			// 我们不需要流的内容，但必须消费掉流以防发送端阻塞
			// 使用 goroutine 异步排空，避免阻塞媒体发送
			go func(stream <-chan string) {
				for range stream {
					// 丢弃内容
				}
			}(msg.Stream)
		}
	}

	log.Printf("[DingTalk] Sending message to %s (len=%d) Type=%s", msg.ChatID, len(msg.Content), msg.Type)

	switch msg.Type {
	case bus.MessageTypeImage:
		if msg.Media == "" {
			return fmt.Errorf("media is empty")
		}
		reader, filename, err := utils.GetMediaReader(msg.Media)
		if err != nil {
			return err
		}
		defer reader.Close()

		mediaId, err := c.uploadMedia(token, "image", filename, reader)
		if err != nil {
			return err
		}

		log.Printf("[DingTalk] Image uploaded, mediaId: %s", mediaId)

		param := map[string]string{
			"photoURL": mediaId,
			"picURL":   mediaId,
		}
		return c.sendMedia(token, msg.ChatID, "sampleImageMsg", param)

	case bus.MessageTypeAudio:
		if msg.Media == "" {
			return fmt.Errorf("media is empty")
		}
		reader, filename, err := utils.GetMediaReader(msg.Media)
		if err != nil {
			return err
		}
		defer reader.Close()

		mediaId, err := c.uploadMedia(token, "voice", filename, reader)
		if err != nil {
			return err
		}

		param := map[string]string{"mediaId": mediaId, "duration": "10"}
		return c.sendMedia(token, msg.ChatID, "sampleAudio", param)

	case bus.MessageTypeVideo:
		if msg.Media == "" {
			return fmt.Errorf("media is empty")
		}
		reader, filename, err := utils.GetMediaReader(msg.Media)
		if err != nil {
			return err
		}
		defer reader.Close()

		videoMediaId, err := c.uploadMedia(token, "video", filename, reader)
		if err != nil {
			return err
		}

		picMediaId, err := c.getCoverMediaId(token)
		if err != nil {
			log.Printf("failed to get cover media id: %v", err)
			return err
		}

		param := map[string]string{
			"videoMediaId": videoMediaId,
			"picMediaId":   picMediaId,
			"duration":     "10",
			"videoType":    "mp4",
		}
		return c.sendMedia(token, msg.ChatID, "sampleVideo", param)

	default:
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

func (c *DingTalkChannel) uploadMedia(token, mediaType, filename string, reader io.Reader) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("media", filename)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, reader)
	if err != nil {
		return "", err
	}
	writer.Close()

	url := fmt.Sprintf("https://oapi.dingtalk.com/media/upload?access_token=%s&type=%s", token, mediaType)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaId string `json:"media_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk upload failed: %d %s", result.ErrCode, result.ErrMsg)
	}

	return result.MediaId, nil
}

var dingtalkDefaultCoverPng = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x01, 0x03, 0x00, 0x00, 0x00, 0x25, 0xdb, 0x56, 0xca, 0x00, 0x00, 0x00,
	0x03, 0x50, 0x4c, 0x54, 0x45, 0x00, 0x00, 0x00, 0xa7, 0x7a, 0x3d, 0xda,
	0x00, 0x00, 0x00, 0x01, 0x74, 0x52, 0x4e, 0x53, 0x00, 0x40, 0xe6, 0xd8,
	0x66, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63,
	0x60, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func (c *DingTalkChannel) getCoverMediaId(token string) (string, error) {
	r := bytes.NewReader(dingtalkDefaultCoverPng)
	return c.uploadMedia(token, "image", "cover.png", r)
}

func (c *DingTalkChannel) sendMedia(token, chatID, msgKey string, param interface{}) error {
	paramBytes, _ := json.Marshal(param)
	msgParam := string(paramBytes)

	if strings.HasPrefix(chatID, "cid") {
		headers := &dingtalkrobot.OrgGroupSendHeaders{
			XAcsDingtalkAccessToken: tea.String(token),
		}
		req := &dingtalkrobot.OrgGroupSendRequest{
			RobotCode:          tea.String(c.Config.RobotCode),
			OpenConversationId: tea.String(chatID),
			MsgKey:             tea.String(msgKey),
			MsgParam:           tea.String(msgParam),
		}
		_, err := c.robotClient.OrgGroupSendWithOptions(req, headers, &util.RuntimeOptions{})
		return err
	}

	headers := &dingtalkrobot.BatchSendOTOHeaders{
		XAcsDingtalkAccessToken: tea.String(token),
	}
	req := &dingtalkrobot.BatchSendOTORequest{
		RobotCode: tea.String(c.Config.RobotCode),
		UserIds:   []*string{tea.String(chatID)},
		MsgKey:    tea.String(msgKey),
		MsgParam:  tea.String(msgParam),
	}
	_, err := c.robotClient.BatchSendOTOWithOptions(req, headers, &util.RuntimeOptions{})
	return err
}
