package tools

import (
	"fmt"

	"github.com/HKUDS/nanobot-go/pkg/config"
	"github.com/HKUDS/nanobot-go/pkg/mediaproviders"
)

// MediaGenTool supports various media generation tasks using pluggable providers.
type MediaGenTool struct {
	BaseTool
	Factory *mediaproviders.Factory
	Config  *config.Config
}

// NewMediaGenTool creates a new MediaGenTool.
func NewMediaGenTool(cfg *config.Config) *MediaGenTool {
	return &MediaGenTool{
		Config:  cfg,
		Factory: mediaproviders.NewFactory(cfg),
	}
}

func (t *MediaGenTool) Name() string {
	return "media-generation"
}

func (t *MediaGenTool) Description() string {
	return "Generate media content (images, videos, audio). Supports text-to-image, image-to-image (editing), image-to-video, and text-to-audio."
}

func (t *MediaGenTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *MediaGenTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The type of generation task.",
				"enum":        []string{"text-to-image", "image-to-image", "image-to-video", "text-to-audio"},
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Text prompt describing the content to generate.",
			},
			"image_url": map[string]interface{}{
				"type":        "string",
				"description": "Source image URL (required for image-to-image and image-to-video).",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Specific model to use (optional).",
			},
		},
		"required": []string{"task", "prompt"},
	}
}

func (t *MediaGenTool) Execute(args map[string]interface{}) (string, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	task, _ := args["task"].(string)
	model, _ := args["model"].(string)
	imageURL, _ := args["image_url"].(string)

	// Determine default model if not provided
	if model == "" {
		switch task {
		case "text-to-image":
			model = t.Config.Tools.Media.DefaultTextToImageModel
		case "image-to-image":
			model = t.Config.Tools.Media.DefaultImageToImageModel
		case "image-to-video":
			model = t.Config.Tools.Media.DefaultImageToVideoModel
		case "text-to-audio":
			model = t.Config.Tools.Media.DefaultTextToAudioModel
		}
	}

	// Use OpenAI API Key from environment if model is OpenAI-specific and config is missing
	// (Though Factory handles this via config, we can inject env vars into config if needed,
	// but better to rely on what Factory does with Config)

	provider := t.Factory.GetProvider(model)
	if provider == nil {
		return "", fmt.Errorf("no provider found for model: %s", model)
	}

	// Helper for checking image_url
	checkImageURL := func() error {
		if imageURL == "" {
			return fmt.Errorf("image_url is required for %s", task)
		}
		return nil
	}

	switch task {
	case "text-to-image":
		return provider.GenerateImage(prompt, model)
	case "image-to-image":
		if err := checkImageURL(); err != nil {
			return "", err
		}
		return provider.EditImage(prompt, imageURL, model)
	case "image-to-video":
		if err := checkImageURL(); err != nil {
			return "", err
		}
		return provider.GenerateVideo(prompt, imageURL, model)
	case "text-to-audio":
		return provider.GenerateAudio(prompt, model)
	default:
		return "", fmt.Errorf("unsupported task: %s", task)
	}
}
