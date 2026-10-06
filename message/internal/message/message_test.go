package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  message.Text
		err   error
	}{
		{"plain", "привет", "привет", nil},
		{"trimmed, windows line breaks", "  раз\r\nдва \n", "раз\nдва", nil},
		{"emoji count as one character", strings.Repeat("👋", 4096), message.Text(strings.Repeat("👋", 4096)), nil},
		{"empty", "", "", message.ErrTextEmpty},
		{"only spaces and breaks", " \n\t\r\n ", "", message.ErrTextEmpty},
		{"too long", strings.Repeat("я", 4097), "", message.ErrTextTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.NewText(tt.input)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewText = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 5, 18, 0, 0, 123456789, time.FixedZone("MSK", 3*3600))

	m, err := message.New(ann, bob, "привет", now)
	if err != nil {
		t.Fatal(err)
	}
	if m.SenderID() != ann || m.RecipientID() != bob || m.Text() != "привет" {
		t.Fatalf("message = %v %v %q", m.SenderID(), m.RecipientID(), m.Text())
	}
	if m.CreatedAt().Location() != time.UTC || m.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", m.CreatedAt())
	}

	later, err := message.New(bob, ann, "и тебе", now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= m.ID().String() {
		t.Fatal("IDs must grow, dialogs are ordered by them")
	}

	if _, err = message.New(ann, ann, "сам себе", now); !errors.Is(err, message.ErrToYourself) {
		t.Fatalf("to yourself: %v", err)
	}
}

func TestReadMarks(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	mustNew := func(sender, recipient uuid.UUID) *message.Message {
		m, err := message.New(sender, recipient, "x", now)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	fromBob1, fromAnn, fromBob2 := mustNew(bob, ann), mustNew(ann, bob), mustNew(bob, ann)

	// Ann has read bob's first message; bob has read nothing of hers.
	marks := message.ReadMarks{Mine: fromBob1.ID()}

	if !marks.IsRead(fromBob1, ann) || marks.IsRead(fromBob2, ann) {
		t.Fatal("incoming messages are read up to Mine")
	}
	if marks.IsRead(fromAnn, ann) {
		t.Fatal("outgoing messages are read up to Theirs")
	}

	marks.Theirs = fromAnn.ID()
	if !marks.IsRead(fromAnn, ann) {
		t.Fatal("bob has read it now")
	}

	d := message.Dialog{PeerID: bob, LastMessage: fromBob2, LastIncomingID: fromBob2.ID(), Marks: marks}
	if !d.HasUnread() {
		t.Fatal("bob's second message is unread")
	}

	d.Marks.Mine = fromBob2.ID()
	if d.HasUnread() {
		t.Fatal("all read")
	}

	if (message.Dialog{LastMessage: fromAnn}).HasUnread() {
		t.Fatal("nothing incoming, nothing unread")
	}
}
