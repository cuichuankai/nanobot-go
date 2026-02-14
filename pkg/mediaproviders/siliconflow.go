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

// SiliconFlowProvider implements Provider for SiliconFlow API.
type SiliconFlowProvider struct {
	APIKey string
}

// NewSiliconFlowProvider creates a new SiliconFlow provider.
func NewSiliconFlowProvider(apiKey string) *SiliconFlowProvider {
	return &SiliconFlowProvider{APIKey: apiKey}
}

func (p *SiliconFlowProvider) GenerateImage(prompt, model string) (string, error) {
	reqBody := map[string]interface{}{
		"model":      model,
		"prompt":     prompt,
		"image_size": "928x1664",
		"batch_size": 1,
		"cfg":        4.5,
	}
	return p.callAPI("https://api.siliconflow.cn/v1/images/generations", reqBody)
}

func (p *SiliconFlowProvider) EditImage(prompt, imageURL, model string) (string, error) {
	reqBody := map[string]interface{}{
		"model":               model,
		"prompt":              prompt,
		"image_size":          "928x1664",
		"batch_size":          1,
		"num_inference_steps": 30,
		"cfg":                 10,
		"image":               imageURL,
	}
	return p.callAPI("https://api.siliconflow.cn/v1/images/generations", reqBody)
}

func (p *SiliconFlowProvider) GenerateVideo(prompt, imageURL, model string) (string, error) {
	reqBody := map[string]interface{}{
		"model":     model,
		"prompt":    prompt,
		"image_url": imageURL,
	}
	return p.callAPI("https://api.siliconflow.cn/v1/video/generations", reqBody)
}

func (p *SiliconFlowProvider) GenerateAudio(input, model string) (string, error) {
	reqBody := map[string]interface{}{
		"model":           model,
		"input":           input,
		"voice":           "fishaudio/fish-speech-1.5:alex",
		"response_format": "mp3",
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.siliconflow.cn/v1/audio/speech", bytes.NewBuffer(jsonData))
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

	fileName := fmt.Sprintf("audio_%d.mp3", time.Now().Unix())
	filePath := filepath.Join(os.TempDir(), fileName)
	if err := ioutil.WriteFile(filePath, body, 0644); err != nil {
		return "", fmt.Errorf("failed to save audio file: %v", err)
	}

	return filePath, nil
}

func (p *SiliconFlowProvider) callAPI(url string, reqBody map[string]interface{}) (string, error) {
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
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if len(result.Images) > 0 && result.Images[0].URL != "" {
		return result.Images[0].URL, nil
	}
	if len(result.Data) > 0 && result.Data[0].URL != "" {
		return result.Data[0].URL, nil
	}

	return "", fmt.Errorf("no URL found in response")
}
