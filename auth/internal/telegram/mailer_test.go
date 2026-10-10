package telegram_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vexner67/freenet/auth/internal/telegram"
)

const token = "123456:secret-token"

func TestMailer_SendCode(t *testing.T) {
	t.Parallel()

	var path, chat, text string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, chat, text = r.URL.Path, r.PostFormValue("chat_id"), r.PostFormValue("text")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(srv.Close)

	m := telegram.NewMailer(srv.URL+"/", token, "-100500")
	if err := m.SendCode(context.Background(), "ann@example.com", "123456"); err != nil {
		t.Fatalf("SendCode: %v", err)
	}

	if path != "/bot"+token+"/sendMessage" || chat != "-100500" {
		t.Fatalf("sent to %q, chat %q", path, chat)
	}
	if !strings.Contains(text, "ann@example.com") || !strings.Contains(text, "123456") {
		t.Fatalf("the message = %q", text)
	}
}

func TestMailer_SendCode_Failures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"telegram refuses", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
		}},
		{"ok is false", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
		}},
		{"not an answer of the API", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>gateway</html>`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			t.Cleanup(srv.Close)

			err := telegram.NewMailer(srv.URL, token, "1").SendCode(context.Background(), "ann@example.com", "123456")
			if !errors.Is(err, telegram.ErrRejected) {
				t.Fatalf("SendCode = %v, want ErrRejected", err)
			}
			if strings.Contains(err.Error(), "123456") {
				t.Fatalf("the error tells the code or the token: %v", err)
			}
		})
	}
}

// An error of the network must not carry the URL, which has the token.
func TestMailer_SendCode_HidesToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()

	err := telegram.NewMailer(srv.URL, token, "1").SendCode(context.Background(), "ann@example.com", "654321")
	if err == nil {
		t.Fatal("a closed server took the message")
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "654321") {
		t.Fatalf("the error tells a secret: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = telegram.NewMailer("http://127.0.0.1:1", token, "1").SendCode(ctx, "ann@example.com", "654321"); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("a cancelled send = %v", err)
	}
}
