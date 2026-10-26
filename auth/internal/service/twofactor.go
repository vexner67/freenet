package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/twofactor"
	"github.com/vexner67/freenet/pkg/errs"
)

// TwoFactorRepository keeps the second steps of the pages.
type TwoFactorRepository interface {
	// Get returns twofactor.ErrNotSetUp.
	Get(ctx context.Context, userID uuid.UUID) (*twofactor.Factor, error)
	// SetUp keeps a factor just set up in place of one not enabled;
	// twofactor.ErrEnabled.
	SetUp(ctx context.Context, f *twofactor.Factor) error
	// Enable turns the factor on with the step of its first code and the
	// hashes of the recovery codes; twofactor.ErrInvalidCode when it was
	// enabled meanwhile, twofactor.ErrNotSetUp.
	Enable(ctx context.Context, userID uuid.UUID, step int64, recovery []string) error
	// Delete turns the factor off; twofactor.ErrNotSetUp.
	Delete(ctx context.Context, userID uuid.UUID) error
}

// TwoFactorCodes spends the codes of enabled factors, each once, even when
// two requests bring the same code at once.
type TwoFactorCodes interface {
	// UseStep records the step of a code of the app; twofactor.ErrInvalidCode
	// when it is not later than the last one used.
	UseStep(ctx context.Context, userID uuid.UUID, step int64) error
	// UseRecovery spends the recovery code of the hash;
	// twofactor.ErrInvalidCode when it is not among those left.
	UseRecovery(ctx context.Context, userID uuid.UUID, hash string) error
	// ReplaceRecovery gives new recovery codes, by their hashes, for a code
	// of the app at step; twofactor.ErrInvalidCode as UseStep.
	ReplaceRecovery(ctx context.Context, userID uuid.UUID, step int64, recovery []string) error
}

// TwoFactorPolicy tunes the second step against guessing.
type TwoFactorPolicy struct {
	// Issuer names the site in the authenticator app.
	Issuer string
	// ChecksPerUser codes of a page are checked per ChecksWindow, right
	// or wrong: six digits are guessed in a million tries.
	ChecksPerUser int
	ChecksWindow  time.Duration
}

// TwoFactorSetup is what the app is given: the key and its QR address.
type TwoFactorSetup struct {
	Secret string
	URI    string
}

// TwoFactorState is what the settings show of the second step.
type TwoFactorState struct {
	Enabled      bool
	RecoveryLeft int
}

// TwoFactorService is the second step of signing in: openvk's
// "Двухфакторная аутентификация" with an authenticator app and recovery
// codes.
type TwoFactorService struct {
	factors TwoFactorRepository
	codes   TwoFactorCodes
	hasher  Hasher
	limiter RateLimiter
	policy  TwoFactorPolicy
	now     func() time.Time
}

func NewTwoFactorService(factors TwoFactorRepository, codes TwoFactorCodes, hasher Hasher, limiter RateLimiter, policy TwoFactorPolicy) *TwoFactorService {
	return &TwoFactorService{factors: factors, codes: codes, hasher: hasher, limiter: limiter, policy: policy, now: time.Now}
}

// State is whether the second step of userID is on.
func (s *TwoFactorService) State(ctx context.Context, userID uuid.UUID) (TwoFactorState, error) {
	f, err := s.factors.Get(ctx, userID)
	if errors.Is(err, twofactor.ErrNotSetUp) {
		return TwoFactorState{}, nil
	}
	if err != nil {
		return TwoFactorState{}, errs.Wrap("s.factors.Get", err)
	}

	return TwoFactorState{Enabled: f.Enabled(), RecoveryLeft: f.RecoveryLeft()}, nil
}

// Enabled is whether signing in as userID takes a second step.
func (s *TwoFactorService) Enabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	state, err := s.State(ctx, userID)

	return state.Enabled, errs.Wrap("s.State", err)
}

// SetUp makes a new secret for the app; account names the page in it.
// One set up before and not enabled is replaced; twofactor.ErrEnabled.
func (s *TwoFactorService) SetUp(ctx context.Context, userID uuid.UUID, account string) (TwoFactorSetup, error) {
	secret, err := twofactor.NewSecret()
	if err != nil {
		return TwoFactorSetup{}, errs.Wrap("twofactor.NewSecret", err)
	}

	if err = s.factors.SetUp(ctx, twofactor.New(userID, secret)); err != nil {
		return TwoFactorSetup{}, errs.Wrap("s.factors.SetUp", err)
	}

	return TwoFactorSetup{Secret: secret.String(), URI: secret.URI(s.policy.Issuer, account)}, nil
}

// Enable turns the second step on with the first code of the app and
// returns the recovery codes, shown this once; twofactor.ErrNotSetUp,
// twofactor.ErrEnabled, twofactor.ErrInvalidCode, ErrTooManyRequests.
func (s *TwoFactorService) Enable(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	if err := s.allow(ctx, userID); err != nil {
		return nil, errs.Wrap("s.allow", err)
	}

	f, err := s.factors.Get(ctx, userID)
	if err != nil {
		return nil, errs.Wrap("s.factors.Get", err)
	}

	if f.Enabled() {
		return nil, twofactor.ErrEnabled
	}

	step, err := f.Match(code, s.now())
	if err != nil {
		return nil, errs.Wrap("f.Match", err)
	}

	recovery, hashes, err := s.newRecovery()
	if err != nil {
		return nil, errs.Wrap("s.newRecovery", err)
	}

	if err = s.factors.Enable(ctx, userID, step, hashes); err != nil {
		return nil, errs.Wrap("s.factors.Enable", err)
	}

	return recovery, nil
}

// Disable turns the second step off for a code of either kind;
// twofactor.ErrNotEnabled, twofactor.ErrInvalidCode, ErrTooManyRequests.
func (s *TwoFactorService) Disable(ctx context.Context, userID uuid.UUID, code string) error {
	if err := s.Check(ctx, userID, code); err != nil {
		return errs.Wrap("s.Check", err)
	}

	return errs.Wrap("s.factors.Delete", s.factors.Delete(ctx, userID))
}

// RenewRecovery replaces the recovery codes for a code of the app: only
// the latest ones work, as openvk's; twofactor.ErrNotEnabled,
// twofactor.ErrInvalidCode, ErrTooManyRequests.
func (s *TwoFactorService) RenewRecovery(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	f, err := s.enabled(ctx, userID)
	if err != nil {
		return nil, errs.Wrap("s.enabled", err)
	}

	step, err := f.Match(code, s.now())
	if err != nil {
		return nil, errs.Wrap("f.Match", err)
	}

	recovery, hashes, err := s.newRecovery()
	if err != nil {
		return nil, errs.Wrap("s.newRecovery", err)
	}

	if err = s.codes.ReplaceRecovery(ctx, userID, step, hashes); err != nil {
		return nil, errs.Wrap("s.codes.ReplaceRecovery", err)
	}

	return recovery, nil
}

// Check spends a code of the app or a recovery code of the enabled second
// step of userID; twofactor.ErrNotEnabled, twofactor.ErrInvalidCode,
// ErrTooManyRequests.
func (s *TwoFactorService) Check(ctx context.Context, userID uuid.UUID, code string) error {
	f, err := s.enabled(ctx, userID)
	if err != nil {
		return errs.Wrap("s.enabled", err)
	}

	if plain := twofactor.Normalize(code); twofactor.IsRecoveryCode(plain) {
		return errs.Wrap("s.codes.UseRecovery", s.codes.UseRecovery(ctx, userID, s.hasher.Hash(plain)))
	}

	step, err := f.Match(code, s.now())
	if err != nil {
		return errs.Wrap("f.Match", err)
	}

	return errs.Wrap("s.codes.UseStep", s.codes.UseStep(ctx, userID, step))
}

// enabled is the factor of userID, counting one more check of its codes.
func (s *TwoFactorService) enabled(ctx context.Context, userID uuid.UUID) (*twofactor.Factor, error) {
	if err := s.allow(ctx, userID); err != nil {
		return nil, errs.Wrap("s.allow", err)
	}

	f, err := s.factors.Get(ctx, userID)
	if errors.Is(err, twofactor.ErrNotSetUp) {
		return nil, twofactor.ErrNotEnabled
	}
	if err != nil {
		return nil, errs.Wrap("s.factors.Get", err)
	}

	if !f.Enabled() {
		return nil, twofactor.ErrNotEnabled
	}

	return f, nil
}

func (s *TwoFactorService) allow(ctx context.Context, userID uuid.UUID) error {
	ok, err := s.limiter.Allow(ctx, "two_factor:user:"+userID.String(), s.policy.ChecksPerUser, s.policy.ChecksWindow)
	if err != nil {
		return errs.Wrap("s.limiter.Allow", err)
	}

	if !ok {
		return ErrTooManyRequests
	}

	return nil
}

// newRecovery are fresh recovery codes and the hashes kept of them.
func (s *TwoFactorService) newRecovery() (codes, hashes []string, err error) {
	codes, err = twofactor.NewRecoveryCodes()
	if err != nil {
		return nil, nil, errs.Wrap("twofactor.NewRecoveryCodes", err)
	}

	hashes = make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = s.hasher.Hash(twofactor.Normalize(c))
	}

	return codes, hashes, nil
}
