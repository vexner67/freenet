// Package twofactor is the second step of signing in: a code of an
// authenticator app (TOTP, RFC 6238, as Google Authenticator, Aegis and
// the others make them) or a recovery code for when the phone is away.
package twofactor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 and every authenticator app use HMAC-SHA1
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// secretLen is 160 bits, the length RFC 4226 recommends.
	secretLen = 20
	// Period and Digits are what every app assumes when the URI is silent.
	Period = 30 * time.Second
	Digits = 6
	// skew accepts the code of the step before and after: clocks of
	// phones drift and the person needs a moment to type.
	skew = 1
)

var (
	// ErrInvalidCode is a wrong, stale or reused code of either kind.
	ErrInvalidCode = errors.New("invalid two-factor code")
	ErrNotEnabled  = errors.New("two-factor authentication is not enabled")
	ErrEnabled     = errors.New("two-factor authentication is already enabled")
	ErrNotSetUp    = errors.New("two-factor authentication is not set up")
	ErrSecretLen   = errors.New("two-factor secret has a wrong length")
)

// base32 without padding, as authenticator apps take the key.
var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Secret is the key the server and the app share.
type Secret []byte

// NewSecret is a random secret.
func NewSecret() (Secret, error) {
	s := make([]byte, secretLen)
	if _, err := rand.Read(s); err != nil {
		return nil, fmt.Errorf("rand.Read: %w", err)
	}

	return s, nil
}

// RestoreSecret is a secret read from storage.
func RestoreSecret(b []byte) (Secret, error) {
	if len(b) != secretLen {
		return nil, ErrSecretLen
	}

	return Secret(b), nil
}

// String is the key typed into an app by hand, in groups of four.
func (s Secret) String() string {
	key := encoding.EncodeToString(s)

	var b strings.Builder
	for i, r := range key {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}

	return b.String()
}

// URI is the otpauth:// address of the QR code that apps scan; account
// names the page in the app's list under the issuer.
func (s Secret) URI(issuer, account string) string {
	q := url.Values{}
	q.Set("secret", encoding.EncodeToString(s))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period.Seconds())))

	label := url.PathEscape(issuer + ":" + account)

	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Step is the number of the period t falls in.
func Step(t time.Time) int64 {
	return t.Unix() / int64(Period.Seconds())
}

// Code is the code of the secret for step, RFC 4226 truncated to Digits.
func (s Secret) Code(step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // steps since 1970 are positive

	mac := hmac.New(sha1.New, s)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%0*d", Digits, value%1_000_000)
}

// Match finds the step of code around now that is later than after, the
// step of the last code accepted: a code works once.
func (s Secret) Match(code string, now time.Time, after int64) (int64, error) {
	code = Normalize(code)
	if len(code) != Digits {
		return 0, ErrInvalidCode
	}

	current := Step(now)
	for step := current - skew; step <= current+skew; step++ {
		if step > after && subtle.ConstantTimeCompare([]byte(s.Code(step)), []byte(code)) == 1 {
			return step, nil
		}
	}

	return 0, ErrInvalidCode
}

// Normalize drops the spaces and dashes people type into codes and makes
// recovery codes lower case.
func Normalize(code string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(strings.TrimSpace(code)))
}

// Factor is a page's second step: an app's secret, waiting for its first
// code until it is enabled.
type Factor struct {
	userID   uuid.UUID
	secret   Secret
	enabled  bool
	lastStep int64
	recovery int
}

// New is a factor set up and not yet enabled.
func New(userID uuid.UUID, secret Secret) *Factor {
	return &Factor{userID: userID, secret: secret}
}

// Restore rebuilds a factor from storage; recovery is how many recovery
// codes are left.
func Restore(userID uuid.UUID, secret Secret, enabled bool, lastStep int64, recovery int) *Factor {
	return &Factor{userID: userID, secret: secret, enabled: enabled, lastStep: lastStep, recovery: recovery}
}

func (f *Factor) UserID() uuid.UUID {
	return f.userID
}

func (f *Factor) Secret() Secret {
	return f.secret
}

func (f *Factor) Enabled() bool {
	return f.enabled
}

// LastStep is the step of the last code accepted, 0 before any.
func (f *Factor) LastStep() int64 {
	return f.lastStep
}

// RecoveryLeft is how many recovery codes are still unused.
func (f *Factor) RecoveryLeft() int {
	return f.recovery
}

// Match is the step of a code of the app not used before.
func (f *Factor) Match(code string, now time.Time) (int64, error) {
	return f.secret.Match(code, now, f.lastStep)
}
