package grpc

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/twofactor"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeTwoFactor takes "123456" and remembers what it was asked.
type fakeTwoFactor struct {
	userID  uuid.UUID
	account string
	enabled bool
}

func (f *fakeTwoFactor) State(_ context.Context, userID uuid.UUID) (service.TwoFactorState, error) {
	f.userID = userID
	return service.TwoFactorState{Enabled: f.enabled, RecoveryLeft: 7}, nil
}

func (f *fakeTwoFactor) SetUp(_ context.Context, userID uuid.UUID, account string) (service.TwoFactorSetup, error) {
	f.userID, f.account = userID, account
	return service.TwoFactorSetup{Secret: "ABCD EFGH", URI: "otpauth://totp/x"}, nil
}

func (f *fakeTwoFactor) check(code string) error {
	if code != "123456" {
		return twofactor.ErrInvalidCode
	}
	return nil
}

func (f *fakeTwoFactor) Enable(_ context.Context, _ uuid.UUID, code string) ([]string, error) {
	if err := f.check(code); err != nil {
		return nil, err
	}
	f.enabled = true
	return []string{"abcde-fghij"}, nil
}

func (f *fakeTwoFactor) Disable(_ context.Context, _ uuid.UUID, code string) error {
	if err := f.check(code); err != nil {
		return err
	}
	f.enabled = false
	return nil
}

func (f *fakeTwoFactor) RenewRecovery(_ context.Context, _ uuid.UUID, code string) ([]string, error) {
	if err := f.check(code); err != nil {
		return nil, err
	}
	return []string{"kmnpq-rstuv"}, nil
}

func TestAuthHandler_TwoFactor(t *testing.T) {
	t.Parallel()

	ann := uuid.New()
	fake := &fakeTwoFactor{}
	h := NewAuthHandler(nil, nil, fake, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: ann, SessionID: uuid.New()})

	if _, err := h.GetTwoFactor(context.Background(), &authv1.GetTwoFactorRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetTwoFactor without a token: %v", err)
	}

	setup, err := h.SetUpTwoFactor(ctx, &authv1.SetUpTwoFactorRequest{Account: " ann:\n" + strings.Repeat("я", 300)})
	if err != nil || setup.GetSecret() != "ABCD EFGH" || setup.GetUri() != "otpauth://totp/x" || fake.userID != ann {
		t.Fatalf("SetUpTwoFactor = %v, %v", setup, err)
	}
	if fake.account != "ann"+strings.Repeat("я", 251) {
		t.Fatalf("account = %q", fake.account)
	}

	if _, err = h.EnableTwoFactor(ctx, &authv1.EnableTwoFactorRequest{Code: "000000"}); err == nil {
		t.Fatal("EnableTwoFactor with a wrong code succeeded")
	} else if st, ok := toStatus(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("EnableTwoFactor with a wrong code: %v", err)
	}

	enabled, err := h.EnableTwoFactor(ctx, &authv1.EnableTwoFactorRequest{Code: "123456"})
	if err != nil || len(enabled.GetRecoveryCodes()) != 1 {
		t.Fatalf("EnableTwoFactor = %v, %v", enabled, err)
	}

	state, err := h.GetTwoFactor(ctx, &authv1.GetTwoFactorRequest{})
	if err != nil || !state.GetEnabled() || state.GetRecoveryLeft() != 7 {
		t.Fatalf("GetTwoFactor = %v, %v", state, err)
	}

	renewed, err := h.RenewRecoveryCodes(ctx, &authv1.RenewRecoveryCodesRequest{Code: "123456"})
	if err != nil || renewed.GetRecoveryCodes()[0] != "kmnpq-rstuv" {
		t.Fatalf("RenewRecoveryCodes = %v, %v", renewed, err)
	}

	if _, err = h.DisableTwoFactor(ctx, &authv1.DisableTwoFactorRequest{Code: "123456"}); err != nil || fake.enabled {
		t.Fatalf("DisableTwoFactor: %v", err)
	}
}

func TestAccountName(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"ann":         "ann",
		"  ":          "freenet",
		"a:b":         "ab",
		"Анна\tПетро": "АннаПетро",
	} {
		if got := accountName(in); got != want {
			t.Fatalf("accountName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToStatus_TwoFactor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		code codes.Code
	}{
		{twofactor.ErrInvalidCode, codes.InvalidArgument},
		{twofactor.ErrNotSetUp, codes.FailedPrecondition},
		{twofactor.ErrNotEnabled, codes.FailedPrecondition},
		{twofactor.ErrEnabled, codes.FailedPrecondition},
		{service.ErrTicketNotFound, codes.Unauthenticated},
	}

	for _, tt := range tests {
		if st, ok := toStatus(tt.err); !ok || st.Code() != tt.code {
			t.Fatalf("toStatus(%v) = %v", tt.err, st)
		}
	}
}
