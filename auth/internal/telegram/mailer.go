// Package telegram sends verification codes to a Telegram chat of the
// site's owner, who passes them on: for a site whose users are the
// owner's friends and which has no mail server yet.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
)

// DefaultAPI is the Bot API of Telegram.
const DefaultAPI = "https://api.telegram.org"

// sendTimeout bounds a send when the caller's context has no deadline.
const sendTimeout = 10 * time.Second

// maxResponseSize is more than any answer of sendMessage needs.
const maxResponseSize = 64 << 10

// ErrRejected: Telegram answered, and did not take the message.
var ErrRejected = errors.New("telegram rejected the message")

type Mailer struct {
	api    string
	token  string
	chatID string
	client *http.Client
}

// NewMailer sends to the chat chatID as the bot of token; api is
// DefaultAPI unless a test serves its own.
func NewMailer(api, token, chatID string) *Mailer {
	return &Mailer{
		api:    strings.TrimRight(api, "/"),
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: sendTimeout},
	}
}

// SendCode tells the chat whose code it is: the owner passes it on.
func (m *Mailer) SendCode(ctx context.Context, email, code string) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	form := url.Values{
		"chat_id": {m.chatID},
		"text":    {fmt.Sprintf("Код входа Freenet для %s: %s", email, code)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.api+"/bot"+m.token+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return errs.Wrap("http.NewRequestWithContext", m.safe(err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return errs.Wrap("m.client.Do", m.safe(err))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var answer struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return errs.Wrap("io.ReadAll", m.safe(err))
	}

	// An answer that is not JSON is a failure too: answer.OK stays false.
	_ = json.Unmarshal(body, &answer)

	if resp.StatusCode != http.StatusOK || !answer.OK {
		return fmt.Errorf("%w: status %d: %s", ErrRejected, resp.StatusCode, answer.Description)
	}

	return nil
}

// safe is err without the token: net/http puts the URL of a failed
// request, and so the token in it, into its errors, and errors are logged.
func (m *Mailer) safe(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	return errors.New(strings.ReplaceAll(err.Error(), m.token, "<token>"))
}
