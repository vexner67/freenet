package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// ErrCaptcha: the digits of the picture are wrong, or it expired, was used
// or never was. One answer, and a new picture is needed after it.
var ErrCaptcha = errors.New("wrong captcha")

// CaptchaRepository keeps what a picture shows, hashed, until it is used.
type CaptchaRepository interface {
	Save(ctx context.Context, id, answerHash string, ttl time.Duration) error
	// Take gets the hash and forgets it, so a picture is used only once;
	// ErrCaptcha when there is none.
	Take(ctx context.Context, id string) (string, error)
}

// CaptchaPolicy says whether signing in asks for a picture, how long one
// lives and how many an address gets in a window.
type CaptchaPolicy struct {
	Required bool
	TTL      time.Duration
	PerIP    int
	Window   time.Duration
}

// Captcha is a picture to read. Without ID and Image none is needed.
type Captcha struct {
	Required bool
	ID       string
	Image    string
}

// CaptchaService stops bots from mailing codes to anyone.
type CaptchaService struct {
	repo    CaptchaRepository
	hasher  Hasher
	limiter RateLimiter
	draw    func() (answer, image string, err error)
	policy  CaptchaPolicy
}

func NewCaptchaService(repo CaptchaRepository, hasher Hasher, limiter RateLimiter, draw func() (string, string, error), policy CaptchaPolicy) *CaptchaService {
	return &CaptchaService{repo: repo, hasher: hasher, limiter: limiter, draw: draw, policy: policy}
}

// New gives a picture to clientIP, which may be empty when unknown.
func (s *CaptchaService) New(ctx context.Context, clientIP string) (Captcha, error) {
	if !s.policy.Required {
		return Captcha{}, nil
	}

	if clientIP != "" {
		ok, err := s.limiter.Allow(ctx, "captcha:ip:"+clientIP, s.policy.PerIP, s.policy.Window)
		if err != nil {
			return Captcha{}, errs.Wrap("s.limiter.Allow", err)
		}
		if !ok {
			return Captcha{}, ErrTooManyRequests
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Captcha{}, errs.Wrap("uuid.NewV7", err)
	}

	answer, image, err := s.draw()
	if err != nil {
		return Captcha{}, errs.Wrap("s.draw", err)
	}

	if err = s.repo.Save(ctx, id.String(), s.hasher.Hash(answer), s.policy.TTL); err != nil {
		return Captcha{}, errs.Wrap("s.repo.Save", err)
	}

	return Captcha{Required: true, ID: id.String(), Image: image}, nil
}

// Check spends the picture id on answer; ErrCaptcha when it is not read
// right. With pictures off anything passes.
func (s *CaptchaService) Check(ctx context.Context, id, answer string) error {
	if !s.policy.Required {
		return nil
	}

	if id == "" {
		return ErrCaptcha
	}

	hash, err := s.repo.Take(ctx, id)
	if err != nil {
		return errs.Wrap("s.repo.Take", err)
	}

	if !s.hasher.Compare(strings.TrimSpace(answer), hash) {
		return ErrCaptcha
	}

	return nil
}
