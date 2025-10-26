package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const copilotAuthURL = "https://api.github.com/copilot_internal/v2/token"

// CopilotAuth manages GitHub Copilot credentials.
// CopilotAuth 는 GitHub Copilot 자격 증명을 관리합니다.
type CopilotAuth struct {
	oauthToken string

	mu          sync.RWMutex
	githubToken map[string]any

	configDir string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCopilotAuth creates a CopilotAuth that manages GitHub Copilot credentials.
// NewCopilotAuth 는 GitHub Copilot 자격 증명을 관리할 CopilotAuth 를 생성합니다.
func NewCopilotAuth(parent context.Context) (*CopilotAuth, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	var configDir string
	if runtime.GOOS == "windows" {
		configDir = filepath.Join(home, "AppData", "Local")
	} else {
		configDir = filepath.Join(home, ".config")
	}
	ctx, cancel := context.WithCancel(parent)
	return &CopilotAuth{
		configDir: configDir,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

// Setup prepares the OAuth token and starts periodic refresh work.
// Setup 는 OAuth 토큰을 준비하고 주기적인 갱신 작업을 시작합니다.
func (a *CopilotAuth) Setup() error {
	oauth, err := a.getOAuthToken()
	if err != nil {
		return err
	}
	a.oauthToken = oauth

	if ok, err := a.RefreshToken(true); err != nil {
		return err
	} else if !ok {
		return errors.New("failed to refresh token during startup")
	}

	a.wg.Add(1)
	go a.runRefreshTimer()
	return nil
}

// Cleanup tears down any running background tasks.
// Cleanup 는 실행 중인 백그라운드 작업을 정리합니다.
func (a *CopilotAuth) Cleanup() {
	a.cancel()
	a.wg.Wait()
}

// BearerToken returns the Copilot API token currently stored in memory.
// BearerToken 는 현재 메모리에 저장된 Copilot API 토큰을 반환합니다.
func (a *CopilotAuth) BearerToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.githubToken == nil {
		return ""
	}
	if token, ok := a.githubToken["token"].(string); ok {
		return token
	}
	return ""
}

// RefreshToken fetches a new Copilot token from GitHub when necessary.
// RefreshToken 는 필요 시 GitHub API 에 요청하여 새 Copilot 토큰을 가져옵니다.
func (a *CopilotAuth) RefreshToken(force bool) (bool, error) {
	if !force && a.isTokenValid() {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotAuthURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s", a.oauthToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Plugin-Version", "copilot.lua")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("token refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return false, fmt.Errorf("decode token response: %w", err)
	}

	a.mu.Lock()
	a.githubToken = token
	a.mu.Unlock()

	log.Printf("token refreshed successfully")
	return true, nil
}
