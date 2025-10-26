package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ilcm96/gh-copilot-proxy/internal/httpx"
)

// ProxyOptions defines request/response transformation hooks during proxying.
// ProxyOptions 는 프록시 과정에서 요청/응답 변환 훅을 정의합니다.
type ProxyOptions struct {
	TransformRequest  func([]byte) ([]byte, error)
	TransformResponse func(http.ResponseWriter, *http.Response) error
}

// forward sends the client request to the Copilot API and writes the response back.
// forward 는 클라이언트 요청을 Copilot API 에 전달하고 응답을 작성합니다.
func (s *ProxyServer) forward(w http.ResponseWriter, r *http.Request, target string, opts *ProxyOptions) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}

	if opts != nil && opts.TransformRequest != nil {
		body, err = opts.TransformRequest(body)
		if err != nil {
			return fmt.Errorf("transform request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}

	for key, values := range r.Header {
		lower := strings.ToLower(key)
		if _, skip := httpx.HopByHopHeaders[lower]; skip {
			continue
		}
		if lower == "host" || lower == "authorization" || lower == "content-length" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	bearer := s.auth.BearerToken()
	if bearer == "" {
		return errors.New("copilot token unavailable")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearer))
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Editor-Version", "Neovim/0.9.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request: %w", err)
	}
	defer resp.Body.Close()

	if opts != nil && opts.TransformResponse != nil {
		return opts.TransformResponse(w, resp)
	}

	httpx.CopyHeaders(w.Header(), resp.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}
