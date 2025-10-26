package httpx

import "net/http"

// HopByHopHeaders lists hop-by-hop headers that must be removed when forwarding upstream.
// HopByHopHeaders 는 업스트림 전달 시 제외해야 하는 hop-by-hop 헤더 목록입니다.
var HopByHopHeaders = map[string]struct{}{
	"connection":          {},
	"proxy-connection":    {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
	"http2-settings":      {},
}

// CopyHeaders copies HTTP headers from src to dst.
// CopyHeaders 는 src 의 HTTP 헤더를 dst 에 복사합니다.
func CopyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
