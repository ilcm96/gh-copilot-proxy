package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getOAuthToken retrieves the OAuth token from environment variables or GitHub config files.
// getOAuthToken 는 환경 변수 또는 GitHub 설정 파일에서 OAuth 토큰을 검색합니다.
func (a *CopilotAuth) getOAuthToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("COPILOT_OAUTH_TOKEN")); token != "" {
		return token, nil
	}

	files := []string{"apps.json", "hosts.json"}
	for _, name := range files {
		path := filepath.Join(a.configDir, "github-copilot", name)
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		if len(bytes.TrimSpace(data)) == 0 {
			continue
		}
		var hosts map[string]map[string]any
		if err := json.Unmarshal(data, &hosts); err != nil {
			return "", fmt.Errorf("parse %s: %w", path, err)
		}
		for host, entry := range hosts {
			if !strings.Contains(host, "github.com") {
				continue
			}
			if entry == nil {
				continue
			}
			if token, ok := entry["oauth_token"].(string); ok && token != "" {
				return token, nil
			}
		}
	}
	return "", errors.New("GitHub OAuth token not found")
}
