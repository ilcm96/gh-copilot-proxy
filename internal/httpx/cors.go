package httpx

import "net/http"

// WithCORS adds basic CORS headers and handles OPTIONS preflight requests.
// WithCORS 는 단순 CORS 허용 헤더를 추가하고 OPTIONS 사전 요청을 처리합니다.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Access-Control-Allow-Origin", "*")
		headers.Set("Access-Control-Allow-Credentials", "true")
		headers.Set("Access-Control-Allow-Methods", "*")
		headers.Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
