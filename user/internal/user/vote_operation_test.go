package user_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewTransferNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
		err            error
	}{
		{"empty", "", "", nil},
		{"trimmed", "  За пиццу 🍕 ", "За пиццу 🍕", nil},
		{"longest", strings.Repeat("я", user.MaxTransferNoteLength), strings.Repeat("я", user.MaxTransferNoteLength), nil},
		{"too long", strings.Repeat("я", user.MaxTransferNoteLength+1), "", user.ErrTransferNoteTooLong},
		{"two lines", "раз\nдва", "", user.ErrTransferNoteLines},
		{"invisible", "a\u200bb", "", user.ErrInvisibleText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := user.NewTransferNote(tt.in)
			if err != tt.err || got.String() != tt.want {
				t.Fatalf("NewTransferNote(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestVoteFlow(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "in", "out"} {
		if _, err := user.NewVoteFlow(s); err != nil {
			t.Fatalf("NewVoteFlow(%q): %v", s, err)
		}
	}
	if _, err := user.NewVoteFlow("sideways"); err != user.ErrVoteFlowInvalid {
		t.Fatalf("NewVoteFlow(sideways): %v", err)
	}

	if !user.VoteFlowIn.Matches(5) || user.VoteFlowIn.Matches(-5) ||
		!user.VoteFlowOut.Matches(-5) || user.VoteFlowOut.Matches(5) ||
		!user.VoteFlowAll.Matches(5) || !user.VoteFlowAll.Matches(-5) {
		t.Fatal("Matches is wrong")
	}
}

func TestEntry(t *testing.T) {
	t.Parallel()

	peer := uuid.New()
	e := user.Entry(user.VoteStake, peer, strings.Repeat("ё", user.MaxVoteNoteLength+5))
	if e.Kind != user.VoteStake || e.PeerID != peer || len([]rune(e.Note)) != user.MaxVoteNoteLength {
		t.Fatalf("Entry = %+v", e)
	}
}

func TestOperationsSince(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	since := user.OperationsSince(at)

	before, after := user.OperationsSince(at.Add(-time.Millisecond)), user.OperationsSince(at.Add(time.Millisecond))
	if string(before[:]) >= string(since[:]) || string(since[:]) >= string(after[:]) {
		t.Fatalf("OperationsSince does not grow with the time: %v %v %v", before, since, after)
	}
}
