package tools

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WebSearchTool searches the web using Brave Search API.
type WebSearchTool struct {
	BaseTool
	APIKey     string
	MaxResults int
}

// NewWebSearchTool creates a new WebSearchTool.
func NewWebSearchTool(apiKey string, maxResults int) *WebSearchTool {
	if maxResults <= 0 {
		maxResults = 5
	}
	return &WebSearchTool{
		APIKey:     apiKey,
		MaxResults: maxResults,
	}
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Search the web. Returns titles, URLs, and snippets."
}

func (t *WebSearchTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "Results (1-10)",
				"minimum":     1,
				"maximum":     10,
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(args map[string]interface{}) (string, error) {
	if t.APIKey == "" {
		return "Error: BRAVE_API_KEY not configured", nil
	}

	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query must be a string")
	}

	count := t.MaxResults
	if c, ok := args["count"].(float64); ok {
		count = int(c)
	}

	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	reqURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d", url.QueryEscape(query), count)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", t.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Error: API returned status %d", resp.StatusCode), nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Web.Results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Results for: %s\n", query))
	for i, item := range result.Web.Results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, item.Title, item.URL))
		if item.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", item.Description))
		}
	}

	return sb.String(), nil
}

// WebFetchTool fetches and extracts content from a URL.
type WebFetchTool struct {
	BaseTool
	MaxChars int
}

// NewWebFetchTool creates a new WebFetchTool.
func NewWebFetchTool(maxChars int) *WebFetchTool {
	if maxChars <= 0 {
		maxChars = 50000
	}
	return &WebFetchTool{
		MaxChars: maxChars,
	}
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch URL and extract readable content (HTML -> markdown/text)."
}

func (t *WebFetchTool) ToSchema() map[string]interface{} {
	return GenerateSchema(t)
}

func (t *WebFetchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"extractMode": map[string]interface{}{
				"type":    "string",
				"enum":    []string{"markdown", "text"},
				"default": "markdown",
			},
			"maxChars": map[string]interface{}{
				"type":    "integer",
				"minimum": 100,
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(args map[string]interface{}) (string, error) {
	urlStr, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("url must be a string")
	}

	extractMode := "markdown"
	if m, ok := args["extractMode"].(string); ok {
		extractMode = m
	}

	maxChars := t.MaxChars
	if m, ok := args["maxChars"].(float64); ok {
		maxChars = int(m)
	}

	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return jsonError(fmt.Sprintf("URL validation failed: %s", urlStr), urlStr)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return jsonError(err.Error(), urlStr)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return jsonError(err.Error(), urlStr)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return jsonError(err.Error(), urlStr)
	}

	contentType := resp.Header.Get("Content-Type")
	var text, extractor string

	if strings.Contains(contentType, "application/json") {
		text = string(bodyBytes)
		extractor = "json"
	} else if strings.Contains(contentType, "text/html") {
		// Simple HTML processing
		htmlContent := string(bodyBytes)
		if extractMode == "markdown" {
			text = toMarkdown(htmlContent)
		} else {
			text = stripTags(htmlContent)
		}
		extractor = "simple-html"
	} else {
		text = string(bodyBytes)
		extractor = "raw"
	}

	truncated := false
	if len(text) > maxChars {
		text = text[:maxChars]
		truncated = true
	}

	result := map[string]interface{}{
		"url":       urlStr,
		"finalUrl":  resp.Request.URL.String(),
		"status":    resp.StatusCode,
		"extractor": extractor,
		"truncated": truncated,
		"length":    len(text),
		"text":      text,
	}

	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func jsonError(msg, url string) (string, error) {
	res := map[string]string{"error": msg, "url": url}
	b, _ := json.Marshal(res)
	return string(b), nil
}

// Regex compilation
var (
	reScript = regexp.MustCompile(`(?i)<script[\s\S]*?</script>`)
	reStyle  = regexp.MustCompile(`(?i)<style[\s\S]*?</style>`)
	reTags   = regexp.MustCompile(`<[^>]+>`)
	reSpace  = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
	reLink   = regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
	reList   = regexp.MustCompile(`(?i)<li[^>]*>([\s\S]*?)</li>`)
	reBlock  = regexp.MustCompile(`(?i)</(p|div|section|article)>`)
	reBreak  = regexp.MustCompile(`(?i)<(br|hr)\s*/?>`)
)

func stripTags(text string) string {
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	text = reTags.ReplaceAllString(text, "")
	// Unescape handled by caller or just left as is for now, 
	// or we can use html.UnescapeString but need "html" package
	return normalize(text)
}

func normalize(text string) string {
	text = reSpace.ReplaceAllString(text, " ")
	text = reNewlines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func toMarkdown(html string) string {
	// Convert links
	html = reLink.ReplaceAllStringFunc(html, func(s string) string {
		matches := reLink.FindStringSubmatch(s)
		if len(matches) == 3 {
			return fmt.Sprintf("[%s](%s)", stripTags(matches[2]), matches[1])
		}
		return s
	})

	// Headings
	for i := 1; i <= 6; i++ {
		reHeader := regexp.MustCompile(fmt.Sprintf(`(?i)<h%d[^>]*>([\s\S]*?)</h%d>`, i, i))
		html = reHeader.ReplaceAllStringFunc(html, func(s string) string {
			matches := reHeader.FindStringSubmatch(s)
			if len(matches) == 2 {
				hashes := strings.Repeat("#", i)
				return fmt.Sprintf("\n%s %s\n", hashes, stripTags(matches[1]))
			}
			return s
		})
	}

	// Lists
	html = reList.ReplaceAllStringFunc(html, func(s string) string {
		matches := reList.FindStringSubmatch(s)
		if len(matches) == 2 {
			return fmt.Sprintf("\n- %s", stripTags(matches[1]))
		}
		return s
	})

	// Blocks
	html = reBlock.ReplaceAllString(html, "\n\n")
	html = reBreak.ReplaceAllString(html, "\n")

	return normalize(stripTags(html))
}
