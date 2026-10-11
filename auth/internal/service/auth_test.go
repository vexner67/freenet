package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/inmem"
	"github.com/vexner67/freenet/auth/internal/jwt"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/hmac"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/ratelimit"
)

const (
	testEmail account.Email = "ann@example.com"
	testIP    string        = "203.0.113.7"
)

var testKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
})

type fakeUserProvider struct {
	id          uuid.UUID
	deactivated bool
	// gone: the user was erased, Account finds nobody.
	gone  bool
	err   error
	calls int
}

func (f *fakeUserProvider) GetOrCreateByEmail(context.Context, account.Email) (service.Account, error) {
	f.calls++
	return service.Account{UserID: f.id, Deactivated: f.deactivated}, f.err
}

func (f *fakeUserProvider) Account(_ context.Context, id uuid.UUID) (service.Account, error) {
	if f.gone {
		return service.Account{}, service.ErrAccountNotFound
	}
	return service.Account{UserID: id, Deactivated: f.deactivated}, f.err
}

// countingSessions records saves on top of the real in-memory repository.
type countingSessions struct {
	*inmem.SessionRepository
	saved int
}

func (c *countingSessions) Save(ctx context.Context, s *session.Session) error {
	c.saved++
	return c.SessionRepository.Save(ctx, s)
}

// captureMailer remembers the last code sent to each address.
type captureMailer struct {
	codes map[string]string
	sent  int
}

func (m *captureMailer) SendCode(_ context.Context, email, code string) error {
	m.codes[email] = code
	m.sent++
	return nil
}

type fixture struct {
	svc      *service.AuthService
	users    *fakeUserProvider
	sessions *countingSessions
	mailer   *captureMailer
	tokens   *jwt.JWT
	key      *rsa.PrivateKey
}

// testPolicy does not get in the way unless a test tightens it.
// testDevice is where the tests sign in from.
var testDevice = session.NewDevice(testIP, "Mozilla/5.0 (test)")

func testPolicy() service.Policy {
	return service.Policy{
		CodeTTL:            time.Minute,
		MaxCodeAttempts:    5,
		CodeResendInterval: time.Nanosecond,
		CodeRequestsPerIP:  1000,
		CodeRequestsWindow: time.Hour,
	}
}

func newFixture(t *testing.T, userErr error, tune ...func(*service.Policy)) fixture {
	t.Helper()

	policy := testPolicy()
	for _, f := range tune {
		f(&policy)
	}

	key := testKey()
	tokens := jwt.New(jwt.Config{
		PrivateKey: key,
		PublicKey:  &key.PublicKey,
		Issuer:     "freenet-test",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})

	f := fixture{
		users:    &fakeUserProvider{id: uuid.New(), err: userErr},
		sessions: &countingSessions{SessionRepository: inmem.NewSessionRepository()},
		mailer:   &captureMailer{codes: map[string]string{}},
		tokens:   tokens,
		key:      key,
	}

	f.svc = service.NewAuthService(
		f.users,
		f.sessions,
		hmac.NewHasher([]byte("test-secret")),
		inmem.NewCodeRepository(),
		ratelimit.NewMemory(),
		f.mailer,
		tokens,
		policy,
	)

	return f
}

// requestCode asks for a code and returns what the user got by email.
func (f fixture) requestCode(t *testing.T, email account.Email) string {
	t.Helper()

	if err := f.svc.RequestCode(context.Background(), email, testIP); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}

	return f.mailer.codes[email.String()]
}

func (f fixture) signIn(t *testing.T) service.TokenPair {
	t.Helper()

	pair, err := f.svc.VerifyCode(context.Background(), testEmail, f.requestCode(t, testEmail), testDevice)
	if err != nil {
		t.Fatalf("VerifyCode: %v", err)
	}

	return pair
}

func TestAuthService_VerifyCode(t *testing.T) {
	t.Parallel()

	t.Run("valid code opens a session for the user from the provider", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		pair := f.signIn(t)

		if pair.Access == "" || pair.Refresh == "" {
			t.Fatalf("empty token pair: %+v", pair)
		}
		if f.sessions.saved != 1 {
			t.Fatalf("sessions saved = %d, want 1", f.sessions.saved)
		}
	})

	t.Run("code works only once", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		code := f.requestCode(t, testEmail)

		if _, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice); err != nil {
			t.Fatalf("first use: %v", err)
		}

		_, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice)
		if !errors.Is(err, service.ErrCodeNotFound) {
			t.Fatalf("second use: got %v, want ErrCodeNotFound", err)
		}
	})

	t.Run("signing in again after the first time", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		f.signIn(t)
		f.signIn(t)

		if f.sessions.saved != 2 {
			t.Fatalf("sessions saved = %d, want 2", f.sessions.saved)
		}
	})

	t.Run("new code replaces the previous one", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()

		first := f.requestCode(t, testEmail)
		second := f.requestCode(t, testEmail)
		if first == second {
			t.Skip("random codes collided")
		}

		if _, err := f.svc.VerifyCode(ctx, testEmail, first, testDevice); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("old code: got %v, want ErrInvalidCode", err)
		}
		if _, err := f.svc.VerifyCode(ctx, testEmail, second, testDevice); err != nil {
			t.Fatalf("new code: %v", err)
		}
	})

	t.Run("wrong code does not reach the user service or burn the code", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		code := f.requestCode(t, testEmail)

		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}

		if _, err := f.svc.VerifyCode(ctx, testEmail, wrong, testDevice); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("got %v, want ErrInvalidCode", err)
		}
		if f.users.calls != 0 {
			t.Fatalf("user service called %d times", f.users.calls)
		}
		if _, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice); err != nil {
			t.Fatalf("right code after a typo: %v", err)
		}
	})

	t.Run("no code requested", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)

		_, err := f.svc.VerifyCode(context.Background(), testEmail, "123456", testDevice)
		if !errors.Is(err, service.ErrCodeNotFound) {
			t.Fatalf("got %v, want ErrCodeNotFound", err)
		}
	})

	t.Run("user service failure creates no session", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, service.ErrUserServiceUnavailable)

		_, err := f.svc.VerifyCode(context.Background(), testEmail, f.requestCode(t, testEmail), testDevice)
		if !errors.Is(err, service.ErrUserServiceUnavailable) {
			t.Fatalf("got %v, want ErrUserServiceUnavailable", err)
		}
		if f.sessions.saved != 0 {
			t.Fatalf("session saved despite failure")
		}
	})
}

func TestAuthService_RequestCode(t *testing.T) {
	t.Parallel()

	slow := func(p *service.Policy) { p.CodeResendInterval = time.Minute }

	t.Run("one code per email per interval", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, slow)
		ctx := context.Background()

		if err := f.svc.RequestCode(ctx, testEmail, testIP); err != nil {
			t.Fatal(err)
		}

		err := f.svc.RequestCode(ctx, testEmail, "198.51.100.1")
		if !errors.Is(err, service.ErrTooManyRequests) {
			t.Fatalf("got %v, want ErrTooManyRequests", err)
		}
		if f.mailer.sent != 1 {
			t.Fatalf("mails sent = %d, want 1", f.mailer.sent)
		}

		if err = f.svc.RequestCode(ctx, "bob@example.com", testIP); err != nil {
			t.Fatalf("other email must not be limited: %v", err)
		}
	})

	t.Run("limit per address across emails", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, func(p *service.Policy) { p.CodeRequestsPerIP = 2 })
		ctx := context.Background()

		for _, email := range []account.Email{"a@example.com", "b@example.com"} {
			if err := f.svc.RequestCode(ctx, email, testIP); err != nil {
				t.Fatalf("%s: %v", email, err)
			}
		}

		if err := f.svc.RequestCode(ctx, "c@example.com", testIP); !errors.Is(err, service.ErrTooManyRequests) {
			t.Fatalf("got %v, want ErrTooManyRequests", err)
		}
		if err := f.svc.RequestCode(ctx, "c@example.com", "198.51.100.1"); err != nil {
			t.Fatalf("other address must not be limited: %v", err)
		}
	})

	t.Run("unknown address is limited per email only", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, func(p *service.Policy) { p.CodeRequestsPerIP = 1 })
		ctx := context.Background()

		for _, email := range []account.Email{"a@example.com", "b@example.com"} {
			if err := f.svc.RequestCode(ctx, email, ""); err != nil {
				t.Fatalf("%s: %v", email, err)
			}
		}
	})
}

func TestAuthService_CodeAttempts(t *testing.T) {
	t.Parallel()

	three := func(p *service.Policy) { p.MaxCodeAttempts = 3 }

	wrongCode := func(code string) string {
		if code == "000000" {
			return "111111"
		}
		return "000000"
	}

	t.Run("code is burned after the last attempt", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, three)
		ctx := context.Background()
		code := f.requestCode(t, testEmail)

		for i := 0; i < 3; i++ {
			if _, err := f.svc.VerifyCode(ctx, testEmail, wrongCode(code), testDevice); !errors.Is(err, service.ErrInvalidCode) {
				t.Fatalf("attempt %d: got %v, want ErrInvalidCode", i+1, err)
			}
		}

		if _, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice); !errors.Is(err, service.ErrTooManyAttempts) {
			t.Fatalf("over the limit: got %v, want ErrTooManyAttempts", err)
		}
		if _, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice); !errors.Is(err, service.ErrCodeNotFound) {
			t.Fatalf("burned code: got %v, want ErrCodeNotFound", err)
		}
		if f.users.calls != 0 {
			t.Fatal("no attempt over the limit may sign in")
		}
	})

	t.Run("right code on the last attempt", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, three)
		ctx := context.Background()
		code := f.requestCode(t, testEmail)

		for i := 0; i < 2; i++ {
			if _, err := f.svc.VerifyCode(ctx, testEmail, wrongCode(code), testDevice); !errors.Is(err, service.ErrInvalidCode) {
				t.Fatal(err)
			}
		}

		if _, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice); err != nil {
			t.Fatalf("third attempt with the right code: %v", err)
		}
	})

	t.Run("a new code starts with fresh attempts", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, three)
		ctx := context.Background()
		old := f.requestCode(t, testEmail)

		for i := 0; i < 3; i++ {
			_, _ = f.svc.VerifyCode(ctx, testEmail, wrongCode(old), testDevice)
		}

		code := f.requestCode(t, testEmail)
		if _, err := f.svc.VerifyCode(ctx, testEmail, code, testDevice); err != nil {
			t.Fatalf("new code: %v", err)
		}
	})
}

func TestAuthService_RefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("rotates the refresh token", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		first := f.signIn(t)

		second, err := f.svc.RefreshToken(ctx, first.Refresh, testDevice)
		if err != nil {
			t.Fatalf("RefreshToken: %v", err)
		}
		if second.Refresh == first.Refresh || second.Access == first.Access {
			t.Fatal("tokens were not renewed")
		}

		if _, err = f.svc.RefreshToken(ctx, second.Refresh, testDevice); err != nil {
			t.Fatalf("new refresh token must work: %v", err)
		}
	})

	t.Run("reusing an old token ends the session", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		stolen := f.signIn(t)

		current, err := f.svc.RefreshToken(ctx, stolen.Refresh, testDevice)
		if err != nil {
			t.Fatal(err)
		}

		if _, err = f.svc.RefreshToken(ctx, stolen.Refresh, testDevice); !errors.Is(err, service.ErrInvalidRefreshToken) {
			t.Fatalf("reuse: got %v, want ErrInvalidRefreshToken", err)
		}
		if _, err = f.svc.RefreshToken(ctx, current.Refresh, testDevice); !errors.Is(err, service.ErrSessionNotFound) {
			t.Fatalf("after reuse the session must be gone, got %v", err)
		}
	})

	t.Run("access token is not a refresh token", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		pair := f.signIn(t)

		_, err := f.svc.RefreshToken(context.Background(), pair.Access, testDevice)
		if !errors.Is(err, service.ErrInvalidRefreshToken) {
			t.Fatalf("got %v, want ErrInvalidRefreshToken", err)
		}
	})

	t.Run("garbage", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)

		_, err := f.svc.RefreshToken(context.Background(), "garbage", testDevice)
		if !errors.Is(err, service.ErrInvalidRefreshToken) {
			t.Fatalf("got %v, want ErrInvalidRefreshToken", err)
		}
	})
}

func TestAuthService_Logout(t *testing.T) {
	t.Parallel()

	t.Run("ends the session", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		pair := f.signIn(t)

		if err := f.svc.Logout(ctx, pair.Refresh); err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if _, err := f.svc.RefreshToken(ctx, pair.Refresh, testDevice); !errors.Is(err, service.ErrSessionNotFound) {
			t.Fatalf("refresh after logout: got %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		pair := f.signIn(t)

		if err := f.svc.Logout(ctx, pair.Refresh); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.Logout(ctx, pair.Refresh); err != nil {
			t.Fatalf("second logout: %v", err)
		}
	})

	t.Run("does not touch other sessions", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)
		ctx := context.Background()
		phone := f.signIn(t)
		laptop := f.signIn(t)

		if err := f.svc.Logout(ctx, phone.Refresh); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.RefreshToken(ctx, laptop.Refresh, testDevice); err != nil {
			t.Fatalf("other session must survive: %v", err)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)

		err := f.svc.Logout(context.Background(), "garbage")
		if !errors.Is(err, service.ErrInvalidRefreshToken) {
			t.Fatalf("got %v, want ErrInvalidRefreshToken", err)
		}
	})
}

// A deleted page gets access tokens good only to restore it, refreshed
// as the page changes; an erased user's session ends.
func TestAuthService_DeactivatedAccount(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	verifier := jwtauth.NewVerifier(&f.key.PublicKey, "freenet-test")
	ctx := context.Background()

	deactivated := func(access string) bool {
		t.Helper()

		id, err := verifier.Verify(access, jwtauth.AccessToken)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		return id.Deactivated
	}

	pair := f.signIn(t)
	if deactivated(pair.Access) {
		t.Fatal("the token of an active page is marked deleted")
	}

	f.users.deactivated = true
	pair, err := f.svc.RefreshToken(ctx, pair.Refresh, testDevice)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if !deactivated(pair.Access) {
		t.Fatal("refresh after deleting the page kept a full token")
	}

	if again := f.signIn(t); !deactivated(again.Access) {
		t.Fatal("signing in to a deleted page gave a full token")
	}

	f.users.deactivated = false
	if pair, err = f.svc.RefreshToken(ctx, pair.Refresh, testDevice); err != nil || deactivated(pair.Access) {
		t.Fatalf("refresh after restoring: %v, deactivated %v", err, err == nil && deactivated(pair.Access))
	}

	f.users.gone = true
	if _, err = f.svc.RefreshToken(ctx, pair.Refresh, testDevice); !errors.Is(err, service.ErrInvalidRefreshToken) {
		t.Fatalf("refresh of an erased user: %v, want ErrInvalidRefreshToken", err)
	}
	f.users.gone = false
	if _, err = f.svc.RefreshToken(ctx, pair.Refresh, testDevice); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("the session of an erased user is still there: %v", err)
	}
}
