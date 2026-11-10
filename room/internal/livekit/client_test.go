package livekit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/livekit"
)

type clock struct{ now time.Time }

func (c clock) Now() time.Time { return c.now }
func TestTokenAndAdministration(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	calls := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		token, err := jwt.Parse(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), func(*jwt.Token) (any, error) { return []byte("test-secret"), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("test-key"))
		if err != nil || !token.Valid {
			t.Error("invalid admin token", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "CreateRoom") {
			var payload struct {
				Name string `json:"name"`
				Max  int    `json:"max_participants"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Max != 5 || payload.Name == "" {
				t.Error("room limits missing", err)
			}
		}
		if strings.HasSuffix(r.URL.Path, "GetParticipant") {
			_, err := w.Write([]byte(`{"tracks":[{"sid":"audio","type":"AUDIO"},{"sid":"video","type":"VIDEO"}]}`))
			if err != nil {
				t.Error(err)
			}
			return
		}
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := livekit.New(livekit.Config{URL: server.URL, PublicURL: "wss://example.com", APIKey: "test-key", APISecret: "test-secret", Timeout: time.Second, TokenTTL: 10 * time.Minute, EmptyTTL: 5 * time.Minute, MaxMembers: 5}, clock{now})
	rid, uid := uuid.New(), uuid.New()
	media, err := client.Token(context.Background(), rid, uid)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Parse(media.Token, func(*jwt.Token) (any, error) { return []byte("test-secret"), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	claims := token.Claims.(jwt.MapClaims)
	grant := claims["video"].(map[string]any)
	if grant["room"] != rid.String() || grant["roomJoin"] != true || grant["canPublishData"] != false || grant["roomAdmin"] != nil || grant["canPublishSources"].([]any)[0] != "microphone" || claims["sub"] != uid.String() {
		t.Fatal("unsafe room grant", grant)
	}
	if !media.ExpiresAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatal("wrong TTL")
	}
	if err = client.Mute(context.Background(), rid, uid); err != nil {
		t.Fatal(err)
	}
	if err = client.Remove(context.Background(), rid, uid); err != nil {
		t.Fatal(err)
	}
	if err = client.Delete(context.Background(), rid); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 5 {
		t.Fatal(calls)
	}
}
func TestUnavailableAndIdempotentRemove(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer server.Close()
	client := livekit.New(livekit.Config{URL: server.URL, APIKey: "test", APISecret: "test", Timeout: time.Second, TokenTTL: 10 * time.Minute}, clock{time.Now()})
	if _, err := client.Token(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("missing media server accepted")
	}
	if err := client.Remove(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
}

func TestMediaTokensRejectTamperingAndExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	c := livekit.New(livekit.Config{APIKey: "key", APISecret: "secret"}, clock{now})
	id, user := uuid.New(), uuid.New()
	claims := jwt.MapClaims{"iss": "key", "sub": user.String(), "nbf": now.Add(-time.Minute).Unix(), "exp": now.Add(time.Minute).Unix(), "video": map[string]any{"room": id.String(), "roomJoin": true}}
	sign := func(key string) string {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	rid, uid, err := c.Verify(sign("secret"))
	if err != nil || rid != id || uid != user {
		t.Fatal(rid, uid, err)
	}
	if _, _, err = c.Verify(sign("other")); err == nil {
		t.Fatal("forged signature accepted")
	}
	claims["exp"] = now.Add(-time.Minute).Unix()
	if _, _, err = c.Verify(sign("secret")); err == nil {
		t.Fatal("expired media token accepted")
	}
	delete(claims, "exp")
	if _, _, err = c.Verify(sign("secret")); err == nil {
		t.Fatal("token without expiry accepted")
	}
}
