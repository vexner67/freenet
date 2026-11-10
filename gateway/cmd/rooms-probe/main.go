// rooms-probe runs only on loopback, using the real gateway page handler and
// CSP. It is a development harness, not part of the deployed gateway.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func main() {
	web := flag.String("web", "scripts/rooms-probe/web", "temporary website directory")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*web, log); err != nil {
		log.Error("rooms probe", "error", err)
		os.Exit(1)
	}
}

func run(web string, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	site, err := rest.NewServer(rest.Config{WebDir: web, RequestTimeout: 10 * time.Second}, rest.Backends{}, log, prometheus.NewRegistry())
	if err != nil {
		return fmt.Errorf("rest.NewServer: %w", err)
	}
	target, err := url.Parse("http://127.0.0.1:7880")
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/rtc", httputil.NewSingleHostReverseProxy(target))
	mux.Handle("/rtc/", httputil.NewSingleHostReverseProxy(target))
	mux.HandleFunc("POST /probe-token", token)
	mux.Handle("/", site.Handler())
	server := &http.Server{Addr: "127.0.0.1:8090", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Info("rooms probe ready", "url", "http://localhost:8090")
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(closing); err != nil {
		return fmt.Errorf("server.Shutdown: %w", err)
	}
	return nil
}

func token(w http.ResponseWriter, r *http.Request) {
	// These are LiveKit's public --dev credentials, never production keys.
	id, err := uuid.NewV7()
	if err != nil {
		http.Error(w, "identity unavailable", 500)
		return
	}
	claims := map[string]any{"iss": "devkey", "sub": id.String(), "nbf": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(10 * time.Minute).Unix(), "video": map[string]any{"room": "rooms-csp-probe", "roomJoin": true, "canPublish": true, "canSubscribe": true, "canPublishData": false}}
	payload, err := json.Marshal(claims)
	if err != nil {
		http.Error(w, "token unavailable", 500)
		return
	}
	enc := base64.RawURLEncoding.EncodeToString
	unsigned := enc([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc(payload)
	mac := hmac.New(sha256.New, []byte("secret"))
	if _, err := mac.Write([]byte(unsigned)); err != nil {
		http.Error(w, "signature unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(map[string]string{"token": unsigned + "." + enc(mac.Sum(nil))}); err != nil {
		return
	}
}
