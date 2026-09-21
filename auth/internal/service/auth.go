package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/errs"
	"github.com/vexner67/freenet/auth/internal/session"
)

type SessionRepository interface {
	Save(ctx context.Context, s *session.Session) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type Hasher interface {
	Hash(value string) string
}

type CodeRepository interface {
	Save(ctx context.Context, email string, codeHash string, ttl time.Duration) error
}

type Mailer interface {
	SendCode(ctx context.Context, email string, code string) error
}

type AuthService struct {
	sessionRepo SessionRepository
	hasher      Hasher
	codeRepo    CodeRepository
	mailer      Mailer
}

func NewAuthService(
	repo SessionRepository,
	hasher Hasher,
	codeRepo CodeRepository,
	mailer Mailer,
) *AuthService {
	return &AuthService{
		sessionRepo: repo,
		hasher:      hasher,
		codeRepo:    codeRepo,
		mailer:      mailer,
	}
}

func (s *AuthService) RequestCode(ctx context.Context, email string) error {
	code, err := generateCode()
	if err != nil {
		return errs.Wrap("generateCode", err)
	}

	codeHash := s.hasher.Hash(code)

	if err = s.codeRepo.Save(ctx, email, codeHash, 5*time.Minute); err != nil {
		return errs.Wrap("s.codeRepo.Save", err)
	}

	return s.mailer.SendCode(ctx, email, code)
}

func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", errs.Wrap("rand.Int", err)
	}

	return fmt.Sprintf("%06d", n.Int64()), nil
}
