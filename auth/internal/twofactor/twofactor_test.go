package twofactor_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/twofactor"
)

// rfcSecret is the SHA-1 key of RFC 6238, appendix B.
var rfcSecret = twofactor.Secret("12345678901234567890")

func TestCodeRFC6238(t *testing.T) {
	t.Parallel()

	// The vectors of the RFC, cut to six digits.
	tests := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}

	for _, tt := range tests {
		if got := rfcSecret.Code(twofactor.Step(time.Unix(tt.unix, 0))); got != tt.want {
			t.Fatalf("Code at %d = %s, want %s", tt.unix, got, tt.want)
		}
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	now := time.Unix(1234567890, 0)
	step := twofactor.Step(now)

	tests := []struct {
		name  string
		code  string
		after int64
		want  int64
		err   error
	}{
		{name: "now", code: rfcSecret.Code(step), want: step},
		{name: "spaced", code: " 005 924 ", want: step},
		{name: "before", code: rfcSecret.Code(step - 1), want: step - 1},
		{name: "after", code: rfcSecret.Code(step + 1), want: step + 1},
		{name: "too old", code: rfcSecret.Code(step - 2), err: twofactor.ErrInvalidCode},
		{name: "too new", code: rfcSecret.Code(step + 2), err: twofactor.ErrInvalidCode},
		{name: "used", code: rfcSecret.Code(step), after: step, err: twofactor.ErrInvalidCode},
		{name: "short", code: "12345", err: twofactor.ErrInvalidCode},
		{name: "empty", code: "", err: twofactor.ErrInvalidCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := rfcSecret.Match(tt.code, now, tt.after)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("Match(%q) = %d, %v", tt.code, got, err)
			}
		})
	}
}

func TestSecret(t *testing.T) {
	t.Parallel()

	s, err := twofactor.NewSecret()
	if err != nil || len(s) != 20 {
		t.Fatalf("NewSecret = %x, %v", s, err)
	}

	if _, err = twofactor.RestoreSecret(s[:10]); !errors.Is(err, twofactor.ErrSecretLen) {
		t.Fatalf("RestoreSecret short: %v", err)
	}

	if got := rfcSecret.String(); got != "GEZD GNBV GY3T QOJQ GEZD GNBV GY3T QOJQ" {
		t.Fatalf("String = %q", got)
	}

	uri := rfcSecret.URI("Freenet", "ann")
	if uri != "otpauth://totp/Freenet:ann?algorithm=SHA1&digits=6&issuer=Freenet&period=30&secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" {
		t.Fatalf("URI = %q", uri)
	}
}

func TestFactor(t *testing.T) {
	t.Parallel()

	user := uuid.New()
	now := time.Unix(1234567890, 0)

	f := twofactor.New(user, rfcSecret)
	if f.UserID() != user || f.Enabled() || f.LastStep() != 0 || f.RecoveryLeft() != 0 {
		t.Fatalf("New = %+v", f)
	}
	if step, err := f.Match("005924", now); err != nil || step != twofactor.Step(now) {
		t.Fatalf("Match = %d, %v", step, err)
	}

	used := twofactor.Restore(user, rfcSecret, true, twofactor.Step(now), 3)
	if _, err := used.Match("005924", now); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Match of a used code: %v", err)
	}
	if !used.Enabled() || used.RecoveryLeft() != 3 {
		t.Fatalf("Restore = %+v", used)
	}
}

func TestRecoveryCodes(t *testing.T) {
	t.Parallel()

	codes, err := twofactor.NewRecoveryCodes()
	if err != nil || len(codes) != twofactor.RecoveryCodes {
		t.Fatalf("NewRecoveryCodes = %v, %v", codes, err)
	}

	seen := map[string]bool{}
	for _, c := range codes {
		plain := twofactor.Normalize(c)
		if len(c) != 11 || c[5] != '-' || !twofactor.IsRecoveryCode(plain) || seen[plain] {
			t.Fatalf("recovery code %q", c)
		}
		seen[plain] = true

		if !twofactor.IsRecoveryCode(twofactor.Normalize(strings.ToUpper(c))) {
			t.Fatalf("upper case %q", c)
		}
	}

	for _, c := range []string{"005924", "abcde-fghi", "abcde-fgh0l", ""} {
		if twofactor.IsRecoveryCode(twofactor.Normalize(c)) {
			t.Fatalf("IsRecoveryCode(%q)", c)
		}
	}
}
