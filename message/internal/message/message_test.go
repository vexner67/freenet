package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewContent(t *testing.T) {
	t.Parallel()

	photo := uuid.New()

	if _, err := message.NewContent("", nil, uuid.Nil, nil); !errors.Is(err, message.ErrTextEmpty) {
		t.Fatalf("neither text nor photos: %v", err)
	}
	if c, err := message.NewContent("", message.Photos{photo}, uuid.Nil, nil); err != nil || len(c.Photos()) != 1 {
		t.Fatalf("photos only = %+v, %v", c, err)
	}
	// A post shared into the dialog says something too.
	if c, err := message.NewContent("", nil, photo, nil); err != nil || c.Post() != photo {
		t.Fatalf("a post only = %+v, %v", c, err)
	}

	for _, tt := range []struct {
		name string
		ids  []uuid.UUID
		err  error
	}{
		{"none", nil, nil},
		{"ten", make11()[:10], nil},
		{"eleven", make11(), message.ErrTooManyPhotos},
		{"twice the same", []uuid.UUID{photo, photo}, message.ErrDuplicatePhoto},
		{"nil", []uuid.UUID{uuid.Nil}, message.ErrInvalidPhoto},
	} {
		if _, err := message.NewPhotos(tt.ids); !errors.Is(err, tt.err) {
			t.Errorf("%s: NewPhotos = %v, want %v", tt.name, err, tt.err)
		}
	}
}

func make11() []uuid.UUID {
	ids := make([]uuid.UUID, 11)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return ids
}

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
		{"emoji count as one character", strings.Repeat("👋", 60000), message.Text(strings.Repeat("👋", 60000)), nil},
		{"empty is fine, photos may say it all", "", "", nil},
		{"only spaces and breaks", " \n\t\r\n ", "", nil},
		{"too long", strings.Repeat("я", 60001), "", message.ErrTextTooLong},
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

	m, err := message.New(ann, bob, message.RestoreContent("привет", nil, uuid.Nil, nil), now)
	if err != nil {
		t.Fatal(err)
	}
	if m.SenderID() != ann || m.RecipientID() != bob || m.Text() != "привет" {
		t.Fatalf("message = %v %v %q", m.SenderID(), m.RecipientID(), m.Text())
	}
	if m.CreatedAt().Location() != time.UTC || m.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", m.CreatedAt())
	}

	later, err := message.New(bob, ann, message.RestoreContent("и тебе", nil, uuid.Nil, nil), now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= m.ID().String() {
		t.Fatal("IDs must grow, dialogs are ordered by them")
	}

	if _, err = message.New(ann, ann, message.RestoreContent("сам себе", nil, uuid.Nil, nil), now); !errors.Is(err, message.ErrToYourself) {
		t.Fatalf("to yourself: %v", err)
	}
}

func TestReadMarks(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	mustNew := func(sender, recipient uuid.UUID) *message.Message {
		m, err := message.New(sender, recipient, message.RestoreContent("x", nil, uuid.Nil, nil), now)
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

func TestCheckDelete(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	sent := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)

	m, err := message.New(ann, bob, message.RestoreContent("ой", nil, uuid.Nil, nil), sent)
	if err != nil {
		t.Fatal(err)
	}
	if want := sent.Add(24 * time.Hour); !m.DeletableUntil().Equal(want) {
		t.Fatalf("DeletableUntil = %v, want %v", m.DeletableUntil(), want)
	}

	tests := []struct {
		name      string
		requester uuid.UUID
		now       time.Time
		err       error
	}{
		{"the sender at once", ann, sent, nil},
		{"the sender on the last moment", ann, m.DeletableUntil(), nil},
		{"the sender a day later", ann, m.DeletableUntil().Add(time.Second), message.ErrDeleteExpired},
		{"the recipient", bob, sent, message.ErrCannotDelete},
		{"a stranger", uuid.New(), sent, message.ErrCannotDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := m.CheckDelete(tt.requester, tt.now); !errors.Is(err, tt.err) {
				t.Fatalf("CheckDelete = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestMessage_Edited(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	sent := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	m, _ := message.New(ann, bob, message.RestoreContent("привет", nil, uuid.Nil, nil), sent)
	photo, _ := message.New(ann, bob, message.RestoreContent("море", message.Photos{uuid.New()}, uuid.Nil, nil), sent)

	edited, err := m.Edited(ann, "привет!", sent.Add(time.Hour))
	if err != nil || edited.Text() != "привет!" || !edited.EditedAt().Equal(sent.Add(time.Hour)) || edited.ID() != m.ID() {
		t.Fatalf("Edited = %+v, %v", edited, err)
	}
	if m.Text() != "привет" || !m.EditedAt().IsZero() {
		t.Fatal("the message itself changed")
	}
	if !m.EditableUntil().Equal(sent.Add(24 * time.Hour)) {
		t.Fatalf("EditableUntil = %v", m.EditableUntil())
	}

	if _, err = m.Edited(bob, "x", sent); !errors.Is(err, message.ErrCannotEdit) {
		t.Errorf("the recipient edits: %v", err)
	}
	if _, err = m.Edited(ann, "x", sent.Add(24*time.Hour+time.Second)); !errors.Is(err, message.ErrEditExpired) {
		t.Errorf("a day later: %v", err)
	}
	if _, err = m.Edited(ann, "", sent); !errors.Is(err, message.ErrTextEmpty) {
		t.Errorf("no text left: %v", err)
	}
	// With photos the words may go; the photos stay.
	if edited, err = photo.Edited(ann, "", sent); err != nil || len(edited.Photos()) != 1 {
		t.Errorf("a photo without words: %+v, %v", edited, err)
	}
}

func TestMessage_HiddenFrom(t *testing.T) {
	t.Parallel()

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	m, _ := message.New(ann, bob, message.RestoreContent("привет", nil, uuid.Nil, nil), time.Now())

	if !m.VisibleTo(ann) || !m.VisibleTo(bob) || m.VisibleTo(cat) || !m.Between(bob) || m.Between(cat) {
		t.Fatal("a message is its two sides' only")
	}
	if _, err := m.HiddenFrom(cat); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a stranger hides it: %v", err)
	}

	forBob, err := m.HiddenFrom(bob)
	if err != nil || forBob.VisibleTo(bob) || !forBob.VisibleTo(ann) || forBob.Gone() || !m.VisibleTo(bob) {
		t.Fatalf("hidden from bob = %+v, %v", forBob, err)
	}
	if _, err = forBob.HiddenFrom(bob); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("hidden twice: %v", err)
	}

	gone, err := forBob.HiddenFrom(ann)
	if err != nil || !gone.Gone() {
		t.Fatalf("hidden from both = %+v, %v", gone, err)
	}
}

func TestNewQuery(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	m, _ := message.New(ann, bob, message.RestoreContent("Еду на МОРЕ", nil, uuid.Nil, nil), time.Now())

	q, err := message.NewQuery("  море ")
	if err != nil || q.String() != "море" || !q.Matches(m) {
		t.Fatalf("NewQuery = %q, %v", q, err)
	}
	if other, _ := message.NewQuery("горы"); other.Matches(m) {
		t.Error("горы matches")
	}
	if _, err = message.NewQuery(" \n "); !errors.Is(err, message.ErrQueryEmpty) {
		t.Errorf("empty: %v", err)
	}
	if _, err = message.NewQuery(strings.Repeat("я", 101)); !errors.Is(err, message.ErrQueryTooLong) {
		t.Errorf("101 characters: %v", err)
	}
}

func TestMessage_TakenBack(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	sent := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	m, _ := message.New(ann, bob, message.RestoreContent("ой", message.Photos{uuid.New()}, uuid.Nil, nil), sent)

	if _, err := m.TakenBack(bob, sent); !errors.Is(err, message.ErrCannotDelete) {
		t.Fatalf("the recipient: %v", err)
	}
	if _, err := m.TakenBack(ann, sent.Add(25*time.Hour)); !errors.Is(err, message.ErrDeleteExpired) {
		t.Fatalf("a day later: %v", err)
	}

	gone, err := m.TakenBack(ann, sent.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !gone.IsDeleted() || gone.Text() != "" || len(gone.Photos()) != 0 || !gone.DeletedAt().Equal(sent.Add(time.Hour)) || gone.ID() != m.ID() {
		t.Fatalf("taken back = %+v", gone)
	}
	if m.IsDeleted() || m.Text() != "ой" {
		t.Fatal("the message itself must not change")
	}

	// It stays in the dialog for both, but is nothing to work with.
	if !gone.VisibleTo(ann) || !gone.VisibleTo(bob) {
		t.Fatal("both still have it")
	}
	if _, err = gone.TakenBack(ann, sent.Add(2*time.Hour)); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("taken back twice: %v", err)
	}
	if _, err = gone.Edited(ann, "снова", sent.Add(2*time.Hour)); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("edit: %v", err)
	}
	if got := gone.Carried(); len(got) != 0 {
		t.Fatalf("carried = %+v, want nothing", got)
	}
}

func TestMessage_Carried(t *testing.T) {
	t.Parallel()

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	photo, post := uuid.New(), uuid.New()

	plain, _ := message.New(ann, bob, message.RestoreContent("смотри", message.Photos{photo}, post, nil), now)
	carried := plain.Carried()
	if len(carried) != 1 || carried[0].SenderID != ann || carried[0].Text != "смотри" || carried[0].Photos[0] != photo ||
		carried[0].Post != post || !carried[0].CreatedAt.Equal(plain.CreatedAt()) {
		t.Fatalf("carried = %+v", carried)
	}

	// A forward with words of its own carries them first; one without
	// carries only what it forwards.
	forwards, err := message.NewForwards(carried)
	if err != nil {
		t.Fatal(err)
	}
	withWords, _ := message.New(bob, cat, message.RestoreContent("вот", nil, uuid.Nil, forwards), now)
	if got := withWords.Carried(); len(got) != 2 || got[0].SenderID != bob || got[0].Text != "вот" || got[1].SenderID != ann {
		t.Fatalf("carried = %+v", got)
	}
	bare, _ := message.New(bob, cat, message.RestoreContent("", nil, uuid.Nil, forwards), now)
	if got := bare.Carried(); len(got) != 1 || got[0].SenderID != ann {
		t.Fatalf("carried = %+v", got)
	}

	// Forwards alone are something to send; too many are not.
	if c, err := message.NewContent("", nil, uuid.Nil, forwards); err != nil || len(c.Forwarded()) != 1 {
		t.Fatalf("content of forwards = %+v, %v", c, err)
	}
	if none, err := message.NewForwards(nil); err != nil || none != nil {
		t.Fatalf("no forwards = %v, %v", none, err)
	}
	if _, err := message.NewForwards(make([]message.Forwarded, message.MaxForwarded+1)); !errors.Is(err, message.ErrTooManyForwarded) {
		t.Fatalf("too many: %v", err)
	}

	// Edited, a forward keeps what it carries and may lose its words.
	edited, err := withWords.Edited(bob, "", now.Add(time.Minute))
	if err != nil || len(edited.Forwarded()) != 1 || edited.Text() != "" {
		t.Fatalf("edited = %+v, %v", edited, err)
	}
}
