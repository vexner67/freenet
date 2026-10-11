package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewTitle(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, in, want string
		err            error
	}{
		{name: "empty", in: "", want: ""},
		{name: "trimmed", in: "  Поход  ", want: "Поход"},
		{name: "one line", in: "Поход\nв горы\t летом", want: "Поход в горы летом"},
		{name: "at the limit", in: strings.Repeat("я", 64), want: strings.Repeat("я", 64)},
		{name: "too long", in: strings.Repeat("я", 65), err: message.ErrTitleTooLong},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.NewTitle(tt.in)
			if !errors.Is(err, tt.err) || got.String() != tt.want {
				t.Fatalf("NewTitle(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestInvited(t *testing.T) {
	t.Parallel()

	owner, a, b := uuid.New(), uuid.New(), uuid.New()

	many := make([]uuid.UUID, message.MaxChatMembers)
	for i := range many {
		many[i] = uuid.New()
	}

	for _, tt := range []struct {
		name string
		ids  []uuid.UUID
		want int
		err  error
	}{
		{name: "two others", ids: []uuid.UUID{a, b}, want: 2},
		{name: "each once, without the owner", ids: []uuid.UUID{a, owner, b, a, uuid.Nil}, want: 2},
		{name: "one other is a dialog", ids: []uuid.UUID{a, a, owner}, err: message.ErrTooFewMembers},
		{name: "nobody", err: message.ErrTooFewMembers},
		{name: "as many as fit", ids: many[:message.MaxChatMembers-1], want: message.MaxChatMembers - 1},
		{name: "too many", ids: many, err: message.ErrChatFull},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.Invited(owner, tt.ids)
			if !errors.Is(err, tt.err) || len(got) != tt.want {
				t.Fatalf("Invited = %d, %v; want %d, %v", len(got), err, tt.want, tt.err)
			}
		})
	}
}

func TestChat_CheckKick(t *testing.T) {
	t.Parallel()

	owner, member := uuid.New(), uuid.New()

	chat, err := message.NewChat(owner, "Поход", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err = chat.CheckKick(owner, member); err != nil {
		t.Errorf("the creator removes a member: %v", err)
	}
	if err = chat.CheckKick(member, owner); !errors.Is(err, message.ErrCannotKick) {
		t.Errorf("a member removes the creator: %v", err)
	}
	if err = chat.CheckKick(owner, owner); !errors.Is(err, message.ErrKickYourself) {
		t.Errorf("the creator removes themselves: %v", err)
	}

	if renamed := chat.Renamed("Горы"); renamed.Title() != "Горы" || chat.Title() != "Поход" || renamed.ID() != chat.ID() {
		t.Errorf("Renamed = %q, the chat itself %q", renamed.Title(), chat.Title())
	}
}

func TestAdmit(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		m     message.Member
		known bool
		want  message.Admission
		err   error
	}{
		{name: "someone new", want: message.Admitted},
		{name: "removed before", m: message.Member{State: message.MemberKicked}, known: true, want: message.Admitted},
		{name: "in the chat", m: message.Member{State: message.MemberActive}, known: true, want: message.AlreadyIn},
		{name: "left on their own", m: message.Member{State: message.MemberLeft}, known: true, err: message.ErrMemberLeft},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := message.Admit(tt.m, tt.known)
			if !errors.Is(err, tt.err) || (err == nil && got != tt.want) {
				t.Fatalf("Admit = %v, %v; want %v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestMember(t *testing.T) {
	t.Parallel()

	older, seen, newer := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	in := message.Member{State: message.MemberActive, SeenUpTo: seen, LastIncomingID: seen, ReadUpTo: older}
	if !in.IsActive() || in.CheckActive() != nil || !in.Sees(newer) || in.Bound() != uuid.Max || !in.HasUnread() {
		t.Errorf("a member in the chat: %+v", in)
	}

	for _, state := range []message.MemberState{message.MemberLeft, message.MemberKicked} {
		gone := message.Member{State: state, SeenUpTo: seen, LastIncomingID: seen, ReadUpTo: seen}
		if gone.IsActive() || !errors.Is(gone.CheckActive(), message.ErrNotInChat) {
			t.Errorf("%s is in the chat", state)
		}
		if !gone.Sees(older) || !gone.Sees(seen) || gone.Sees(newer) || gone.Bound() != seen {
			t.Errorf("%s sees what came after they went", state)
		}
		if gone.HasUnread() {
			t.Errorf("%s has read everything and has unread", state)
		}
	}
}

func TestChatMessage(t *testing.T) {
	t.Parallel()

	chat, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	if _, err := message.NewChatMessage(chat, ann, "", nil, now); !errors.Is(err, message.ErrTextEmpty) {
		t.Fatalf("an empty message: %v", err)
	}

	m, err := message.NewChatMessage(chat, ann, "привет", message.Photos{uuid.New()}, now)
	if err != nil {
		t.Fatal(err)
	}
	if m.ChatID() != chat || m.SenderID() != ann || m.IsAction() || m.IsDeleted() || !m.EditableUntil().Equal(now.Add(24*time.Hour)) {
		t.Fatalf("message = %+v", m)
	}

	t.Run("edit", func(t *testing.T) {
		t.Parallel()

		edited, err := m.Edited(ann, "", now.Add(time.Hour))
		if err != nil || edited.Text() != "" || len(edited.Photos()) != 1 || edited.EditedAt().IsZero() || m.Text() != "привет" {
			t.Fatalf("Edited to photos only = %+v, %v", edited, err)
		}

		if _, err = m.Edited(bob, "нет", now); !errors.Is(err, message.ErrCannotEdit) {
			t.Errorf("someone else edits: %v", err)
		}
		if _, err = m.Edited(ann, "поздно", now.Add(25*time.Hour)); !errors.Is(err, message.ErrEditExpired) {
			t.Errorf("an edit a day later: %v", err)
		}

		words, _ := message.NewChatMessage(chat, ann, "слова", nil, now)
		if _, err = words.Edited(ann, "", now); !errors.Is(err, message.ErrTextEmpty) {
			t.Errorf("words edited to nothing: %v", err)
		}
	})

	t.Run("take back", func(t *testing.T) {
		t.Parallel()

		gone, err := m.TakenBack(ann, now.Add(time.Hour))
		if err != nil || !gone.IsDeleted() || gone.Text() != "" || len(gone.Photos()) != 0 || m.IsDeleted() {
			t.Fatalf("TakenBack = %+v, %v", gone, err)
		}

		if _, err = gone.TakenBack(ann, now.Add(time.Hour)); !errors.Is(err, message.ErrNotFound) {
			t.Errorf("taken back twice: %v", err)
		}
		if _, err = gone.Edited(ann, "снова", now.Add(time.Hour)); !errors.Is(err, message.ErrNotFound) {
			t.Errorf("a message taken back is edited: %v", err)
		}
		if _, err = m.TakenBack(bob, now); !errors.Is(err, message.ErrCannotDelete) {
			t.Errorf("someone else takes back: %v", err)
		}
		if _, err = m.TakenBack(ann, now.Add(25*time.Hour)); !errors.Is(err, message.ErrDeleteExpired) {
			t.Errorf("taken back a day later: %v", err)
		}
	})

	t.Run("what happened is not anyone's to change", func(t *testing.T) {
		t.Parallel()

		invited, err := message.NewChatAction(chat, ann, message.ActionInvite, bob, "", now)
		if err != nil || !invited.IsAction() || invited.TargetID() != bob || invited.Text() != "" {
			t.Fatalf("NewChatAction = %+v, %v", invited, err)
		}

		if _, err = invited.Edited(ann, "не было", now); !errors.Is(err, message.ErrCannotEdit) {
			t.Errorf("an action is edited: %v", err)
		}
		if _, err = invited.TakenBack(ann, now); !errors.Is(err, message.ErrCannotDelete) {
			t.Errorf("an action is taken back: %v", err)
		}

		renamed, _ := message.NewChatAction(chat, ann, message.ActionTitle, uuid.Nil, "Горы", now)
		if renamed.Text() != "Горы" {
			t.Errorf("a renaming tells of %q", renamed.Text())
		}
	})
}
