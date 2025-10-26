package proxy

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/ilcm96/gh-copilot-proxy/internal/adapter"
)

// proxyHandler creates an HTTP handler that performs a simple proxy.
// proxyHandler 는 단순 프록시를 수행하는 HTTP 핸들러를 생성합니다.
func (s *ProxyServer) proxyHandler(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.forward(w, r, target, nil); err != nil {
			log.Printf("proxy error: %v", err)
			http.Error(w, "proxy error", http.StatusBadGateway)
		}
	}
}

// messagesHandler creates a handler for the Anthropic-compatible messages endpoint.
// messagesHandler 는 Anthropic 호환 메시지 엔드포인트를 처리하는 핸들러를 생성합니다.
func (s *ProxyServer) messagesHandler() http.HandlerFunc {
	target := "https://api.githubcopilot.com/chat/completions"
	opts := &ProxyOptions{
		TransformRequest: func(body []byte) ([]byte, error) {
			if len(body) == 0 {
				return body, nil
			}
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				return nil, err
			}
			converted := adapter.ConvertRequestAnthropicToOpenAI(payload)
			return json.Marshal(converted)
		},
		TransformResponse: func(w http.ResponseWriter, resp *http.Response) error {
			return adapter.TransformOpenAIResponseToAnthropic(w, resp)
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.forward(w, r, target, opts); err != nil {
			log.Printf("messages proxy error: %v", err)
			http.Error(w, "proxy error", http.StatusBadGateway)
		}
	}
}
