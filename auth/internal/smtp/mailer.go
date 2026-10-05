package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
)

// sendTimeout bounds a send when the caller's context has no deadline.
const sendTimeout = 10 * time.Second

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
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.host, s.port))
	if err != nil {
		return errs.Wrap("dialer.DialContext", err)
	}

	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return errs.Wrap("conn.SetDeadline", err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return errs.Wrap("smtp.NewClient", err)
	}
	defer func() {
		_ = client.Close()
	}()

	if err = s.send(client, email, code); err != nil {
		return errs.Wrap("s.send", err)
	}

	return nil
}

func (s *Mailer) send(client *smtp.Client, email, code string) error {
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return errs.Wrap("client.StartTLS", err)
		}
	}

	// Local catchers such as Mailpit accept mail without credentials.
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return errs.Wrap("client.Auth", err)
		}
	}

	if err := client.Mail(s.from); err != nil {
		return errs.Wrap("client.Mail", err)
	}

	if err := client.Rcpt(email); err != nil {
		return errs.Wrap("client.Rcpt", err)
	}

	w, err := client.Data()
	if err != nil {
		return errs.Wrap("client.Data", err)
	}

	if _, err = w.Write(s.message(email, code)); err != nil {
		_ = w.Close()
		return errs.Wrap("w.Write", err)
	}

	if err = w.Close(); err != nil {
		return errs.Wrap("w.Close", err)
	}

	if err = client.Quit(); err != nil {
		return errs.Wrap("client.Quit", err)
	}

	return nil
}

func (s *Mailer) message(email, code string) []byte {
	return []byte(
		"From: " + s.from + "\r\n" +
			"To: " + email + "\r\n" +
			// Non-ASCII headers must be encoded (RFC 2047).
			"Subject: " + mime.QEncoding.Encode("utf-8", "Код подтверждения Freenet") + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=utf-8\r\n" +
			"\r\n" +
			fmt.Sprintf("%s\r\n", code),
	)
}
