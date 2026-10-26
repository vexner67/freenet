package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/errs"
)

var (
	ErrInvalidCode     = errors.New("invalid code")
	ErrCodeNotFound    = errors.New("code not found")
	ErrTooManyAttempts = errors.New("too many attempts")
	ErrTooManyRequests = errors.New("too many requests")

	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrSessionNotFound     = errors.New("session not found")

	// ErrTicketNotFound: the sign-in waiting for its second step expired,
	// was finished or never was.
	ErrTicketNotFound = errors.New("ticket not found")

	ErrUserServiceUnavailable = errors.New("user service unavailable")
	// ErrAccountNotFound: the user was erased after deleting the page.
	ErrAccountNotFound = errors.New("account not found")
)

// Account is auth's view of a user: who it is and whether the page is
// deleted, which restricts its access tokens to restoring it, or blocked
// by the administrators, which restricts them to reading why.
type Account struct {
	UserID      uuid.UUID
	Deactivated bool
	Banned      bool
}

// UserProvider is auth's view of the user service.
type UserProvider interface {
	GetOrCreateByEmail(ctx context.Context, email account.Email) (Account, error)
	// Account returns ErrAccountNotFound once the user is erased.
	Account(ctx context.Context, userID uuid.UUID) (Account, error)
}

type SessionRepository interface {
	Save(ctx context.Context, s *session.Session) error
	// Get returns ErrSessionNotFound.
	Get(ctx context.Context, id uuid.UUID) (*session.Session, error)
	// Rotate saves the session as it is after a refresh (its token hashes,
	// device and time of use) only if its hash still equals oldHash,
	// otherwise returns ErrSessionNotFound. This makes two concurrent
	// refreshes with the same token unable to both succeed.
	Rotate(ctx context.Context, rotated *session.Session, oldHash string) error
	// Delete returns ErrSessionNotFound.
	Delete(ctx context.Context, id uuid.UUID) error
}

type Hasher interface {
	Hash(value string) string
	Compare(value string, hash string) bool
}

type CodeRepository interface {
	// Save replaces any previous code for the email and resets its attempts.
	Save(ctx context.Context, email account.Email, codeHash string, ttl time.Duration) error
	// Get returns ErrCodeNotFound if there is no active code for the email.
	Get(ctx context.Context, email account.Email) (string, error)
	// IncrementAttempts atomically counts one more check of the active code
	// and returns the new count, or ErrCodeNotFound.
	IncrementAttempts(ctx context.Context, email account.Email) (int, error)
	// Delete returns ErrCodeNotFound if there was nothing to delete.
	Delete(ctx context.Context, email account.Email) error
}

// RateLimiter counts events per key in fixed time windows.
type RateLimiter interface {
	// Allow records an event and reports whether the key is still within
	// limit events for the current window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// TicketRepository keeps the tickets of sign-ins waiting for the second
// step: the email code is right, the code of the app is still to come.
type TicketRepository interface {
	// Save keeps the ticket of the hash for userID, with no attempts.
	Save(ctx context.Context, hash string, userID uuid.UUID, ttl time.Duration) error
	// Get returns ErrTicketNotFound when there is no live ticket.
	Get(ctx context.Context, hash string) (uuid.UUID, error)
	// IncrementAttempts atomically counts one more code brought with the
	// ticket and returns the new count, or ErrTicketNotFound.
	IncrementAttempts(ctx context.Context, hash string) (int, error)
	// Delete returns ErrTicketNotFound if there was nothing to delete.
	Delete(ctx context.Context, hash string) error
}

// SecondStep is the second step of signing in, for a page that has it on.
type SecondStep interface {
	Enabled(ctx context.Context, userID uuid.UUID) (bool, error)
	// Check spends a code of the app or a recovery code;
	// twofactor.ErrInvalidCode, ErrTooManyRequests.
	Check(ctx context.Context, userID uuid.UUID, code string) error
}

type Mailer interface {
	SendCode(ctx context.Context, email string, code string) error
}

// Policy tunes sign-in codes against guessing and spam.
type Policy struct {
	CodeTTL time.Duration
	// MaxCodeAttempts is how many checks one code survives, the right one
	// included; then it is burned and a new code must be requested.
	MaxCodeAttempts int
	// CodeResendInterval allows one code per email per interval.
	CodeResendInterval time.Duration
	// CodeRequestsPerIP codes per CodeRequestsWindow from one address.
	CodeRequestsPerIP  int
	CodeRequestsWindow time.Duration
	// TicketTTL is how long the second step of a sign-in may take.
	TicketTTL time.Duration
}

// RefreshClaims is what auth needs from a verified refresh token.
type RefreshClaims struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}

type TokenProvider interface {
	// CreateAccessToken marks the token of a deleted or a blocked page.
	CreateAccessToken(acc Account, sessionID, tokenID string) (string, error)
	CreateRefreshToken(userID, sessionID, tokenID string) (string, error)
	// ParseRefreshToken returns ErrInvalidRefreshToken for a token that is
	// malformed, expired, forged or not a refresh token.
	ParseRefreshToken(token string) (RefreshClaims, error)
}

type AuthService struct {
	userProvider  UserProvider
	sessionRepo   SessionRepository
	hasher        Hasher
	codeRepo      CodeRepository
	limiter       RateLimiter
	mailer        Mailer
	tokenProvider TokenProvider
	second        SecondStep
	tickets       TicketRepository
	policy        Policy
	now           func() time.Time
}

func NewAuthService(
	userProvider UserProvider,
	repo SessionRepository,
	hasher Hasher,
	codeRepo CodeRepository,
	limiter RateLimiter,
	mailer Mailer,
	tokenProvider TokenProvider,
	second SecondStep,
	tickets TicketRepository,
	policy Policy,
) *AuthService {
	return &AuthService{
		userProvider:  userProvider,
		sessionRepo:   repo,
		hasher:        hasher,
		codeRepo:      codeRepo,
		limiter:       limiter,
		mailer:        mailer,
		tokenProvider: tokenProvider,
		second:        second,
		tickets:       tickets,
		policy:        policy,
		now:           time.Now,
	}
}

// RequestCode mails a sign-in code. clientIP may be empty when unknown;
// then only the per-email limit applies.
func (s *AuthService) RequestCode(ctx context.Context, email account.Email, clientIP string) error {
	if clientIP != "" {
		if err := s.allow(ctx, "code_request:ip:"+clientIP, s.policy.CodeRequestsPerIP, s.policy.CodeRequestsWindow); err != nil {
			return errs.Wrap("s.allow", err)
		}
	}

	if err := s.allow(ctx, "code_request:email:"+email.String(), 1, s.policy.CodeResendInterval); err != nil {
		return errs.Wrap("s.allow", err)
	}

	code, err := generateCode()
	if err != nil {
		return errs.Wrap("generateCode", err)
	}

	if err = s.codeRepo.Save(ctx, email, s.hasher.Hash(code), s.policy.CodeTTL); err != nil {
		return errs.Wrap("s.codeRepo.Save", err)
	}

	if err = s.mailer.SendCode(ctx, email.String(), code); err != nil {
		return errs.Wrap("s.mailer.SendCode", err)
	}

	return nil
}

func (s *AuthService) allow(ctx context.Context, key string, limit int, window time.Duration) error {
	ok, err := s.limiter.Allow(ctx, key, limit, window)
	if err != nil {
		return errs.Wrap("s.limiter.Allow", err)
	}

	if !ok {
		return ErrTooManyRequests
	}

	return nil
}

type TokenPair struct {
	Access  string
	Refresh string
}

// SignIn is the outcome of the email code: the tokens, or, for a page with
// the second step on, the ticket that brings the code of the app.
type SignIn struct {
	Tokens TokenPair
	Ticket string
}

// VerifyCode signs in: device is where from, for the owner's list of
// sessions.
func (s *AuthService) VerifyCode(ctx context.Context, email account.Email, code string, device session.Device) (SignIn, error) {
	if err := s.verifyCode(ctx, email, code); err != nil {
		return SignIn{}, errs.Wrap("s.verifyCode", err)
	}

	acc, err := s.getOrCreateUser(ctx, email)
	if err != nil {
		return SignIn{}, errs.Wrap("s.getOrCreateUser", err)
	}

	second, err := s.second.Enabled(ctx, acc.UserID)
	if err != nil {
		return SignIn{}, errs.Wrap("s.second.Enabled", err)
	}

	if second {
		ticket, err := s.newTicket(ctx, acc.UserID)

		return SignIn{Ticket: ticket}, errs.Wrap("s.newTicket", err)
	}

	pair, err := s.createSession(ctx, acc, device)

	return SignIn{Tokens: pair}, errs.Wrap("s.createSession", err)
}

// VerifySecondStep finishes a sign-in of the ticket with a code of the app
// or a recovery code. A ticket survives MaxCodeAttempts codes, as an email
// code does.
func (s *AuthService) VerifySecondStep(ctx context.Context, ticket, code string, device session.Device) (TokenPair, error) {
	hash := s.hasher.Hash(ticket)

	attempts, err := s.tickets.IncrementAttempts(ctx, hash)
	if err != nil {
		return TokenPair{}, errs.Wrap("s.tickets.IncrementAttempts", err)
	}

	if attempts > s.policy.MaxCodeAttempts {
		if err = s.tickets.Delete(ctx, hash); err != nil && !errors.Is(err, ErrTicketNotFound) {
			return TokenPair{}, errs.Wrap("s.tickets.Delete", err)
		}

		return TokenPair{}, ErrTooManyAttempts
	}

	userID, err := s.tickets.Get(ctx, hash)
	if err != nil {
		return TokenPair{}, errs.Wrap("s.tickets.Get", err)
	}

	if err = s.second.Check(ctx, userID, code); err != nil {
		return TokenPair{}, errs.Wrap("s.second.Check", err)
	}

	// Only one of two requests with the same ticket gets the tokens.
	if err = s.tickets.Delete(ctx, hash); err != nil {
		return TokenPair{}, errs.Wrap("s.tickets.Delete", err)
	}

	acc, err := s.userProvider.Account(ctx, userID)
	if err != nil {
		return TokenPair{}, errs.Wrap("s.userProvider.Account", err)
	}

	return s.createSession(ctx, acc, device)
}

// newTicket is a random ticket of the sign-in of userID, kept as a hash.
func (s *AuthService) newTicket(ctx context.Context, userID uuid.UUID) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errs.Wrap("rand.Read", err)
	}

	ticket := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.tickets.Save(ctx, s.hasher.Hash(ticket), userID, s.policy.TicketTTL); err != nil {
		return "", errs.Wrap("s.tickets.Save", err)
	}

	return ticket, nil
}

// RefreshToken exchanges a refresh token for a new pair and rotates it:
// the presented token stops working. The new access token says anew
// whether the page is deleted; the session of an erased user ends. The
// session remembers when and where from it was refreshed.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string, device session.Device) (TokenPair, error) {
	sess, previous, err := s.currentSession(ctx, refreshToken)
	if err != nil {
		return TokenPair{}, errs.Wrap("s.currentSession", err)
	}

	acc, err := s.userProvider.Account(ctx, sess.UserID())
	if errors.Is(err, ErrAccountNotFound) {
		if err = s.sessionRepo.Delete(ctx, sess.ID()); err != nil && !errors.Is(err, ErrSessionNotFound) {
			return TokenPair{}, errs.Wrap("s.sessionRepo.Delete", err)
		}

		return TokenPair{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return TokenPair{}, errs.Wrap("s.userProvider.Account", err)
	}

	pair, err := s.issueTokens(acc, sess.ID())
	if err != nil {
		return TokenPair{}, errs.Wrap("s.issueTokens", err)
	}

	hash := s.hasher.Hash(pair.Refresh)
	rotated := sess.Rotated(hash, device, s.now())
	if previous {
		rotated = sess.Reissued(hash, device, s.now())
	}
	if err = s.sessionRepo.Rotate(ctx, rotated, sess.RefreshTokenHash()); err != nil {
		return TokenPair{}, errs.Wrap("s.sessionRepo.Rotate", err)
	}

	return pair, nil
}

// Logout ends the session of the refresh token. Logging out of a session
// that no longer exists succeeds, so clients can retry safely.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	sess, _, err := s.currentSession(ctx, refreshToken)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.currentSession", err)
	}

	if err = s.sessionRepo.Delete(ctx, sess.ID()); err != nil && !errors.Is(err, ErrSessionNotFound) {
		return errs.Wrap("s.sessionRepo.Delete", err)
	}

	return nil
}

// currentSession returns the session the refresh token belongs to, if the
// token is the latest one issued for it, or the previous one while the
// latest is unused (previous is then true): a client that left a page
// while its refresh was on the way never got the latest one.
func (s *AuthService) currentSession(ctx context.Context, refreshToken string) (sess *session.Session, previous bool, err error) {
	claims, err := s.tokenProvider.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, false, errs.Wrap("s.tokenProvider.ParseRefreshToken", err)
	}

	sess, err = s.sessionRepo.Get(ctx, claims.SessionID)
	if err != nil {
		return nil, false, errs.Wrap("s.sessionRepo.Get", err)
	}

	if sess.UserID() == claims.UserID {
		if s.hasher.Compare(refreshToken, sess.RefreshTokenHash()) {
			return sess, false, nil
		}
		if sess.PreviousTokenHash() != "" && s.hasher.Compare(refreshToken, sess.PreviousTokenHash()) {
			return sess, true, nil
		}
	}

	// A correctly signed token older than that: someone kept a copy and
	// the latest one has been used. We cannot tell the thief from the
	// owner, so end the session for both.
	if err = s.sessionRepo.Delete(ctx, sess.ID()); err != nil && !errors.Is(err, ErrSessionNotFound) {
		return nil, false, errs.Wrap("s.sessionRepo.Delete", err)
	}

	return nil, false, ErrInvalidRefreshToken
}

// verifyCode accepts each code once: a successful check consumes it.
func (s *AuthService) verifyCode(ctx context.Context, email account.Email, code string) error {
	// Count the attempt before comparing: otherwise many parallel guesses
	// could all be compared before the counter reaches the limit.
	attempts, err := s.codeRepo.IncrementAttempts(ctx, email)
	if err != nil {
		return errs.Wrap("s.codeRepo.IncrementAttempts", err)
	}

	if attempts > s.policy.MaxCodeAttempts {
		if err = s.codeRepo.Delete(ctx, email); err != nil && !errors.Is(err, ErrCodeNotFound) {
			return errs.Wrap("s.codeRepo.Delete", err)
		}

		return ErrTooManyAttempts
	}

	codeHash, err := s.codeRepo.Get(ctx, email)
	if err != nil {
		return errs.Wrap("s.codeRepo.Get", err)
	}

	if !s.hasher.Compare(code, codeHash) {
		return ErrInvalidCode
	}

	// Delete fails with ErrCodeNotFound if a concurrent request consumed
	// the code first, so only one of them gets the tokens.
	if err = s.codeRepo.Delete(ctx, email); err != nil {
		return errs.Wrap("s.codeRepo.Delete", err)
	}

	return nil
}

func (s *AuthService) getOrCreateUser(
	ctx context.Context,
	email account.Email,
) (Account, error) {
	acc, err := s.userProvider.GetOrCreateByEmail(ctx, email)
	if err != nil {
		return Account{}, errs.Wrap("s.userProvider.GetOrCreateByEmail", err)
	}

	return acc, nil
}

func (s *AuthService) createSession(
	ctx context.Context,
	acc Account,
	device session.Device,
) (TokenPair, error) {
	sessionID, err := uuid.NewV7()
	if err != nil {
		return TokenPair{}, errs.Wrap("uuid.NewV7", err)
	}

	pair, err := s.issueTokens(acc, sessionID)
	if err != nil {
		return TokenPair{}, errs.Wrap("s.issueTokens", err)
	}

	sess, err := session.New(
		sessionID,
		acc.UserID,
		s.hasher.Hash(pair.Refresh),
		device,
		s.now(),
	)
	if err != nil {
		return TokenPair{}, errs.Wrap("session.New", err)
	}

	if err = s.sessionRepo.Save(ctx, sess); err != nil {
		return TokenPair{}, errs.Wrap("s.sessionRepo.Save", err)
	}

	return pair, nil
}

func (s *AuthService) issueTokens(acc Account, sessionID uuid.UUID) (TokenPair, error) {
	refreshTokenID, err := uuid.NewV7()
	if err != nil {
		return TokenPair{}, errs.Wrap("uuid.NewV7", err)
	}

	refreshToken, err := s.tokenProvider.CreateRefreshToken(
		acc.UserID.String(),
		sessionID.String(),
		refreshTokenID.String(),
	)
	if err != nil {
		return TokenPair{}, errs.Wrap("s.tokenProvider.CreateRefreshToken", err)
	}

	accessTokenID, err := uuid.NewV7()
	if err != nil {
		return TokenPair{}, errs.Wrap("uuid.NewV7", err)
	}

	accessToken, err := s.tokenProvider.CreateAccessToken(acc, sessionID.String(), accessTokenID.String())
	if err != nil {
		return TokenPair{}, errs.Wrap("s.tokenProvider.CreateAccessToken", err)
	}

	return TokenPair{
		Access:  accessToken,
		Refresh: refreshToken,
	}, nil
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", errs.Wrap("rand.Int", err)
	}

	return fmt.Sprintf("%06d", n.Int64()), nil
}
