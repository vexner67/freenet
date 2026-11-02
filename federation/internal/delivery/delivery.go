// Package delivery sends the activities of pages to the inboxes of other
// servers through the outbox: Outbox records an activity in the
// transaction of what caused it, Deliverer posts it afterwards and again
// until the server takes it or it is too old to matter.
package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/apjson"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/remote"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
)

// Stream is the outbox stream of the activities to send.
const Stream = "deliveries"

const kindActivity = "activity"

var errExpired = errors.New("the activity is too old to send")

type message struct {
	Inbox  fedi.URL        `json:"inbox"`
	Signer uuid.UUID       `json:"signer"`
	Body   json.RawMessage `json:"body"`
}

// Recorder adds a message to the outbox in the transaction of ctx.
type Recorder interface {
	Add(ctx context.Context, kind string, payload []byte) error
}

// Outbox is the service.Sender.
type Outbox struct {
	recorder Recorder
}

func NewOutbox(recorder Recorder) *Outbox {
	return &Outbox{
		recorder: recorder,
	}
}

func (o *Outbox) Send(ctx context.Context, inbox fedi.URL, signerID uuid.UUID, a fedi.Outgoing) error {
	body, err := apjson.Activity(a)
	if err != nil {
		return errs.Wrap("apjson.Activity", err)
	}

	payload, err := json.Marshal(message{Inbox: inbox, Signer: signerID, Body: body})
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, kindActivity, payload))
}

// Key keeps the order of the activities sent to one server, and lets
// those to different servers go at the same time: a server that is down
// holds back only its own.
func Key(m outbox.Message) string {
	var msg message
	if json.Unmarshal(m.Payload, &msg) != nil {
		return ""
	}

	return msg.Inbox.Host()
}

// Accounts are the accounts the activities are signed with.
type Accounts interface {
	Get(ctx context.Context, userID uuid.UUID) (*fedi.Account, error)
}

// Poster posts an activity to an inbox.
type Poster interface {
	Post(ctx context.Context, inbox fedi.URL, body []byte, signer *fedi.Account) error
}

// Deliverer posts the recorded activities.
type Deliverer struct {
	accounts Accounts
	poster   Poster
	maxAge   time.Duration
	now      func() time.Time
}

// NewDeliverer gives up on an activity maxAge after it was recorded: a
// server down for that long gets no news older than that.
func NewDeliverer(accounts Accounts, poster Poster, maxAge time.Duration) *Deliverer {
	return &Deliverer{
		accounts: accounts,
		poster:   poster,
		maxAge:   maxAge,
		now:      time.Now,
	}
}

// Deliver is the outbox.Deliver: an answer no retry changes (a server
// that refuses the activity, an account that is gone) drops it.
func (d *Deliverer) Deliver(ctx context.Context, m outbox.Message) error {
	var msg message
	if err := json.Unmarshal(m.Payload, &msg); err != nil {
		return outbox.Permanent(errs.Wrap("json.Unmarshal", err))
	}

	if d.now().Sub(m.CreatedAt) > d.maxAge {
		return outbox.Permanent(errExpired)
	}

	account, err := d.accounts.Get(ctx, msg.Signer)
	if errors.Is(err, fedi.ErrNotFound) {
		return outbox.Permanent(errs.Wrap("d.accounts.Get", err))
	}
	if err != nil {
		return errs.Wrap("d.accounts.Get", err)
	}

	err = d.poster.Post(ctx, msg.Inbox, msg.Body, account)

	var status remote.StatusError
	if errors.As(err, &status) && refused(status.Code) {
		return outbox.Permanent(errs.Wrap("d.poster.Post", err))
	}

	return errs.Wrap("d.poster.Post", err)
}

// refused: the server will not take the activity however often it is
// sent; a server busy or asking to slow down will later.
func refused(code int) bool {
	return code >= 400 && code < 500 && code != http.StatusRequestTimeout && code != http.StatusTooManyRequests
}
