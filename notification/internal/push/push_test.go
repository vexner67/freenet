package push_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/push"
)

func TestNewEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in string
		ok bool
	}{
		{"https://fcm.googleapis.com/fcm/send/abc", true},
		{"  https://web.push.apple.com/QK  ", true},
		{"http://push.example/1", false},
		{"https://user:pw@push.example/1", false},
		{"https:///nohost", false},
		{"", false},
		{"https://push.example/" + strings.Repeat("a", 2048), false},
	}

	for _, tt := range tests {
		e, err := push.NewEndpoint(tt.in)
		if (err == nil) != tt.ok {
			t.Errorf("NewEndpoint(%q) = %v", tt.in, err)
		}
		if tt.ok && e.Origin() != "https://"+strings.Split(strings.TrimPrefix(strings.TrimSpace(tt.in), "https://"), "/")[0] {
			t.Errorf("Origin of %q = %q", tt.in, e.Origin())
		}
	}
}

func TestNewKeys(t *testing.T) {
	t.Parallel()

	const (
		key  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
		auth = "BTBZMqHH6r4Tts7J_aSIgg"
	)

	if k, err := push.NewKeys(key+"=", auth+"=="); err != nil || len(k.P256DH) != 65 || len(k.Auth) != 16 {
		t.Fatalf("padded keys: %v", err)
	}

	bad := [][2]string{{auth, auth}, {key, key}, {"not base64!", auth}, {"A" + key[1:], auth}, {"B" + strings.Repeat("A", 86), auth}}
	for _, b := range bad {
		if _, err := push.NewKeys(b[0], b[1]); !errors.Is(err, push.ErrKeysInvalid) {
			t.Errorf("NewKeys(%q, %q) = %v", b[0], b[1], err)
		}
	}
}

func TestNewSubscription(t *testing.T) {
	t.Parallel()

	keys := push.Keys{P256DH: make([]byte, 65), Auth: make([]byte, 16)}

	if _, err := push.NewSubscription(uuid.Nil, "https://push.example/1", keys, time.Now()); !errors.Is(err, push.ErrNoUser) {
		t.Fatalf("no user: %v", err)
	}

	if _, err := push.NewSubscription(uuid.New(), "https://push.example/1", push.Keys{}, time.Now()); !errors.Is(err, push.ErrKeysInvalid) {
		t.Fatalf("no keys: %v", err)
	}
}

func TestMessage_Validate(t *testing.T) {
	t.Parallel()

	many := make([]uuid.UUID, push.MaxRecipients+1)
	for i := range many {
		many[i] = uuid.New()
	}

	tests := []struct {
		m    push.Message
		want error
	}{
		{push.Message{SenderID: uuid.New(), RecipientIDs: many[:1]}, nil},
		{push.Message{RecipientIDs: many[:1]}, push.ErrNoUser},
		{push.Message{SenderID: uuid.New()}, push.ErrNoRecipients},
		{push.Message{SenderID: uuid.New(), RecipientIDs: many}, push.ErrTooManyRecipients},
	}

	for _, tt := range tests {
		if err := tt.m.Validate(); !errors.Is(err, tt.want) {
			t.Errorf("Validate() = %v, want %v", err, tt.want)
		}
	}
}
