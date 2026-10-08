package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/inmem"
	"github.com/vexner67/freenet/message/internal/mediaclient"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/pubsub"
)

func newService() *service.MessageService {
	store := inmem.NewStore()
	return service.NewMessageService(service.Deps{
		Messages: store.Messages(), Dialogs: store.Dialogs(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: inmem.TxManager{}, Photos: mediaclient.Nop{},
	})
}

func send(t *testing.T, svc *service.MessageService, from, to uuid.UUID, texts ...string) []*message.Message {
	t.Helper()

	sent := make([]*message.Message, 0, len(texts))
	for _, text := range texts {
		m, err := svc.Send(context.Background(), from, to, message.RestoreContent(message.Text(text), nil))
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		sent = append(sent, m)
	}

	return sent
}

func texts(messages []*message.Message) []string {
	out := make([]string, len(messages))
	for i, m := range messages {
		out[i] = m.Text().String()
	}
	return out
}

func TestHistory(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	send(t, svc, ann, bob, "1", "2")
	send(t, svc, bob, ann, "3")
	send(t, svc, ann, cat, "другой диалог")
	send(t, svc, bob, ann, "4", "5")

	var (
		pages  [][]string
		cursor uuid.UUID
	)

	for {
		page, err := svc.History(ctx, ann, bob, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}

		pages = append(pages, texts(page.Messages))

		if page.NextCursor == uuid.Nil {
			break
		}
		cursor = page.NextCursor
	}

	// Back in time page by page, each page oldest first.
	want := [][]string{{"4", "5"}, {"2", "3"}, {"1"}}
	if len(pages) != len(want) {
		t.Fatalf("pages = %v, want %v", pages, want)
	}
	for i := range want {
		if len(pages[i]) != len(want[i]) || pages[i][0] != want[i][0] || pages[i][len(pages[i])-1] != want[i][len(want[i])-1] {
			t.Fatalf("pages = %v, want %v", pages, want)
		}
	}

	// The same dialog from bob's side.
	page, err := svc.History(ctx, bob, ann, uuid.Nil, 50)
	if err != nil || len(page.Messages) != 5 {
		t.Fatalf("bob's history = %v, %v", texts(page.Messages), err)
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	seen := send(t, svc, ann, bob, "1", "2")
	send(t, svc, bob, ann, "3", "4")

	page, err := svc.Newer(ctx, ann, bob, seen[1].ID(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(page.Messages); len(got) != 2 || got[0] != "3" || got[1] != "4" || page.NextCursor != uuid.Nil {
		t.Fatalf("newer = %v", got)
	}
}

func TestDialogs(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	send(t, svc, bob, ann, "от боба")
	send(t, svc, ann, cat, "кату")
	send(t, svc, dan, ann, "от дэна")
	send(t, svc, ann, bob, "бобу") // bob's dialog is the most recent again

	var (
		peers  []uuid.UUID
		cursor uuid.UUID
	)

	for {
		page, err := svc.Dialogs(ctx, ann, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}

		for _, d := range page.Dialogs {
			peers = append(peers, d.PeerID)
		}

		if page.NextCursor == uuid.Nil {
			break
		}
		cursor = page.NextCursor
	}

	if len(peers) != 3 || peers[0] != bob || peers[1] != dan || peers[2] != cat {
		t.Fatalf("dialogs: got %v, want bob, dan, cat", peers)
	}

	first, err := svc.Dialogs(ctx, ann, uuid.Nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d := first.Dialogs[0]; d.LastMessage.Text() != "бобу" || !d.HasUnread() {
		t.Fatalf("bob's dialog: last %q, unread %v; bob's message is still unread", d.LastMessage.Text(), d.HasUnread())
	}
}

func TestReading(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	fromBob := send(t, svc, bob, ann, "раз", "два")
	fromAnn := send(t, svc, ann, bob, "привет")[0]

	if n, err := svc.CountUnread(ctx, ann); err != nil || n != 1 {
		t.Fatalf("ann's unread dialogs = %d, %v", n, err)
	}
	if n, err := svc.CountUnread(ctx, bob); err != nil || n != 1 {
		t.Fatalf("bob's unread dialogs = %d, %v", n, err)
	}

	// Ann reads bob's first message only.
	if err := svc.MarkRead(ctx, ann, bob, fromBob[0].ID()); err != nil {
		t.Fatal(err)
	}

	page, err := svc.History(ctx, ann, bob, uuid.Nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Marks.IsRead(fromBob[0], ann) || page.Marks.IsRead(fromBob[1], ann) {
		t.Fatalf("marks = %+v: the first is read, the second is not", page.Marks)
	}
	if n, _ := svc.CountUnread(ctx, ann); n != 1 {
		t.Fatalf("one still unread: %d", n)
	}

	if err = svc.MarkRead(ctx, ann, bob, fromBob[1].ID()); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.CountUnread(ctx, ann); n != 0 {
		t.Fatalf("all read: %d", n)
	}

	// An older mark does not bring messages back.
	if err = svc.MarkRead(ctx, ann, bob, fromBob[0].ID()); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.CountUnread(ctx, ann); n != 0 {
		t.Fatalf("the mark moved back: %d", n)
	}

	// Bob sees ann has read his messages; his own unread is hers.
	bobs, err := svc.History(ctx, bob, ann, uuid.Nil, 50)
	if err != nil || !bobs.Marks.IsRead(fromBob[1], bob) || bobs.Marks.IsRead(fromAnn, bob) {
		t.Fatalf("bob's marks = %+v, %v", bobs.Marks, err)
	}

	tests := []struct {
		name string
		upTo uuid.UUID
	}{
		{"your own message", fromAnn.ID()},
		{"a made-up ID", uuid.Max},
	}

	for _, tt := range tests {
		if err = svc.MarkRead(ctx, ann, bob, tt.upTo); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("%s: %v", tt.name, err)
		}
	}

	// A message from someone else is not this dialog's.
	cat := uuid.New()
	fromCat := send(t, svc, cat, ann, "мяу")[0]
	if err = svc.MarkRead(ctx, ann, bob, fromCat.ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("another dialog's message: %v", err)
	}
}

func TestSend_ToYourself(t *testing.T) {
	t.Parallel()

	ann := uuid.New()
	if _, err := newService().Send(context.Background(), ann, ann, message.RestoreContent("сам себе", nil)); !errors.Is(err, message.ErrToYourself) {
		t.Fatalf("to yourself: %v", err)
	}
}

func TestCountUnread_Capped(t *testing.T) {
	t.Parallel()

	svc := newService()
	ann := uuid.New()

	for range service.MaxUnreadCount + 3 {
		send(t, svc, uuid.New(), ann, "привет")
	}

	if n, err := svc.CountUnread(context.Background(), ann); err != nil || n != service.MaxUnreadCount {
		t.Fatalf("unread = %d, %v; want the cap", n, err)
	}
}

func TestMessageService_HasMessagesFrom(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()

	has := func(user, peer uuid.UUID) bool {
		t.Helper()

		ok, err := svc.HasMessagesFrom(ctx, user, peer)
		if err != nil {
			t.Fatal(err)
		}

		return ok
	}

	if has(bob, ann) {
		t.Fatal("strangers have no messages")
	}

	send(t, svc, ann, bob, "привет")

	if !has(bob, ann) {
		t.Fatal("ann wrote to bob")
	}
	if has(ann, bob) {
		t.Fatal("bob has not written to ann yet")
	}
}

func TestMessageService_Events(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()

	annEvents, cancelAnn := svc.Subscribe(ann)
	defer cancelAnn()
	bobEvents, cancelBob := svc.Subscribe(bob)
	defer cancelBob()

	m := send(t, svc, bob, ann, "привет")[0]

	for name, events := range map[string]<-chan service.Event{"recipient": annEvents, "sender": bobEvents} {
		if e := <-events; e.Message == nil || e.Message.ID() != m.ID() {
			t.Fatalf("%s got %+v, want the message", name, e)
		}
	}

	if err := svc.MarkRead(ctx, ann, bob, m.ID()); err != nil {
		t.Fatal(err)
	}

	want := service.ReadMark{ReaderID: ann, PeerID: bob, UpTo: m.ID()}
	for name, events := range map[string]<-chan service.Event{"reader": annEvents, "peer": bobEvents} {
		if e := <-events; e.Read == nil || *e.Read != want {
			t.Fatalf("%s got %+v, want read mark %+v", name, e, want)
		}
	}

	// A failed mark tells nobody.
	if err := svc.MarkRead(ctx, bob, ann, m.ID()); err == nil {
		t.Fatal("bob cannot mark his own message read")
	}
	select {
	case e := <-annEvents:
		t.Fatalf("unexpected event %+v", e)
	default:
	}
}

func TestPurgeUser(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	send(t, svc, ann, bob, "привет")
	send(t, svc, bob, ann, "и тебе")
	send(t, svc, cat, ann, "ау")
	send(t, svc, bob, cat, "кот?")

	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("purge twice: %v", err)
	}

	// Both sides of ann's dialogs are gone; bob and cat keep theirs.
	for who, want := range map[uuid.UUID]int{ann: 0, bob: 1, cat: 1} {
		page, err := svc.Dialogs(ctx, who, uuid.Nil, 20)
		if err != nil || len(page.Dialogs) != want {
			t.Errorf("dialogs of %s = %d, %v; want %d", who, len(page.Dialogs), err, want)
		}
	}
	if page, err := svc.History(ctx, bob, ann, uuid.Nil, 20); err != nil || len(page.Messages) != 0 {
		t.Errorf("history with ann = %d, %v; want none", len(page.Messages), err)
	}
	if page, err := svc.History(ctx, bob, cat, uuid.Nil, 20); err != nil || len(page.Messages) != 1 {
		t.Errorf("history of bob and cat = %d, %v; want 1", len(page.Messages), err)
	}
}

// fakePhotos records what media is told, and whether in a transaction.
type fakePhotos struct {
	published, deleted []photoCall
	outsideTx          int
}

type photoCall struct {
	owner  uuid.UUID
	photos []uuid.UUID
}

type inTx struct{}

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(context.WithValue(ctx, inTx{}, true))
}

func (f *fakePhotos) Publish(ctx context.Context, owner uuid.UUID, photos []uuid.UUID) error {
	return f.record(ctx, &f.published, photoCall{owner, photos})
}

func (f *fakePhotos) Delete(ctx context.Context, owner uuid.UUID, photos []uuid.UUID) error {
	return f.record(ctx, &f.deleted, photoCall{owner, photos})
}

func (f *fakePhotos) record(ctx context.Context, to *[]photoCall, c photoCall) error {
	if ctx.Value(inTx{}) == nil {
		f.outsideTx++
	}
	*to = append(*to, c)
	return nil
}

func TestMessageService_Photos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewMessageService(service.Deps{
		Messages: store.Messages(), Dialogs: store.Dialogs(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ann, bob := uuid.New(), uuid.New()
	annsPhoto, bobsPhoto := uuid.New(), uuid.New()

	send := func(from, to, photo uuid.UUID) {
		t.Helper()

		content, err := message.NewContent("", message.Photos{photo})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Send(ctx, from, to, content); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	send(ann, bob, annsPhoto)
	send(bob, ann, bobsPhoto)
	if len(photos.published) != 2 || photos.published[0].owner != ann || photos.published[1].owner != bob {
		t.Fatalf("published = %+v, want each sender's photo", photos.published)
	}

	// Ann's page is erased: bob's photo in their dialog goes; hers media
	// erases with the rest of her.
	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != bob || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{bobsPhoto}) {
		t.Fatalf("deleted = %+v, want bob's photo", photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Fatalf("%d photo calls outside the transaction", photos.outsideTx)
	}
}
