# nanobot-go

This is the Go implementation of [nanobot](https://github.com/nanobot-ai/nanobot):Ultra-Lightweight Personal AI Assistant

# News

-- **2026-02-12**: Support Long-term Memory
-- **2026-02-09**: Add Dingtalk channel
-- **2026-02-07**: nanobot-go first commit

# getting started

First, navigate to the folder where you keep your projects and clone this repository to this folder:

```bash
git clone https://github.com/cuichuankai/nanobot-go.git
```

Then, open the repository folder:

```bash
cd nanobot-go
```
# compile the nanobot:
```bash
go build ./cmd/nanobot/
```

# 🚀 Quick Start

> [!TIP]
> Set your API key in `~/.nanobot/config.json`.
> Get API keys: [OpenRouter](https://openrouter.ai/keys) (Global) · [Brave Search](https://brave.com/search/api/) (optional, for web search)

**1. Initialize**

```bash
nanobot onboard
```

**2. Configure** (`~/.nanobot/config.json`)

For OpenRouter - recommended for global users:
```json
{
  "providers": {
    "openrouter": {
      "apiKey": "sk-or-v1-xxx"
    }
  },
  "agents": {
    "defaults": {
      "model": "anthropic/claude-opus-4-5"
    }
  }
}
```

**3. Chat**

```bash
nanobot agent -m "What is 2+2?"
```
That's it! You have a working AI assistant in 2 minutes.

## 🔌 Feishu Channel

**1. Create a Feishu bot**
- Visit [Feishu Open Platform](https://open.feishu.cn/app)
- Create a new app → Enable **Bot** capability
- **Permissions**: Add `im:message` (send messages)
- **Events**: Add `im.message.receive_v1` (receive messages)
  - Select **Long Connection** mode (requires running nanobot first to establish connection)
- Get **App ID** and **App Secret** from "Credentials & Basic Info"
- Publish the app

**2. Configure**

```json
{
  "channels": {
    "feishu": {
      "enabled": true,
      "appId": "cli_xxx",
      "appSecret": "xxx",
      "encryptKey": "",
      "verificationToken": "",
      "allowFrom": []
    }
  }
}
```

> `encryptKey` and `verificationToken` are optional for Long Connection mode.
> `allowFrom`: Leave empty to allow all users, or add `["ou_xxx"]` to restrict access.

**3. Run**

```bash
nanobot agent
```

## Dingtalk Channel

**1. Create a Dingtalk bot**
- Visit [Dingtalk Open Platform](https://open.dingtalk.com/document/dingstart/robot-application-overview)
- Create a new app → Enable **Bot** capability
- **Permissions**: Add `qyapi_robot_sendmsg` and 'Robot.SingleChat.ReadWrite' (send messages). If wannt use AICard for stream mode, also add 'qyapi_chat_manage' and 'Card.Streaming.Write'
- **Events**: Change to stream mode
- Get **clientId**, **appSecret** and **robotCode** from "Credentials & Basic Info"
- Publish the app
- Add the robot to your Dingtalk group

**2. Configure**

```json
{
  "channels": {
    "dingtalk": {
      "enabled": true,
      "clientId": "",
      "appSecret": "",
      "robotCode": "", 
      "allowFrom": null
    }
  }
}
```
**3. Run**

```bash
nanobot agent
```
