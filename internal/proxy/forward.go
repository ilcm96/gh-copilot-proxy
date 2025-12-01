package proxy

import (
	"bytes"
	"encoding/json"
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
	if hasVisionContent(body) {
		req.Header.Set("Copilot-Vision-Request", "true")
	}

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

// hasVisionContent checks for image content in an OpenAI-style request body.
// hasVisionContent 는 이미지 콘텐츠가 OpenAI-style 요청 본문에 포함되어 있는지 검사합니다.
func hasVisionContent(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		return false
	}
	// messages[] -> content[] items with image_url
	msgs, ok := payload["messages"].([]any)
	if !ok {
		return false
	}
	for _, raw := range msgs {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		// content may be []any or a string
		switch content := m["content"].(type) {
		case []any:
			for _, part := range content {
				pm, ok := part.(map[string]any)
				if !ok {
					continue
				}
				// check both 'image_url' key and 'type' == 'image_url'
				if _, has := pm["image_url"]; has {
					return true
				}
				if t, ok := pm["type"].(string); ok && strings.EqualFold(t, "image_url") {
					return true
				}
			}
		}
	}
	return false
}
