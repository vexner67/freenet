package smtp

import (
	"context"
	"fmt"
	"net/smtp"
)

type Mailer struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewMailer(
	host string,
	port string,
	username string,
	password string,
	from string,
) *Mailer {
	return &Mailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}
}

func (s *Mailer) SendCode(ctx context.Context, email, code string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	auth := smtp.PlainAuth(
		"",
		s.username,
		s.password,
		s.host,
	)

	message := []byte(
		"From: " + s.from + "\r\n" +
			"To: " + email + "\r\n" +
			"Subject: Код подтверждения Freenet\r\n" +
			"\r\n" +
			fmt.Sprintf("%s\r\n", code),
	)

	return smtp.SendMail(
		s.host+":"+s.port,
		auth,
		s.from,
		[]string{email},
		message,
	)
}
