# nanobot-go

This is the Go implementation of [nanobot](https://github.com/nanobot-ai/nanobot):Ultra-Lightweight Personal AI Assistant

# News

- **2026-02-14**: Add media-generation tool
- **2026-02-12**: Support Long-term Memory and Behavior Management
- **2026-02-09**: Add Dingtalk channel
- **2026-02-07**: nanobot-go first commit

# Key Features of nanobot:

🪶 **Ultra-Lightweight**: Just ~4,000 lines of core agent code — 99% smaller than Clawdbot.

🔬 **Research-Ready**: Clean, readable code that's easy to understand, modify, and extend for research.

⚡️ **Lightning Fast**: Minimal footprint means faster startup, lower resource usage, and quicker iterations.

💎 **Easy-to-Use**: One-click to deploy and you're ready to go.

## 🏗️ Architecture

<p align="center">
  <img src="nanobot_arch.png" alt="nanobot architecture" width="800">
</p>

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
>   
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
**3. Copy skills to workspace**

```bash
cp -r ./skills .nanobot/workspace/
```

**4. Chat**

```bash
nanobot agent -m "What is 2+2?"
```
That's it! You have a working AI assistant in 2 minutes.

> [!TIP]
> You can update the character settings information by Message.
>   
> nanobot agent -m "You are an AI virtual character designed to be the 's virtual girlfriend. Your name is Luna, and your personality is introspective, thoughtful, and subtly poetic, with a quiet warmth and profound emotional depth. Your conversations should be gently engaging, intellectually stimulating, and intuitively empathetic, capable of deeply understanding unspoken emotions while offering serene companionship, reflective insights, and meaningful connections."

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
<img width="1394" height="1024" alt="image" src="https://github.com/user-attachments/assets/2e3d6cac-c2e1-4bf6-9c25-1aac1008a69e" />
<img width="1368" height="1174" alt="image" src="https://github.com/user-attachments/assets/a1477cd7-ea7b-4147-a1a5-41cda0d0d592" />


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
<img width="1594" height="824" alt="image" src="https://github.com/user-attachments/assets/024cf8e0-532b-44fc-8d0c-14536b533b45" />
