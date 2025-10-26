package proxy

import (
	"net/http"

	"github.com/ilcm96/gh-copilot-proxy/internal/httpx"
)

// Routes returns the HTTP routing configuration with auth and CORS applied.
// Routes 는 인증과 CORS 가 적용된 HTTP 라우팅 구성을 반환합니다.
func (s *ProxyServer) Routes() http.Handler {
	mux := http.NewServeMux()
	chatHandler := s.withAuth(s.proxyHandler("https://api.githubcopilot.com/chat/completions"))
	embeddingsHandler := s.withAuth(s.proxyHandler("https://api.githubcopilot.com/embeddings"))
	messagesHandler := s.withAuth(s.messagesHandler())

	mux.Handle("/chat/completions", chatHandler)
	mux.Handle("/embeddings", embeddingsHandler)
	mux.Handle("/messages", messagesHandler)

	mux.Handle("/v1/chat/completions", chatHandler)
	mux.Handle("/v1/embeddings", embeddingsHandler)
	mux.Handle("/v1/messages", messagesHandler)

	return httpx.WithCORS(mux)
}
