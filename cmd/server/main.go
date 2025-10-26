package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/ilcm96/gh-copilot-proxy/internal/auth"
	"github.com/ilcm96/gh-copilot-proxy/internal/proxy"
)

// generateAccessToken creates a server token using cryptographically secure random bytes.
// generateAccessToken 는 안전한 임의 바이트를 사용해 서버용 토큰을 생성합니다.
func generateAccessToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// main initializes and runs the Copilot proxy server.
// main 는 Copilot 프록시 서버를 초기화하고 실행합니다.
func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authenticator, err := auth.NewCopilotAuth(ctx)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}
	if err := authenticator.Setup(); err != nil {
		log.Fatalf("auth setup: %v", err)
	}
	defer authenticator.Cleanup()

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = generateAccessToken()
		_ = os.Setenv("API_KEY", apiKey)
	}
	log.Printf("API key: %s", apiKey)

	srv := proxy.NewProxyServer(authenticator, apiKey)

	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}
	addr := ":" + port

	baseHandler := srv.Routes()
	server := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(baseHandler, &http2.Server{}),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Printf("Listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
