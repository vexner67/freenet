package delivery_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/delivery"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/inmem"
	"github.com/vexner67/freenet/federation/internal/remote"
	"github.com/vexner67/freenet/pkg/outbox"
)

type poster struct {
	err   error
	inbox fedi.URL
	body  []byte
}

func (p *poster) Post(_ context.Context, inbox fedi.URL, body []byte, _ *fedi.Account) error {
	p.inbox, p.body = inbox, body
	return p.err
}

func TestSendAndDeliver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queue := outbox.NewMemory()
	accounts := inmem.NewAccountRepository()

	account, err := fedi.NewAccount(uuid.New(), fedi.KeyPair{Public: "pub", Private: "priv"}, time.Now())
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if err = accounts.Save(ctx, account); err != nil {
		t.Fatalf("Save: %v", err)
	}

	activity := fedi.Outgoing{Type: fedi.TypeDelete, ID: "https://f.example/ap/posts/1#delete-1", Actor: "https://f.example/ap/users/1", Tombstone: "https://f.example/ap/posts/1"}
	if err = delivery.NewOutbox(queue).Send(ctx, "https://m.example/inbox", account.UserID(), activity); err != nil {
		t.Fatalf("Send: %v", err)
	}

	messages, _ := queue.Head(ctx, 10)
	if len(messages) != 1 {
		t.Fatalf("queued %d messages", len(messages))
	}

	m := messages[0]
	if key := delivery.Key(m); key != "m.example" {
		t.Fatalf("Key = %q, want the host of the inbox", key)
	}

	tests := []struct {
		name      string
		err       error
		age       time.Duration
		signer    uuid.UUID
		permanent bool
		failed    bool
	}{
		{"taken", nil, 0, account.UserID(), false, false},
		{"refused", remote.StatusError{Code: 403}, 0, account.UserID(), true, true},
		{"too many", remote.StatusError{Code: 429}, 0, account.UserID(), false, true},
		{"down", remote.StatusError{Code: 503}, 0, account.UserID(), false, true},
		{"unreachable", errors.New("dial"), 0, account.UserID(), false, true},
		{"too old", nil, 49 * time.Hour, account.UserID(), true, true},
		{"no account", nil, 0, uuid.New(), true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msg := m
			msg.CreatedAt = time.Now().Add(-tt.age)
			msg.Payload = withSigner(t, m.Payload, tt.signer)

			p := &poster{err: tt.err}
			err := delivery.NewDeliverer(accounts, p, 48*time.Hour).Deliver(ctx, msg)

			if (err != nil) != tt.failed || outbox.IsPermanent(err) != tt.permanent {
				t.Fatalf("Deliver = %v, want failed %v, permanent %v", err, tt.failed, tt.permanent)
			}

			if tt.name == "taken" {
				var body map[string]any
				if p.inbox != "https://m.example/inbox" || json.Unmarshal(p.body, &body) != nil || body["type"] != "Delete" {
					t.Fatalf("posted %s to %s", p.body, p.inbox)
				}
			}
		})
	}
}

func withSigner(t *testing.T, payload []byte, signer uuid.UUID) []byte {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	m["signer"] = signer.String()

	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	return out
}
