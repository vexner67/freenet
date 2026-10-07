package rest_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type eventJSON struct {
	Type          string         `json:"type"`
	Messages      *int           `json:"messages"`
	Notifications *int           `json:"notifications"`
	Peer          map[string]any `json:"peer"`
	Message       map[string]any `json:"message"`
	ByMe          bool           `json:"byMe"`
	UpTo          string         `json:"upTo"`
	Change        string         `json:"change"`
}

// connect opens the event stream of h and sends the token.
func connect(t *testing.T, url, token string, header http.Header) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http")+"/api/v1/events", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	if err = wsjson.Write(ctx, conn, map[string]string{"token": token}); err != nil {
		t.Fatalf("send the token: %v", err)
	}

	return conn
}

func next(t *testing.T, conn *websocket.Conn) eventJSON {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var e eventJSON
	if err := wsjson.Read(ctx, conn, &e); err != nil {
		t.Fatalf("read an event: %v", err)
	}

	return e
}

func TestEvents(t *testing.T) {
	t.Parallel()

	// A request timeout this short would end the stream if it applied.
	h := newHarness(rest.Config{RequestTimeout: time.Millisecond})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	h.messages.events = make(chan model.MessageEvent, 4)
	h.notifications.changes = make(chan string, 4)

	srv := httptest.NewServer(h.handler)
	t.Cleanup(srv.Close)

	conn := connect(t, srv.URL, "tok", nil)

	if e := next(t, conn); e.Type != "ready" || *e.Messages != 2 || *e.Notifications != 3 {
		t.Fatalf("first event = %+v, want ready with both counters", e)
	}

	// u1 is the caller (GetMe), u2 is bob.
	h.messages.events <- model.MessageEvent{Message: &model.Message{
		ID: "m1", SenderID: "u2", RecipientID: "u1", Text: "привет", CreatedAt: time.Now(),
	}}

	if e := next(t, conn); e.Type != "message" || e.Peer["nickname"] != "bob" || e.Message["text"] != "привет" || e.Message["out"] != false {
		t.Fatalf("message event = %+v", e)
	}
	if e := next(t, conn); e.Type != "counters" || e.Messages == nil || *e.Messages != 2 || e.Notifications != nil {
		t.Fatalf("after a message = %+v, want the messages counter only", e)
	}

	h.messages.events <- model.MessageEvent{Read: &model.ReadMark{ReaderID: "u2", PeerID: "u1", UpTo: "m0"}}

	if e := next(t, conn); e.Type != "read" || e.ByMe || e.Peer["nickname"] != "bob" || e.UpTo != "m0" {
		t.Fatalf("read event = %+v, want bob's read", e)
	}
	next(t, conn) // counters

	h.notifications.changes <- "new"

	if e := next(t, conn); e.Type != "notification" || e.Change != "new" {
		t.Fatalf("notification event = %+v", e)
	}
	if e := next(t, conn); e.Type != "counters" || e.Notifications == nil || *e.Notifications != 3 || e.Messages != nil {
		t.Fatalf("after a notification = %+v, want the notifications counter only", e)
	}
}

func TestEvents_Close(t *testing.T) {
	t.Parallel()

	closeStatus := func(conn *websocket.Conn) websocket.StatusCode {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return websocket.CloseStatus(err)
			}
		}
	}

	t.Run("the token expires: 4001, refresh and connect again", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.messages.events = make(chan model.MessageEvent)
		h.messages.streamEnd = failure.New(failure.Unauthenticated, "access token expired")
		srv := httptest.NewServer(h.handler)
		t.Cleanup(srv.Close)

		conn := connect(t, srv.URL, "tok", nil)
		next(t, conn) // ready
		close(h.messages.events)

		if got := closeStatus(conn); got != 4001 {
			t.Fatalf("close status = %d, want 4001", got)
		}
	})

	t.Run("a bad token: 4001", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		srv := httptest.NewServer(h.handler)
		t.Cleanup(srv.Close)

		if got := closeStatus(connect(t, srv.URL, "expired", nil)); got != 4001 {
			t.Fatalf("close status = %d, want 4001", got)
		}
	})

	t.Run("a service falls behind: 1013, connect again later", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.notifications.changes = make(chan string)
		h.notifications.streamEnd = failure.New(failure.TooManyRequests, "fell behind, subscribe again")
		srv := httptest.NewServer(h.handler)
		t.Cleanup(srv.Close)

		conn := connect(t, srv.URL, "tok", nil)
		next(t, conn)
		close(h.notifications.changes)

		if got := closeStatus(conn); got != websocket.StatusTryAgainLater {
			t.Fatalf("close status = %d, want 1013", got)
		}
	})
}

func TestEvents_Origin(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{AllowedOrigins: []string{"http://localhost:5173"}})
	srv := httptest.NewServer(h.handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/events"

	_, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://evil.example"}}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a foreign origin: %v, %v; want 403", resp, err)
	}

	conn := connect(t, srv.URL, "tok", http.Header{"Origin": {"http://localhost:5173"}})
	if e := next(t, conn); e.Type != "ready" {
		t.Fatalf("an allowed origin: %+v", e)
	}
}

// The server stopping says "going away" to the open streams and waits for
// them, which Shutdown alone does not do for hijacked connections.
func TestEvents_Shutdown(t *testing.T) {
	t.Parallel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()

	h := newHarness(rest.Config{})
	server := rest.NewServer(rest.Config{Addr: addr, RequestTimeout: time.Second}, rest.Backends{
		Auth: h.auth, Users: h.users, Posts: h.posts, Media: h.media, Notifications: h.notifications, Messages: h.messages,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	var conn *websocket.Conn
	for range 50 {
		dialCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, _, err = websocket.Dial(dialCtx, "ws://"+addr+"/api/v1/events", nil)
		cancel()
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err = wsjson.Write(context.Background(), conn, map[string]string{"token": "tok"}); err != nil {
		t.Fatal(err)
	}
	next(t, conn)

	stop()

	_, _, err = conn.Read(context.Background())
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway {
		t.Fatalf("close status = %d (%v), want 1001", got, err)
	}
	if err = <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Run: %v", err)
	}
}
