package providers

import (
	"fmt"
	"os"
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/config"
)

// NewProvider creates a new LLM provider based on configuration.
func NewProvider(cfg *config.Config) (LLMProvider, error) {
	defaultModel := cfg.Agents.Defaults.Model
	explicitProvider := cfg.Agents.Defaults.Provider

	// Helper to check env if config is empty
	checkEnv := func(cfgVal, envKey string) string {
		if cfgVal != "" {
			return cfgVal
		}
		return os.Getenv(envKey)
	}

	// 1. Explicit selection
	if explicitProvider != "" {
		switch strings.ToLower(explicitProvider) {
		case "openai":
			apiKey := checkEnv(cfg.Providers.OpenAI.APIKey, "OPENAI_API_KEY")
			return NewOpenAIProvider(apiKey, cfg.Providers.OpenAI.APIBase, defaultModel), nil
		case "anthropic":
			// Assuming Anthropic uses OpenAI-compatible endpoint or we have a specific provider
			// For now, if we don't have AnthropicProvider, we might fail or use generic if compatible
			// But Anthropic API is different.
			// TODO: Implement AnthropicProvider if not using OpenRouter
			return nil, fmt.Errorf("anthropic provider not implemented yet (use openrouter)")
		case "deepseek":
			apiKey := checkEnv(cfg.Providers.DeepSeek.APIKey, "DEEPSEEK_API_KEY")
			apiBase := cfg.Providers.DeepSeek.APIBase
			if apiBase == "" {
				apiBase = "https://api.deepseek.com"
			}
			return NewOpenAIProvider(apiKey, apiBase, defaultModel), nil
		case "openrouter":
			apiKey := checkEnv(cfg.Providers.OpenRouter.APIKey, "OPENROUTER_API_KEY")
			apiBase := cfg.Providers.OpenRouter.APIBase
			if apiBase == "" {
				apiBase = "https://openrouter.ai/api/v1"
			}
			return NewOpenAIProvider(apiKey, apiBase, defaultModel), nil
		case "vllm":
			apiKey := checkEnv(cfg.Providers.VLLM.APIKey, "VLLM_API_KEY")
			apiBase := cfg.Providers.VLLM.APIBase
			return NewOpenAIProvider(apiKey, apiBase, defaultModel), nil
		case "gemini":
			apiKey := checkEnv(cfg.Providers.Gemini.APIKey, "GEMINI_API_KEY")
			// Gemini has an OpenAI compatible endpoint now
			apiBase := cfg.Providers.Gemini.APIBase
			if apiBase == "" {
				apiBase = "https://generativelanguage.googleapis.com/v1beta/openai/"
			}
			return NewOpenAIProvider(apiKey, apiBase, defaultModel), nil
		default:
			return nil, fmt.Errorf("unknown provider: %s", explicitProvider)
		}
	}

	// 2. Heuristic selection based on keys (Precedence: OpenRouter > DeepSeek > OpenAI > ...)
	
	// OpenRouter
	if key := checkEnv(cfg.Providers.OpenRouter.APIKey, "OPENROUTER_API_KEY"); key != "" {
		apiBase := cfg.Providers.OpenRouter.APIBase
		if apiBase == "" {
			apiBase = "https://openrouter.ai/api/v1"
		}
		return NewOpenAIProvider(key, apiBase, defaultModel), nil
	}

	// DeepSeek
	if key := checkEnv(cfg.Providers.DeepSeek.APIKey, "DEEPSEEK_API_KEY"); key != "" {
		apiBase := cfg.Providers.DeepSeek.APIBase
		if apiBase == "" {
			apiBase = "https://api.deepseek.com"
		}
		return NewOpenAIProvider(key, apiBase, defaultModel), nil
	}

	// OpenAI
	if key := checkEnv(cfg.Providers.OpenAI.APIKey, "OPENAI_API_KEY"); key != "" {
		apiBase := cfg.Providers.OpenAI.APIBase
		return NewOpenAIProvider(key, apiBase, defaultModel), nil
	}

	// VLLM
	if key := checkEnv(cfg.Providers.VLLM.APIKey, "VLLM_API_KEY"); key != "" {
		return NewOpenAIProvider(key, cfg.Providers.VLLM.APIBase, defaultModel), nil
	}
	
	// Gemini
	if key := checkEnv(cfg.Providers.Gemini.APIKey, "GEMINI_API_KEY"); key != "" {
		apiBase := cfg.Providers.Gemini.APIBase
		if apiBase == "" {
			apiBase = "https://generativelanguage.googleapis.com/v1beta/openai/"
		}
		return NewOpenAIProvider(key, apiBase, defaultModel), nil
	}

	// Zhipu
	if key := checkEnv(cfg.Providers.Zhipu.APIKey, "ZHIPU_API_KEY"); key != "" {
		apiBase := cfg.Providers.Zhipu.APIBase
		if apiBase == "" {
			apiBase = "https://open.bigmodel.cn/api/paas/v4/"
		}
		return NewOpenAIProvider(key, apiBase, defaultModel), nil
	}

	// Groq
	if key := checkEnv(cfg.Providers.Groq.APIKey, "GROQ_API_KEY"); key != "" {
		apiBase := cfg.Providers.Groq.APIBase
		if apiBase == "" {
			apiBase = "https://api.groq.com/openai/v1"
		}
		return NewOpenAIProvider(key, apiBase, defaultModel), nil
	}

	return nil, fmt.Errorf("no API key configured for any provider")
}
