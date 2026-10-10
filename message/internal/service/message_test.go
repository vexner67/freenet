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
		m, err := svc.Send(context.Background(), from, to, message.RestoreContent(message.Text(text), nil, uuid.Nil))
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
	if _, err := newService().Send(context.Background(), ann, ann, message.RestoreContent("сам себе", nil, uuid.Nil)); !errors.Is(err, message.ErrToYourself) {
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

	// A failed mark or deletion tells nobody.
	if err := svc.MarkRead(ctx, bob, ann, m.ID()); err == nil {
		t.Fatal("bob cannot mark his own message read")
	}
	if err := svc.Delete(ctx, ann, m.ID()); err == nil {
		t.Fatal("ann cannot delete bob's message")
	}
	select {
	case e := <-annEvents:
		t.Fatalf("unexpected event %+v", e)
	default:
	}

	if err := svc.Delete(ctx, bob, m.ID()); err != nil {
		t.Fatal(err)
	}

	gone := service.Deleted{ID: m.ID(), SenderID: bob, RecipientID: ann}
	for name, events := range map[string]<-chan service.Event{"recipient": annEvents, "sender": bobEvents} {
		if e := <-events; e.Deleted == nil || *e.Deleted != gone {
			t.Fatalf("%s got %+v, want deletion %+v", name, e, gone)
		}
	}
}

func TestMessageService_Delete(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	fromAnn := send(t, svc, ann, bob, "привет", "ой, не то")
	fromBob := send(t, svc, bob, ann, "что?")[0]

	tests := []struct {
		name      string
		requester uuid.UUID
		id        uuid.UUID
		err       error
	}{
		{"the recipient", bob, fromAnn[1].ID(), message.ErrCannotDelete},
		{"a stranger cannot even tell it exists", cat, fromAnn[1].ID(), message.ErrNotFound},
		{"a made-up ID", ann, uuid.Max, message.ErrNotFound},
	}
	for _, tt := range tests {
		if err := svc.Delete(ctx, tt.requester, tt.id); !errors.Is(err, tt.err) {
			t.Fatalf("%s: Delete = %v, want %v", tt.name, err, tt.err)
		}
	}

	if err := svc.Delete(ctx, ann, fromAnn[1].ID()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, ann, fromAnn[1].ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}

	for who, peer := range map[uuid.UUID]uuid.UUID{ann: bob, bob: ann} {
		page, err := svc.History(ctx, who, peer, uuid.Nil, 50)
		if got := texts(page.Messages); err != nil || !slices.Equal(got, []string{"привет", "что?"}) {
			t.Fatalf("history = %v, %v; the message is gone from both sides", got, err)
		}
	}

	// Bob takes his message back too: ann's first is the last of the
	// dialog for both, ann has nothing unread, and bob is no longer
	// someone who wrote to her first.
	if err := svc.Delete(ctx, bob, fromBob.ID()); err != nil {
		t.Fatal(err)
	}
	for _, who := range []uuid.UUID{ann, bob} {
		page, err := svc.Dialogs(ctx, who, uuid.Nil, 20)
		if err != nil || len(page.Dialogs) != 1 || page.Dialogs[0].LastMessage.ID() != fromAnn[0].ID() {
			t.Fatalf("dialogs = %+v, %v; want the first message last", page.Dialogs, err)
		}
	}
	if n, _ := svc.CountUnread(ctx, ann); n != 0 {
		t.Fatalf("ann's unread = %d: bob took his message back", n)
	}
	if ok, _ := svc.HasMessagesFrom(ctx, ann, bob); ok {
		t.Fatal("bob has no messages to ann left")
	}

	// The last message gone, the dialog leaves both lists.
	if err := svc.Delete(ctx, ann, fromAnn[0].ID()); err != nil {
		t.Fatal(err)
	}
	for _, who := range []uuid.UUID{ann, bob} {
		if page, err := svc.Dialogs(ctx, who, uuid.Nil, 20); err != nil || len(page.Dialogs) != 0 {
			t.Fatalf("dialogs = %+v, %v; want none", page.Dialogs, err)
		}
	}
	if n, _ := svc.CountUnread(ctx, bob); n != 0 {
		t.Fatalf("bob's unread = %d", n)
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

		content, err := message.NewContent("", message.Photos{photo}, uuid.Nil)
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

	// A deleted message takes its photos along.
	annsSecond := uuid.New()
	send(ann, bob, annsSecond)
	page, err := svc.History(ctx, ann, bob, uuid.Nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(ctx, ann, page.Messages[0].ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != ann || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{annsSecond}) {
		t.Fatalf("deleted = %+v, want ann's second photo", photos.deleted)
	}
	photos.deleted = nil

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

func TestMessageService_Hide(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	fromAnn := send(t, svc, ann, bob, "привет", "как дела?")
	fromBob := send(t, svc, bob, ann, "хорошо")[0]

	annEvents, cancelAnn := svc.Subscribe(ann)
	defer cancelAnn()
	bobEvents, cancelBob := svc.Subscribe(bob)
	defer cancelBob()

	if err := svc.Hide(ctx, cat, fromBob.ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a stranger hides: %v", err)
	}

	// Ann deletes bob's message for herself: he keeps it and hears nothing.
	if err := svc.Hide(ctx, ann, fromBob.ID()); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	if e := <-annEvents; e.Deleted == nil || e.Deleted.ID != fromBob.ID() {
		t.Fatalf("ann's other pages got %+v", e)
	}
	select {
	case e := <-bobEvents:
		t.Fatalf("bob heard %+v", e)
	default:
	}

	anns, _ := svc.History(ctx, ann, bob, uuid.Nil, 50)
	bobs, _ := svc.History(ctx, bob, ann, uuid.Nil, 50)
	if !slices.Equal(texts(anns.Messages), []string{"привет", "как дела?"}) || len(bobs.Messages) != 3 {
		t.Fatalf("ann has %v, bob %v", texts(anns.Messages), texts(bobs.Messages))
	}

	// Her list of dialogs falls back to what she has left, with nothing
	// unread; his is as it was.
	if page, _ := svc.Dialogs(ctx, ann, uuid.Nil, 20); len(page.Dialogs) != 1 || page.Dialogs[0].LastMessage.ID() != fromAnn[1].ID() || page.Dialogs[0].HasUnread() {
		t.Fatalf("ann's dialogs = %+v", page.Dialogs)
	}
	if n, _ := svc.CountUnread(ctx, ann); n != 0 {
		t.Fatalf("ann's unread = %d", n)
	}
	if page, _ := svc.Dialogs(ctx, bob, uuid.Nil, 20); page.Dialogs[0].LastMessage.ID() != fromBob.ID() {
		t.Fatalf("bob's dialogs = %+v", page.Dialogs)
	}

	// What she deleted she neither hides, deletes, edits, reads nor finds.
	if err := svc.Hide(ctx, ann, fromBob.ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("hidden twice: %v", err)
	}
	if err := svc.MarkRead(ctx, ann, bob, fromBob.ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("reading up to a hidden message: %v", err)
	}
	q, _ := message.NewQuery("хорошо")
	if page, _ := svc.Search(ctx, ann, uuid.Nil, q, uuid.Nil, 20); len(page.Messages) != 0 {
		t.Fatalf("ann finds a hidden message: %v", texts(page.Messages))
	}

	// Her own message she hides from herself; he keeps it. Then she cannot
	// take it back from both.
	if err := svc.Hide(ctx, ann, fromAnn[1].ID()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, ann, fromAnn[1].ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("deleting for all what she hid: %v", err)
	}
	if _, err := svc.Edit(ctx, ann, fromAnn[1].ID(), "x"); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("editing what she hid: %v", err)
	}

	// Hidden by both, a message is gone for good.
	if err := svc.Hide(ctx, bob, fromAnn[1].ID()); err != nil {
		t.Fatal(err)
	}
	if bobs, _ = svc.History(ctx, bob, ann, uuid.Nil, 50); !slices.Equal(texts(bobs.Messages), []string{"привет", "хорошо"}) {
		t.Fatalf("bob has %v", texts(bobs.Messages))
	}

	// Everything hidden, the dialog leaves her list, not his.
	if err := svc.Hide(ctx, ann, fromAnn[0].ID()); err != nil {
		t.Fatal(err)
	}
	if page, _ := svc.Dialogs(ctx, ann, uuid.Nil, 20); len(page.Dialogs) != 0 {
		t.Fatalf("ann's dialogs = %+v", page.Dialogs)
	}
	if page, _ := svc.Dialogs(ctx, bob, uuid.Nil, 20); len(page.Dialogs) != 1 {
		t.Fatalf("bob's dialogs = %+v", page.Dialogs)
	}
}

func TestMessageService_HidePhotos(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewMessageService(service.Deps{
		Messages: store.Messages(), Dialogs: store.Dialogs(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	photo := uuid.New()

	m, err := svc.Send(ctx, ann, bob, message.RestoreContent("", message.Photos{photo}, uuid.Nil))
	if err != nil {
		t.Fatal(err)
	}

	// The photo stays while one of them has the message.
	if err = svc.Hide(ctx, bob, m.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 0 {
		t.Fatalf("deleted %+v while ann has the message", photos.deleted)
	}
	if err = svc.Hide(ctx, ann, m.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != ann || photos.deleted[0].photos[0] != photo {
		t.Fatalf("deleted %+v, want ann's photo", photos.deleted)
	}
}

func TestMessageService_Edit(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	m := send(t, svc, ann, bob, "привет")[0]

	annEvents, cancelAnn := svc.Subscribe(ann)
	defer cancelAnn()
	bobEvents, cancelBob := svc.Subscribe(bob)
	defer cancelBob()

	for name, tt := range map[string]struct {
		requester uuid.UUID
		text      message.Text
		err       error
	}{
		"the recipient": {bob, "x", message.ErrCannotEdit},
		"a stranger":    {cat, "x", message.ErrNotFound},
		"no text":       {ann, "", message.ErrTextEmpty},
	} {
		if _, err := svc.Edit(ctx, tt.requester, m.ID(), tt.text); !errors.Is(err, tt.err) {
			t.Fatalf("%s: %v, want %v", name, err, tt.err)
		}
	}
	if _, err := svc.Edit(ctx, ann, uuid.New(), "x"); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("no message: %v", err)
	}

	edited, err := svc.Edit(ctx, ann, m.ID(), "привет!")
	if err != nil || edited.Text() != "привет!" || edited.EditedAt().IsZero() {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}
	for name, events := range map[string]<-chan service.Event{"sender": annEvents, "recipient": bobEvents} {
		if e := <-events; e.Edited == nil || e.Edited.ID() != m.ID() || e.Edited.Text() != "привет!" {
			t.Fatalf("%s got %+v, want the edit", name, e)
		}
	}

	page, _ := svc.History(ctx, bob, ann, uuid.Nil, 50)
	if len(page.Messages) != 1 || page.Messages[0].Text() != "привет!" || page.Messages[0].EditedAt().IsZero() {
		t.Fatalf("bob reads %+v", page.Messages)
	}
	if dialogs, _ := svc.Dialogs(ctx, bob, uuid.Nil, 20); dialogs.Dialogs[0].LastMessage.Text() != "привет!" {
		t.Fatalf("bob's dialog says %q", dialogs.Dialogs[0].LastMessage.Text())
	}

	// Who deleted the message for themselves does not hear of its edits.
	if err = svc.Hide(ctx, bob, m.ID()); err != nil {
		t.Fatal(err)
	}
	<-bobEvents // his own deletion
	if _, err = svc.Edit(ctx, ann, m.ID(), "пока"); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-bobEvents:
		t.Fatalf("bob heard %+v", e)
	default:
	}
}

func TestMessageService_Search(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	send(t, svc, ann, bob, "Еду на море", "горы")
	send(t, svc, bob, ann, "МОРЕ волнуется")
	send(t, svc, cat, ann, "на море хорошо")
	send(t, svc, bob, cat, "море не для ann")

	q, _ := message.NewQuery("море")

	all, err := svc.Search(ctx, ann, uuid.Nil, q, uuid.Nil, 2)
	if err != nil || !slices.Equal(texts(all.Messages), []string{"на море хорошо", "МОРЕ волнуется"}) || all.NextCursor == uuid.Nil {
		t.Fatalf("the first page = %v, %v", texts(all.Messages), err)
	}
	rest, err := svc.Search(ctx, ann, uuid.Nil, q, all.NextCursor, 2)
	if err != nil || !slices.Equal(texts(rest.Messages), []string{"Еду на море"}) || rest.NextCursor != uuid.Nil {
		t.Fatalf("the second page = %v, %v", texts(rest.Messages), err)
	}

	withBob, err := svc.Search(ctx, ann, bob, q, uuid.Nil, 20)
	if err != nil || !slices.Equal(texts(withBob.Messages), []string{"МОРЕ волнуется", "Еду на море"}) {
		t.Fatalf("with bob = %v, %v", texts(withBob.Messages), err)
	}
}
