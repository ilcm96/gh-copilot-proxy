package auth

import (
	"encoding/json"
	"log"
	"strconv"
	"time"
)

// runRefreshTimer monitors token expiry and attempts periodic refreshes.
// runRefreshTimer 는 토큰 만료 시간을 감시하며 주기적으로 갱신을 시도합니다.
func (a *CopilotAuth) runRefreshTimer() {
	defer a.wg.Done()
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}
		if _, err := a.RefreshToken(false); err != nil {
			log.Printf("token refresh error: %v", err)
		}

		sleep := time.Minute
		a.mu.RLock()
		if a.githubToken != nil {
			if expires, ok := extractTimestamp(a.githubToken["expires_at"]); ok {
				refreshAt := time.Unix(int64(expires), 0).Add(-2 * time.Minute)
				if refreshAt.Before(time.Now()) {
					sleep = 5 * time.Second
				} else {
					sleep = time.Until(refreshAt)
					if sleep < 5*time.Second {
						sleep = 5 * time.Second
					}
				}
			}
		}
		a.mu.RUnlock()

		select {
		case <-time.After(sleep):
		case <-a.ctx.Done():
			return
		}
	}
}

// isTokenValid checks whether the cached token is still valid.
// isTokenValid 는 현재 캐시된 토큰이 아직 유효한지 검사합니다.
func (a *CopilotAuth) isTokenValid() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.githubToken == nil {
		return false
	}
	expires, ok := extractTimestamp(a.githubToken["expires_at"])
	if !ok {
		return false
	}
	return float64(time.Now().Unix()+120) < expires
}

// extractTimestamp converts various timestamp representations into Unix seconds.
// extractTimestamp 는 다양한 표현의 타임스탬프 값을 유닉스 초 단위로 변환합니다.
func extractTimestamp(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		if err == nil {
			return f, true
		}
	case string:
		if t == "" {
			return 0, false
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f, true
		}
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return float64(parsed.Unix()), true
		}
	}
	return 0, false
}
