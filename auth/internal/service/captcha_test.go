package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/auth/internal/inmem"
	"github.com/vexner67/freenet/auth/internal/service"
)

type plainHasher struct{}

func (plainHasher) Hash(value string) string        { return "#" + value }
func (plainHasher) Compare(value, hash string) bool { return "#"+value == hash }

type limiterAt struct{ allowed bool }

func (l limiterAt) Allow(context.Context, string, int, time.Duration) (bool, error) {
	return l.allowed, nil
}

func drawn() (string, string, error) { return "12345", "<svg/>", nil }

func TestCaptcha(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	policy := service.CaptchaPolicy{Required: true, TTL: time.Minute, PerIP: 1, Window: time.Minute}
	s := service.NewCaptchaService(inmem.NewCaptchaRepository(), plainHasher{}, limiterAt{true}, drawn, policy)

	c, err := s.New(ctx, "198.51.100.7")
	if err != nil || !c.Required || c.ID == "" || c.Image != "<svg/>" {
		t.Fatalf("New = %+v, %v", c, err)
	}

	if err = s.Check(ctx, c.ID, " 12345 "); err != nil {
		t.Fatalf("the right digits: %v", err)
	}
	if err = s.Check(ctx, c.ID, "12345"); !errors.Is(err, service.ErrCaptcha) {
		t.Fatalf("a picture used twice: %v", err)
	}

	wrong, _ := s.New(ctx, "")
	if err = s.Check(ctx, wrong.ID, "00000"); !errors.Is(err, service.ErrCaptcha) {
		t.Fatalf("wrong digits: %v", err)
	}
	if err = s.Check(ctx, wrong.ID, "12345"); !errors.Is(err, service.ErrCaptcha) {
		t.Fatalf("a second try at one picture: %v", err)
	}
	if err = s.Check(ctx, "", "12345"); !errors.Is(err, service.ErrCaptcha) {
		t.Fatalf("no picture: %v", err)
	}
}

func TestCaptcha_Limits(t *testing.T) {
	t.Parallel()

	policy := service.CaptchaPolicy{Required: true, TTL: time.Minute, PerIP: 1, Window: time.Minute}
	s := service.NewCaptchaService(inmem.NewCaptchaRepository(), plainHasher{}, limiterAt{false}, drawn, policy)

	if _, err := s.New(context.Background(), "198.51.100.7"); !errors.Is(err, service.ErrTooManyRequests) {
		t.Fatalf("New over the limit: %v", err)
	}
}

func TestCaptcha_Off(t *testing.T) {
	t.Parallel()

	s := service.NewCaptchaService(inmem.NewCaptchaRepository(), plainHasher{}, limiterAt{true}, drawn, service.CaptchaPolicy{})

	c, err := s.New(context.Background(), "")
	if err != nil || c.Required || c.ID != "" {
		t.Fatalf("New = %+v, %v", c, err)
	}
	if err = s.Check(context.Background(), "", ""); err != nil {
		t.Fatalf("Check with pictures off: %v", err)
	}
}
