package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GetMediaReader returns a ReadCloser for the media, and its filename.
// The caller is responsible for closing the reader.
func GetMediaReader(pathOrURL string) (io.ReadCloser, string, error) {
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		resp, err := http.Get(pathOrURL)
		if err != nil {
			return nil, "", err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, "", fmt.Errorf("failed to download media: %s", resp.Status)
		}
		
		// Try to get filename from URL
		filename := filepath.Base(pathOrURL)
		// If URL has query parameters, strip them
		if idx := strings.Index(filename, "?"); idx != -1 {
			filename = filename[:idx]
		}
		
		if filename == "" || filename == "." || filename == "/" {
			filename = "downloaded_media"
		}
		return resp.Body, filename, nil
	}

	f, err := os.Open(pathOrURL)
	if err != nil {
		return nil, "", err
	}
	return f, filepath.Base(pathOrURL), nil
}
