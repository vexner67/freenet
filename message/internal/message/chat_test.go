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

	photo := uuid.New()
	if shown := chat.WithPhoto(photo); shown.PhotoID() != photo || chat.PhotoID() != uuid.Nil {
		t.Errorf("WithPhoto = %v, the chat itself %v", shown.PhotoID(), chat.PhotoID())
	}

	if restored := message.RestoreChat(chat.WithPhoto(photo).Stored()); restored.PhotoID() != photo || restored.Title() != "Поход" || restored.OwnerID() != owner {
		t.Errorf("RestoreChat(Stored) = %+v", restored)
	}
}

func TestChat_Pinned(t *testing.T) {
	t.Parallel()

	now := time.Now()
	owner := uuid.New()

	chat, err := message.NewChat(owner, "Поход", now)
	if err != nil {
		t.Fatal(err)
	}

	said, err := message.NewChatMessage(chat.ID(), owner, message.RestoreContent("идём", nil, uuid.Nil, nil), now)
	if err != nil {
		t.Fatal(err)
	}

	pinned, err := chat.Pinned(said)
	if err != nil || pinned.PinnedID() != said.ID() || chat.PinnedID() != uuid.Nil {
		t.Fatalf("Pinned = %v, %v", pinned, err)
	}
	if pinned.Unpinned().PinnedID() != uuid.Nil {
		t.Fatal("Unpinned keeps the message")
	}

	other, _ := message.NewChat(owner, "", now)
	elsewhere, _ := message.NewChatMessage(other.ID(), owner, message.RestoreContent("там", nil, uuid.Nil, nil), now)
	action, _ := message.NewChatAction(chat.ID(), owner, message.ActionTitle, uuid.Nil, "Горы", now)
	gone, _ := said.TakenBack(owner, now)

	for name, m := range map[string]*message.ChatMessage{"of another chat": elsewhere, "an action": action, "taken back": gone} {
		if _, err := chat.Pinned(m); !errors.Is(err, message.ErrNotFound) {
			t.Errorf("pinned %s: %v", name, err)
		}
	}

	active := message.Member{UserID: owner, State: message.MemberActive}
	left := message.Member{UserID: owner, State: message.MemberLeft, SeenUpTo: uuid.Nil}

	if message.PinnedFor(said, active) != said {
		t.Error("a member is not shown the pinned message")
	}
	for name, got := range map[string]*message.ChatMessage{
		"nothing pinned":        message.PinnedFor(nil, active),
		"taken back":            message.PinnedFor(gone, active),
		"written after leaving": message.PinnedFor(said, left),
	} {
		if got != nil {
			t.Errorf("%s: shown %v", name, got)
		}
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

	said := func(text message.Text, photos message.Photos) message.Content {
		c, err := message.NewContent(text, photos, uuid.Nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		return c
	}

	m, err := message.NewChatMessage(chat, ann, said("привет", message.Photos{uuid.New()}), now)
	if err != nil {
		t.Fatal(err)
	}
	if m.ChatID() != chat || m.SenderID() != ann || m.IsAction() || m.IsDeleted() || !m.EditableUntil().Equal(now.Add(24*time.Hour)) {
		t.Fatalf("message = %+v", m)
	}

	t.Run("edit", func(t *testing.T) {
		t.Parallel()

		edited, err := m.Edited(ann, message.Revision{}, now.Add(time.Hour))
		if err != nil || edited.Text() != "" || len(edited.Photos()) != 1 || edited.EditedAt().IsZero() || m.Text() != "привет" {
			t.Fatalf("Edited to photos only = %+v, %v", edited, err)
		}

		if _, err = m.Edited(bob, message.Revision{Text: "нет"}, now); !errors.Is(err, message.ErrCannotEdit) {
			t.Errorf("someone else edits: %v", err)
		}
		if _, err = m.Edited(ann, message.Revision{Text: "поздно"}, now.Add(25*time.Hour)); !errors.Is(err, message.ErrEditExpired) {
			t.Errorf("an edit a day later: %v", err)
		}

		words, _ := message.NewChatMessage(chat, ann, said("слова", nil), now)
		if _, err = words.Edited(ann, message.Revision{Text: ""}, now); !errors.Is(err, message.ErrTextEmpty) {
			t.Errorf("words edited to nothing: %v", err)
		}

		was, stale := message.Text("слова"), message.Text("слово")
		if _, err = words.Edited(ann, message.Revision{Text: "иначе", Base: &was}, now); err != nil {
			t.Errorf("an edit of the text it saw: %v", err)
		}
		if _, err = words.Edited(ann, message.Revision{Text: "иначе", Base: &stale}, now); !errors.Is(err, message.ErrEditConflict) {
			t.Errorf("an edit of an older text: %v", err)
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
		if _, err = gone.Edited(ann, message.Revision{Text: "снова"}, now.Add(time.Hour)); !errors.Is(err, message.ErrNotFound) {
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

		if _, err = invited.Edited(ann, message.Revision{Text: "не было"}, now); !errors.Is(err, message.ErrCannotEdit) {
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

func TestChatMessage_Content(t *testing.T) {
	t.Parallel()

	chat, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	post, kept, dropped, added := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	carried, err := message.NewForwards([]message.Forwarded{{SenderID: bob, Text: "идём в Поход", CreatedAt: now.Add(-time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}

	content, err := message.NewContent("гляньте", message.Photos{kept, dropped}, post, carried)
	if err != nil {
		t.Fatal(err)
	}

	m, err := message.NewChatMessage(chat, ann, content, now)
	if err != nil || m.Post() != post || len(m.Forwarded()) != 1 || len(m.Photos()) != 2 {
		t.Fatalf("NewChatMessage = %+v, %v", m, err)
	}

	// Carried on, the message goes first and what it carried a level deeper.
	on := m.Carried()
	if len(on) != 2 || on[0].SenderID != ann || on[0].Text != "гляньте" || on[0].Post != post || on[0].Depth != 0 || on[1].SenderID != bob || on[1].Depth != 1 {
		t.Fatalf("Carried = %+v", on)
	}

	edited, err := m.Edited(ann, message.Revision{Removed: message.Photos{dropped}, Added: message.Photos{added}}, now.Add(time.Minute))
	if err != nil || edited.Text() != "" || len(edited.Photos()) != 2 || edited.Photos()[0] != kept || edited.Photos()[1] != added ||
		edited.Post() != post || len(edited.Forwarded()) != 1 {
		t.Fatalf("Edited = %+v, %v", edited, err)
	}

	for query, want := range map[string]bool{"глянь": true, "ПОХОД": true, "море": false} {
		q, err := message.NewQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if got := q.MatchesChat(m); got != want {
			t.Errorf("MatchesChat(%q) = %v, want %v", query, got, want)
		}
	}

	gone, err := m.TakenBack(ann, now.Add(time.Minute))
	if err != nil || gone.Post() != uuid.Nil || len(gone.Forwarded()) != 0 || gone.Carried() != nil {
		t.Fatalf("TakenBack = %+v, %v", gone, err)
	}

	q, _ := message.NewQuery("поход")

	renamed, err := message.NewChatAction(chat, ann, message.ActionTitle, uuid.Nil, "Поход", now)
	if err != nil || q.MatchesChat(renamed) || q.MatchesChat(gone) || renamed.Carried() != nil {
		t.Fatalf("what happened and what was taken back say something: %v", err)
	}

	share, err := message.NewPhotoShare("фото", uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	shared, err := message.NewChatMessage(chat, ann, share, now)
	if err != nil || shared.SharedPhoto() == uuid.Nil {
		t.Fatalf("a photo of an album = %+v, %v", shared, err)
	}
	if _, err = shared.Edited(ann, message.Revision{Text: "фото", Added: message.Photos{added}}, now); !errors.Is(err, message.ErrSharedPhotoAlone) {
		t.Errorf("own photos next to a photo of an album: %v", err)
	}
}
