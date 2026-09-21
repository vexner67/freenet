package console

import (
	"context"
	"log/slog"
)

type Mailer struct {
	l *slog.Logger
}

func NewMailer(l *slog.Logger) *Mailer {
	return &Mailer{
		l: l,
	}
}

func (c *Mailer) SendCode(_ context.Context, email, code string) error {
	c.l.Info("verification code", "email", email, "code", code)
	return nil
}
