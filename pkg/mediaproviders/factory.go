package mediaproviders

import (
	"strings"

	"github.com/HKUDS/nanobot-go/pkg/config"
)

// Factory creates media providers based on configuration and model name.
type Factory struct {
	Config *config.Config
}

// NewFactory creates a new media provider factory.
func NewFactory(cfg *config.Config) *Factory {
	return &Factory{Config: cfg}
}

// GetProvider returns a provider instance suitable for the given model.
func (f *Factory) GetProvider(model string) Provider {
	// 1. OpenAI Models
	if strings.HasPrefix(model, "dall-e") || strings.HasPrefix(model, "tts") {
		apiKey := f.Config.Providers.OpenAI.APIKey
		return NewOpenAIProvider(apiKey)
	}

	// 2. Google Models (Placeholder)
	if strings.HasPrefix(model, "gemini") {
		// apiKey := f.Config.Providers.Gemini.APIKey
		// return NewGoogleProvider(apiKey)
	}

	// 3. Default to SiliconFlow for everything else (Flux, Qwen, Stable Diffusion, etc.)
	// Check if SiliconFlow config exists, otherwise fallback to OpenAI or empty?
	// Assuming SiliconFlow is the primary backend for open models.
	apiKey := f.Config.Providers.SiliconFlow.APIKey
	// If SiliconFlow key is missing, check if it's in OpenRouter or others?
	// For now, stick to dedicated config.
	return NewSiliconFlowProvider(apiKey)
}
