package service_test

import (
	"context"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/inmem"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/twofactor"
	"github.com/vexner67/freenet/pkg/hmac"
	"github.com/vexner67/freenet/pkg/ratelimit"
)

// app is the authenticator app of a test: it reads the key typed by hand.
type app struct {
	secret twofactor.Secret
}

func newApp(t *testing.T, setup service.TwoFactorSetup) app {
	t.Helper()

	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ReplaceAll(setup.Secret, " ", ""))
	if err != nil {
		t.Fatalf("secret %q: %v", setup.Secret, err)
	}

	return app{secret: raw}
}

// code is the code of the app shift periods from now.
func (a app) code(shift int64) string {
	return a.secret.Code(twofactor.Step(time.Now()) + shift)
}

// wrong is a code that the app gives for none of the steps around now.
func (a app) wrong() string {
	for n := 0; ; n++ {
		code := fmt.Sprintf("%06d", n)
		if code != a.code(-1) && code != a.code(0) && code != a.code(1) && code != a.code(2) {
			return code
		}
	}
}

// enable sets up and turns on the second step of user and returns the app
// and the recovery codes.
func enable(t *testing.T, svc *service.TwoFactorService, user uuid.UUID) (app, []string) {
	t.Helper()

	ctx := context.Background()

	setup, err := svc.SetUp(ctx, user, "ann")
	if err != nil {
		t.Fatalf("SetUp: %v", err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/Freenet:ann?") {
		t.Fatalf("URI = %q", setup.URI)
	}

	a := newApp(t, setup)

	recovery, err := svc.Enable(ctx, user, a.code(0))
	if err != nil || len(recovery) != twofactor.RecoveryCodes {
		t.Fatalf("Enable = %v, %v", recovery, err)
	}

	return a, recovery
}

func newTwoFactor(policy service.TwoFactorPolicy) *service.TwoFactorService {
	factors := inmem.NewTwoFactorRepository()

	return service.NewTwoFactorService(factors, factors, hmac.NewHasher([]byte("test-secret")), ratelimit.NewMemory(), policy)
}

func TestTwoFactorService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTwoFactor(testTwoFactorPolicy())
	user := uuid.New()

	if state, err := svc.State(ctx, user); err != nil || state.Enabled {
		t.Fatalf("State of none = %+v, %v", state, err)
	}
	if _, err := svc.Enable(ctx, user, "123456"); !errors.Is(err, twofactor.ErrNotSetUp) {
		t.Fatalf("Enable before SetUp: %v", err)
	}
	if err := svc.Check(ctx, user, "123456"); !errors.Is(err, twofactor.ErrNotEnabled) {
		t.Fatalf("Check before Enable: %v", err)
	}

	// A wrong first code leaves it off.
	setup, err := svc.SetUp(ctx, user, "ann")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Enable(ctx, user, newApp(t, setup).wrong()); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Enable with a wrong code: %v", err)
	}
	if err = svc.Check(ctx, user, newApp(t, setup).code(0)); !errors.Is(err, twofactor.ErrNotEnabled) {
		t.Fatalf("Check of a factor not enabled: %v", err)
	}

	a, recovery := enable(t, svc, user)

	if state, err := svc.State(ctx, user); err != nil || !state.Enabled || state.RecoveryLeft != twofactor.RecoveryCodes {
		t.Fatalf("State = %+v, %v", state, err)
	}
	if _, err = svc.SetUp(ctx, user, "ann"); !errors.Is(err, twofactor.ErrEnabled) {
		t.Fatalf("SetUp of an enabled: %v", err)
	}
	if _, err = svc.Enable(ctx, user, a.code(1)); !errors.Is(err, twofactor.ErrEnabled) {
		t.Fatalf("Enable twice: %v", err)
	}

	// The code that enabled it is spent; the next one works once.
	if err = svc.Check(ctx, user, a.code(0)); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Check of the code of Enable: %v", err)
	}
	if err = svc.Check(ctx, user, a.code(1)); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if err = svc.Check(ctx, user, a.code(1)); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Check twice: %v", err)
	}

	// A recovery code, as typed in any case, works once too.
	if err = svc.Check(ctx, user, strings.ToUpper(recovery[3])); err != nil {
		t.Fatalf("Check of a recovery code: %v", err)
	}
	if err = svc.Check(ctx, user, recovery[3]); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Check of a spent recovery code: %v", err)
	}
	if err = svc.Check(ctx, user, "abcde-fghij"); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Check of a made-up recovery code: %v", err)
	}
	if state, _ := svc.State(ctx, user); state.RecoveryLeft != twofactor.RecoveryCodes-1 {
		t.Fatalf("RecoveryLeft = %d", state.RecoveryLeft)
	}

	if err = svc.Disable(ctx, user, a.wrong()); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Disable with a wrong code: %v", err)
	}
	if err = svc.Disable(ctx, user, recovery[0]); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if state, err := svc.State(ctx, user); err != nil || state.Enabled {
		t.Fatalf("State after Disable = %+v, %v", state, err)
	}
	if err = svc.Disable(ctx, user, recovery[1]); !errors.Is(err, twofactor.ErrNotEnabled) {
		t.Fatalf("Disable twice: %v", err)
	}
}

func TestTwoFactorService_RenewRecovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTwoFactor(testTwoFactorPolicy())
	user := uuid.New()

	if _, err := svc.RenewRecovery(ctx, user, "123456"); !errors.Is(err, twofactor.ErrNotEnabled) {
		t.Fatalf("RenewRecovery of none: %v", err)
	}

	a, old := enable(t, svc, user)

	if _, err := svc.RenewRecovery(ctx, user, old[0]); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("RenewRecovery for a recovery code: %v", err)
	}

	renewed, err := svc.RenewRecovery(ctx, user, a.code(1))
	if err != nil || len(renewed) != twofactor.RecoveryCodes {
		t.Fatalf("RenewRecovery = %v, %v", renewed, err)
	}

	// Only the latest codes work.
	if err = svc.Check(ctx, user, old[1]); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Check of an old recovery code: %v", err)
	}
	if err = svc.Check(ctx, user, renewed[1]); err != nil {
		t.Fatalf("Check of a new recovery code: %v", err)
	}
}

func TestTwoFactorService_Limit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	policy := testTwoFactorPolicy()
	policy.ChecksPerUser = 3
	svc := newTwoFactor(policy)
	user := uuid.New()

	a, _ := enable(t, svc, user)

	// Enable counted one; two wrong guesses, then the right code waits.
	for range 2 {
		if err := svc.Check(ctx, user, a.wrong()); !errors.Is(err, twofactor.ErrInvalidCode) {
			t.Fatalf("Check of a wrong code: %v", err)
		}
	}
	if err := svc.Check(ctx, user, a.code(1)); !errors.Is(err, service.ErrTooManyRequests) {
		t.Fatalf("Check over the limit: %v", err)
	}
}

func TestAuthService_VerifySecondStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t, nil)
	a, recovery := enable(t, f.twoFactor, f.users.id)

	// signInHalf brings the email code and returns the ticket.
	signInHalf := func() string {
		t.Helper()

		signIn, err := f.svc.VerifyCode(ctx, testEmail, f.requestCode(t, testEmail), testDevice)
		if err != nil || signIn.Ticket == "" || signIn.Tokens != (service.TokenPair{}) {
			t.Fatalf("VerifyCode = %+v, %v", signIn, err)
		}

		return signIn.Ticket
	}

	ticket := signInHalf()
	if f.sessions.saved != 0 {
		t.Fatal("a session before the second step")
	}

	if _, err := f.svc.VerifySecondStep(ctx, ticket, a.wrong(), testDevice); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("VerifySecondStep with a wrong code: %v", err)
	}
	if _, err := f.svc.VerifySecondStep(ctx, "made-up", a.code(1), testDevice); !errors.Is(err, service.ErrTicketNotFound) {
		t.Fatalf("VerifySecondStep with a made-up ticket: %v", err)
	}

	pair, err := f.svc.VerifySecondStep(ctx, ticket, a.code(1), testDevice)
	if err != nil || pair.Access == "" || pair.Refresh == "" || f.sessions.saved != 1 {
		t.Fatalf("VerifySecondStep = %+v, %v (saved %d)", pair, err, f.sessions.saved)
	}

	// The ticket is spent with the sign-in.
	if _, err = f.svc.VerifySecondStep(ctx, ticket, recovery[0], testDevice); !errors.Is(err, service.ErrTicketNotFound) {
		t.Fatalf("VerifySecondStep with a spent ticket: %v", err)
	}

	// A recovery code signs in when the phone is away.
	if _, err = f.svc.VerifySecondStep(ctx, signInHalf(), recovery[0], testDevice); err != nil {
		t.Fatalf("VerifySecondStep with a recovery code: %v", err)
	}

	// A ticket survives as many codes as an email code does.
	ticket = signInHalf()
	for range testPolicy().MaxCodeAttempts {
		if _, err = f.svc.VerifySecondStep(ctx, ticket, "abcde-fghij", testDevice); !errors.Is(err, twofactor.ErrInvalidCode) {
			t.Fatalf("VerifySecondStep with a wrong code: %v", err)
		}
	}
	if _, err = f.svc.VerifySecondStep(ctx, ticket, recovery[1], testDevice); !errors.Is(err, service.ErrTooManyAttempts) {
		t.Fatalf("VerifySecondStep over the attempts: %v", err)
	}
	if _, err = f.svc.VerifySecondStep(ctx, ticket, recovery[1], testDevice); !errors.Is(err, service.ErrTicketNotFound) {
		t.Fatalf("VerifySecondStep after the ticket was burned: %v", err)
	}
}
