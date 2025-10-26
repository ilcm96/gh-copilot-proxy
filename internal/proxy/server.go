package proxy

import (
	"net/http"

	"github.com/ilcm96/gh-copilot-proxy/internal/auth"
)

// ProxyServer forwards requests to the Copilot API.
// ProxyServer 는 Copilot API 로 요청을 전달합니다.
type ProxyServer struct {
	auth        *auth.CopilotAuth
	accessToken string
	client      *http.Client
}

// NewProxyServer creates a ProxyServer that forwards requests to the Copilot API.
// NewProxyServer 는 Copilot API 로 요청을 전달하는 ProxyServer 를 생성합니다.
func NewProxyServer(authenticator *auth.CopilotAuth, accessToken string) *ProxyServer {
	return &ProxyServer{
		auth:        authenticator,
		accessToken: accessToken,
		client: &http.Client{
			Timeout: 0,
		},
	}
}
