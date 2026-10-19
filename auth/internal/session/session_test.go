package session_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/session"
)

func TestNewDevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ip, agent string
		wantIP    string
		wantAgent string
	}{
		{"known", "203.0.113.7", "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0", "203.0.113.7", "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"},
		{"nothing known", "", "", "", ""},
		{"not an address", "somewhere", " curl/8 ", "", "curl/8"},
		{"IPv6", " 2001:DB8::1 ", "x", "2001:db8::1", "x"},
		{"IPv4 in IPv6", "::ffff:203.0.113.7", "x", "203.0.113.7", "x"},
		{"a name of many lines", "", "one\r\ntwo\tthree", "", "one  two three"},
		{"a long name", "", strings.Repeat("я", 300), "", strings.Repeat("я", 255)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := session.NewDevice(tt.ip, tt.agent)
			if d.IP() != tt.wantIP || d.Agent() != tt.wantAgent {
				t.Fatalf("NewDevice(%q, %q) = %q, %q; want %q, %q", tt.ip, tt.agent, d.IP(), d.Agent(), tt.wantIP, tt.wantAgent)
			}
		})
	}
}

func TestSession_Rotated(t *testing.T) {
	t.Parallel()

	signedIn := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	phone, laptop := session.NewDevice("203.0.113.7", "phone"), session.NewDevice("198.51.100.2", "laptop")

	s, err := session.New(uuid.New(), uuid.New(), "hash-1", phone, signedIn)
	if err != nil {
		t.Fatal(err)
	}
	if !s.UsedAt().Equal(signedIn) || s.Device() != phone {
		t.Fatalf("a new session is used at its sign-in from its device: %v, %v", s.UsedAt(), s.Device())
	}

	later := signedIn.Add(time.Hour)
	r := s.Rotated("hash-2", laptop, later)

	if r.ID() != s.ID() || r.UserID() != s.UserID() || !r.CreatedAt().Equal(signedIn) {
		t.Fatalf("a rotated session is the same session: %+v", r)
	}
	if r.RefreshTokenHash() != "hash-2" || r.Device() != laptop || !r.UsedAt().Equal(later) {
		t.Fatalf("rotated = %q, %v, %v", r.RefreshTokenHash(), r.Device(), r.UsedAt())
	}
	if r.PreviousTokenHash() != "hash-1" || s.PreviousTokenHash() != "" {
		t.Fatalf("previous = %q, of a new session %q", r.PreviousTokenHash(), s.PreviousTokenHash())
	}
	if s.RefreshTokenHash() != "hash-1" || s.Device() != phone {
		t.Fatal("Rotated changed the session it was called on")
	}

	// The client never got hash-2 and refreshed with hash-1 again.
	again := later.Add(time.Second)
	re := r.Reissued("hash-3", phone, again)
	if re.RefreshTokenHash() != "hash-3" || re.PreviousTokenHash() != "hash-1" || re.Device() != phone || !re.UsedAt().Equal(again) {
		t.Fatalf("reissued = %q, %q, %v, %v", re.RefreshTokenHash(), re.PreviousTokenHash(), re.Device(), re.UsedAt())
	}
	if r.RefreshTokenHash() != "hash-2" {
		t.Fatal("Reissued changed the session it was called on")
	}

	if since := session.LiveSince(later, 30*time.Minute); !since.After(signedIn) || !since.Before(later) {
		t.Fatalf("LiveSince = %v", since)
	}
}
