package mediaproviders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// OpenAIProvider implements Provider for OpenAI DALL-E and TTS.
type OpenAIProvider struct {
	APIKey string
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{APIKey: apiKey}
}

func (p *OpenAIProvider) GenerateImage(prompt, model string) (string, error) {
	if model == "" {
		model = "dall-e-3"
	}
	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"size":   "1024x1024",
		"n":      1,
	}
	return p.callAPI("https://api.openai.com/v1/images/generations", reqBody)
}

func (p *OpenAIProvider) EditImage(prompt, imageURL, model string) (string, error) {
	return "", fmt.Errorf("OpenAI edit image (DALL-E 2) requires file upload, not URL")
}

func (p *OpenAIProvider) GenerateVideo(prompt, imageURL, model string) (string, error) {
	return "", fmt.Errorf("OpenAI does not support video generation yet")
}

func (p *OpenAIProvider) GenerateAudio(input, model string) (string, error) {
	if model == "" {
		model = "tts-1"
	}
	reqBody := map[string]interface{}{
		"model": model,
		"input": input,
		"voice": "alloy",
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	fileName := fmt.Sprintf("openai_audio_%d.mp3", time.Now().Unix())
	filePath := filepath.Join(os.TempDir(), fileName)
	if err := ioutil.WriteFile(filePath, body, 0644); err != nil {
		return "", fmt.Errorf("failed to save audio file: %v", err)
	}

	return filePath, nil
}

func (p *OpenAIProvider) callAPI(url string, reqBody map[string]interface{}) (string, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if len(result.Data) > 0 && result.Data[0].URL != "" {
		return result.Data[0].URL, nil
	}

	return "", fmt.Errorf("no URL found in response")
}
