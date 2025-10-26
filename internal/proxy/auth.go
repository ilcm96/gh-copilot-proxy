package proxy

import (
	"net/http"
	"strings"
)

// authorize checks the Authorization header on incoming requests to decide access.
// authorize 는 수신 요청의 Authorization 헤더를 확인해 접근을 허용할지 결정합니다.
func (s *ProxyServer) authorize(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if header == "" {
		header = r.Header.Get("authorization")
	}
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return strings.TrimSpace(parts[1]) == s.accessToken
}

// withAuth returns HTTP middleware that validates the access token.
// withAuth 는 접근 토큰 검증을 수행하는 HTTP 미들웨어를 반환합니다.
func (s *ProxyServer) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !s.authorize(r) {
			http.Error(w, "Invalid access token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
