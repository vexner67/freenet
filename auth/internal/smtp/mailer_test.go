package smtp_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/auth/internal/smtp"
)

// fakeServer speaks just enough SMTP for one message and returns its DATA.
func fakeServer(t *testing.T) (port string, data <-chan string) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	out := make(chan string, 1)

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		r := bufio.NewReader(conn)
		reply := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }

		reply("220 localhost ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}

			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				reply("250 localhost")
			case cmd == "DATA":
				reply("354 go ahead")
				var msg strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					msg.WriteString(l)
				}
				out <- msg.String()
				reply("250 queued")
			case cmd == "QUIT":
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()

	_, port, _ = net.SplitHostPort(lis.Addr().String())

	return port, out
}

func TestMailer_SendCode(t *testing.T) {
	t.Parallel()

	port, data := fakeServer(t)
	m := smtp.NewMailer("127.0.0.1", port, "", "", "noreply@freenet.local")

	if err := m.SendCode(context.Background(), "ann@example.com", "123456"); err != nil {
		t.Fatalf("SendCode: %v", err)
	}

	msg := <-data
	for _, want := range []string{
		"To: ann@example.com\r\n",
		"Subject: =?utf-8?q?",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\n123456\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}

func TestMailer_SendCode_SilentServer(t *testing.T) {
	t.Parallel()

	// Accepts connections but never greets, like a stuck mail server.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	go func() {
		conn, err := lis.Accept()
		if err == nil {
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	_, port, _ := net.SplitHostPort(lis.Addr().String())
	m := smtp.NewMailer("127.0.0.1", port, "", "", "noreply@freenet.local")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	if err := m.SendCode(ctx, "ann@example.com", "123456"); err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("SendCode hung for %v", elapsed)
	}
}
