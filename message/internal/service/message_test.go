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
		Messages: store.Messages(), Dialogs: store.Dialogs(), Keys: sameKeys{}, ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: inmem.TxManager{}, Photos: mediaclient.Nop{},
	})
}

func send(t *testing.T, svc *service.MessageService, from, to uuid.UUID, texts ...string) []*message.Message {
	t.Helper()

	sent := make([]*message.Message, 0, len(texts))
	for _, text := range texts {
		m, err := svc.Send(context.Background(), from, to, service.Draft{Sealed: seal(text)})
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
		out[i] = opened(m)
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
	if d := first.Dialogs[0]; opened(d.LastMessage) != "бобу" || !d.HasUnread() {
		t.Fatalf("bob's dialog: last %q, unread %v; bob's message is still unread", opened(d.LastMessage), d.HasUnread())
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
	if _, err := newService().Send(context.Background(), ann, ann, service.Draft{Sealed: seal("сам себе")}); !errors.Is(err, message.ErrToYourself) {
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

	// Taken back, the message says nothing: both hear what it is now.
	for name, events := range map[string]<-chan service.Event{"recipient": annEvents, "sender": bobEvents} {
		if e := <-events; e.Edited == nil || e.Edited.ID() != m.ID() || !e.Edited.IsDeleted() || e.Edited.Sealed() != "" {
			t.Fatalf("%s got %+v, want the message taken back", name, e)
		}
	}

	// Deleted by ann for herself, it is gone from her pages only.
	if err := svc.Hide(ctx, ann, m.ID()); err != nil {
		t.Fatal(err)
	}
	gone := service.Deleted{ID: m.ID(), SenderID: bob, RecipientID: ann}
	if e := <-annEvents; e.Deleted == nil || *e.Deleted != gone {
		t.Fatalf("ann got %+v, want deletion %+v", e, gone)
	}
	select {
	case e := <-bobEvents:
		t.Fatalf("unexpected event %+v", e)
	default:
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

	// Taken back, the message stays where it was for both and says
	// nothing: the peer sees that something was there.
	for who, peer := range map[uuid.UUID]uuid.UUID{ann: bob, bob: ann} {
		page, err := svc.History(ctx, who, peer, uuid.Nil, 50)
		if got := texts(page.Messages); err != nil || !slices.Equal(got, []string{"привет", "", "что?"}) {
			t.Fatalf("history = %v, %v; want the message empty in its place", got, err)
		}
		if gone := page.Messages[1]; !gone.IsDeleted() || gone.ID() != fromAnn[1].ID() {
			t.Fatalf("message = %+v, want it taken back", gone)
		}
	}

	// It can be neither edited nor forwarded.
	if _, err := svc.Edit(ctx, ann, fromAnn[1].ID(), message.Revision{Sealed: seal("снова")}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("edit a message taken back: %v", err)
	}
	if _, err := svc.Send(ctx, ann, cat, service.Draft{Forward: []uuid.UUID{fromAnn[1].ID()}}); !errors.Is(err, message.ErrForwardNotFound) {
		t.Fatalf("forward a message taken back: %v", err)
	}

	// Bob takes his message back too: it is still the last of the dialog
	// and still unread, and bob is still someone who wrote to ann.
	if err := svc.Delete(ctx, bob, fromBob.ID()); err != nil {
		t.Fatal(err)
	}
	for _, who := range []uuid.UUID{ann, bob} {
		page, err := svc.Dialogs(ctx, who, uuid.Nil, 20)
		if err != nil || len(page.Dialogs) != 1 || page.Dialogs[0].LastMessage.ID() != fromBob.ID() || !page.Dialogs[0].LastMessage.IsDeleted() {
			t.Fatalf("dialogs = %+v, %v; want the message taken back last", page.Dialogs, err)
		}
	}
	if n, _ := svc.CountUnread(ctx, ann); n != 1 {
		t.Fatalf("ann's unread = %d, want what bob took back", n)
	}
	if ok, _ := svc.HasMessagesFrom(ctx, ann, bob); !ok {
		t.Fatal("bob did write to ann")
	}
	if err := svc.MarkRead(ctx, ann, bob, fromBob.ID()); err != nil {
		t.Fatalf("read a message taken back: %v", err)
	}

	// Each side deletes what was taken back for themselves; once both
	// have, it is gone for good.
	if err := svc.Hide(ctx, ann, fromBob.ID()); err != nil {
		t.Fatal(err)
	}
	if page, _ := svc.History(ctx, bob, ann, uuid.Nil, 50); len(page.Messages) != 3 {
		t.Fatalf("bob has %d messages, want all three", len(page.Messages))
	}
	if err := svc.Hide(ctx, bob, fromBob.ID()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Hide(ctx, bob, fromBob.ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("hide a message gone for good: %v", err)
	}
}

func TestMessageService_Forward(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewMessageService(service.Deps{
		Messages: store.Messages(), Dialogs: store.Dialogs(), Keys: sameKeys{}, ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	photo, post := uuid.New(), uuid.New()

	first, err := svc.Send(ctx, bob, ann, service.Draft{Sealed: seal("смотри"), Photos: message.Photos{photo}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Send(ctx, ann, bob, service.Draft{Post: post})
	if err != nil {
		t.Fatal(err)
	}

	// Ann forwards both to cat, newest named first: they go oldest first,
	// the same one once, and bob's photo is kept once more under a new ID.
	fwd, err := svc.Send(ctx, ann, cat, service.Draft{Sealed: seal("вот"), Forward: []uuid.UUID{second.ID(), first.ID(), second.ID()}})
	if err != nil {
		t.Fatal(err)
	}

	carried := fwd.Forwarded()
	if len(carried) != 2 || carried[0].SenderID != bob || carried[0].Text != "" || carried[1].SenderID != ann || carried[1].Post != post {
		t.Fatalf("forwarded = %+v", carried)
	}
	if !carried[0].CreatedAt.Equal(first.CreatedAt()) {
		t.Fatalf("forwarded at %v, want when it was written, %v", carried[0].CreatedAt, first.CreatedAt())
	}
	if len(carried[0].Photos) != 1 || carried[0].Photos[0] == photo {
		t.Fatalf("photos = %v, want a copy of %v", carried[0].Photos, photo)
	}
	if len(photos.copied) != 1 || photos.copied[0].owner != bob || photos.copied[0].copies[0] != (service.PhotoCopy{From: photo, To: carried[0].Photos[0]}) {
		t.Fatalf("copied = %+v, want bob's photo", photos.copied)
	}

	// Cat reads it as it was sent, and forwards it on with nothing of her
	// own: ann's message, and under it what that carried.
	page, err := svc.History(ctx, cat, ann, uuid.Nil, 20)
	if err != nil || len(page.Messages) != 1 || len(page.Messages[0].Forwarded()) != 2 {
		t.Fatalf("cat's history = %+v, %v", page.Messages, err)
	}

	again, err := svc.Send(ctx, cat, dan, service.Draft{Forward: []uuid.UUID{fwd.ID()}})
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Forwarded(); len(got) != 3 || got[0].SenderID != ann || got[0].Text != "" || got[1].SenderID != bob || got[1].Photos[0] == carried[0].Photos[0] ||
		got[0].Depth != 0 || got[1].Depth != 1 || got[2].Depth != 1 {
		t.Fatalf("forwarded again = %+v", got)
	}

	// Only what the sender has in their dialogs is forwarded.
	tests := []struct {
		name    string
		sender  uuid.UUID
		forward []uuid.UUID
		err     error
	}{
		{"a message of others", dan, []uuid.UUID{first.ID()}, message.ErrForwardNotFound},
		{"a made-up ID", ann, []uuid.UUID{uuid.Max}, message.ErrForwardNotFound},
		{"too many", ann, make([]uuid.UUID, message.MaxForwarded+1), message.ErrTooManyForwarded},
		{"nothing at all", ann, nil, message.ErrTextEmpty},
	}
	for _, tt := range tests {
		if _, err := svc.Send(ctx, tt.sender, cat, service.Draft{Forward: tt.forward}); !errors.Is(err, tt.err) {
			t.Fatalf("%s: Send = %v, want %v", tt.name, err, tt.err)
		}
	}

	// Deleted for herself, a message is no longer ann's to forward.
	if err := svc.Hide(ctx, ann, first.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, ann, cat, service.Draft{Forward: []uuid.UUID{first.ID()}}); !errors.Is(err, message.ErrForwardNotFound) {
		t.Fatalf("forward a hidden message: %v", err)
	}

	// Taken back, the forwarding message takes the copies along; the
	// message it was made of keeps its photo.
	photos.deleted = nil
	if err := svc.Delete(ctx, ann, fwd.ID()); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != bob || !slices.Equal(photos.deleted[0].photos, []uuid.UUID(carried[0].Photos)) {
		t.Fatalf("deleted = %+v, want the copy of bob's photo", photos.deleted)
	}

	// Bob's page is erased: the copy in the dialog of cat and dan is his
	// and goes with him, nothing to tell media about it.
	photos.deleted = nil
	if err := svc.PurgeUser(ctx, dan); err != nil {
		t.Fatal(err)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != bob {
		t.Fatalf("deleted = %+v, want the copy of bob's photo in dan's dialog", photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Fatalf("%d photo calls outside the transaction", photos.outsideTx)
	}
}

func TestAround(t *testing.T) {
	t.Parallel()

	svc := newService()
	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	sent := send(t, svc, ann, bob, "1", "2", "3", "4", "5", "6", "7", "8", "9")
	other := send(t, svc, ann, cat, "другой диалог")[0]

	page, err := svc.Around(ctx, bob, ann, sent[4].ID(), 4)
	if got := texts(page.Messages); err != nil || !slices.Equal(got, []string{"3", "4", "5", "6", "7"}) {
		t.Fatalf("around = %v, %v", got, err)
	}
	if page.NextCursor != sent[2].ID() {
		t.Fatalf("next cursor = %v, want the oldest shown", page.NextCursor)
	}

	// Near the start nothing older is left.
	page, err = svc.Around(ctx, ann, bob, sent[1].ID(), 4)
	if got := texts(page.Messages); err != nil || !slices.Equal(got, []string{"1", "2", "3", "4"}) || page.NextCursor != uuid.Nil {
		t.Fatalf("around the second = %v, %v, cursor %v", got, err, page.NextCursor)
	}

	tests := []struct {
		name       string
		user, peer uuid.UUID
		id         uuid.UUID
	}{
		{"a message of another dialog", ann, bob, other.ID()},
		{"a stranger", cat, ann, sent[4].ID()},
		{"a made-up ID", ann, bob, uuid.Max},
	}
	for _, tt := range tests {
		if _, err := svc.Around(ctx, tt.user, tt.peer, tt.id, 4); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("%s: Around = %v, want not found", tt.name, err)
		}
	}

	// Deleted for herself, the message is not ann's to open.
	if err := svc.Hide(ctx, ann, sent[4].ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Around(ctx, ann, bob, sent[4].ID(), 4); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("around a hidden message: %v", err)
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
	copied             []copyCall
	outsideTx          int
}

type copyCall struct {
	owner  uuid.UUID
	copies []service.PhotoCopy
}

func (f *fakePhotos) Copy(ctx context.Context, owner uuid.UUID, copies []service.PhotoCopy) error {
	if ctx.Value(inTx{}) == nil {
		f.outsideTx++
	}
	f.copied = append(f.copied, copyCall{owner, copies})
	return nil
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
		Messages: store.Messages(), Dialogs: store.Dialogs(), Keys: sameKeys{}, ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ann, bob := uuid.New(), uuid.New()
	annsPhoto, bobsPhoto := uuid.New(), uuid.New()

	send := func(from, to, photo uuid.UUID) {
		t.Helper()

		if _, err := svc.Send(ctx, from, to, service.Draft{Photos: message.Photos{photo}}); err != nil {
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

	// Her own message she hides from herself; he keeps it. Then she cannot
	// take it back from both.
	if err := svc.Hide(ctx, ann, fromAnn[1].ID()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, ann, fromAnn[1].ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("deleting for all what she hid: %v", err)
	}
	if _, err := svc.Edit(ctx, ann, fromAnn[1].ID(), message.Revision{Sealed: seal("x")}); !errors.Is(err, message.ErrNotFound) {
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
		Messages: store.Messages(), Dialogs: store.Dialogs(), Keys: sameKeys{}, ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	photo := uuid.New()

	m, err := svc.Send(ctx, ann, bob, service.Draft{Photos: message.Photos{photo}})
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
		if _, err := svc.Edit(ctx, tt.requester, m.ID(), message.Revision{Text: tt.text}); !errors.Is(err, tt.err) {
			t.Fatalf("%s: %v, want %v", name, err, tt.err)
		}
	}
	if _, err := svc.Edit(ctx, ann, uuid.New(), message.Revision{Sealed: seal("x")}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("no message: %v", err)
	}

	edited, err := svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("привет!")})
	if err != nil || opened(edited) != "привет!" || edited.EditedAt().IsZero() {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}
	for name, events := range map[string]<-chan service.Event{"sender": annEvents, "recipient": bobEvents} {
		if e := <-events; e.Edited == nil || e.Edited.ID() != m.ID() || opened(e.Edited) != "привет!" {
			t.Fatalf("%s got %+v, want the edit", name, e)
		}
	}

	page, _ := svc.History(ctx, bob, ann, uuid.Nil, 50)
	if len(page.Messages) != 1 || opened(page.Messages[0]) != "привет!" || page.Messages[0].EditedAt().IsZero() {
		t.Fatalf("bob reads %+v", page.Messages)
	}
	if dialogs, _ := svc.Dialogs(ctx, bob, uuid.Nil, 20); opened(dialogs.Dialogs[0].LastMessage) != "привет!" {
		t.Fatalf("bob's dialog says %q", opened(dialogs.Dialogs[0].LastMessage))
	}

	// Who deleted the message for themselves does not hear of its edits.
	if err = svc.Hide(ctx, bob, m.ID()); err != nil {
		t.Fatal(err)
	}
	<-bobEvents // his own deletion
	if _, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("пока")}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-bobEvents:
		t.Fatalf("bob heard %+v", e)
	default:
	}
}

func TestMessageService_EditPhotos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewMessageService(service.Deps{
		Messages: store.Messages(), Dialogs: store.Dialogs(), Keys: sameKeys{}, ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ann, bob := uuid.New(), uuid.New()
	kept, dropped, added := uuid.New(), uuid.New(), uuid.New()

	m, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: seal("смотри"), Photos: message.Photos{kept, dropped}})
	if err != nil {
		t.Fatal(err)
	}
	photos.published = nil

	// An edit that says nothing of photos keeps them.
	edited, err := svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("гляди")})
	if err != nil || !slices.Equal(edited.Photos(), message.Photos{kept, dropped}) || len(photos.published)+len(photos.deleted) != 0 {
		t.Fatalf("the photos after an edit of the words = %v, %v; media heard %+v, %+v", edited.Photos(), err, photos.published, photos.deleted)
	}

	// The new photos are kept, those taken away deleted, the rest left.
	edited, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("гляди"), Removed: message.Photos{dropped}, Added: message.Photos{added}})
	if err != nil || !slices.Equal(edited.Photos(), message.Photos{kept, added}) {
		t.Fatalf("the photos = %v, %v", edited.Photos(), err)
	}
	if len(photos.published) != 1 || photos.published[0].owner != ann || !slices.Equal(photos.published[0].photos, []uuid.UUID{added}) {
		t.Fatalf("published = %+v", photos.published)
	}
	if len(photos.deleted) != 1 || photos.deleted[0].owner != ann || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{dropped}) {
		t.Fatalf("deleted = %+v", photos.deleted)
	}
	if page, _ := svc.History(ctx, bob, ann, uuid.Nil, 1); !slices.Equal(page.Messages[0].Photos(), message.Photos{kept, added}) {
		t.Fatalf("bob sees %v", page.Messages[0].Photos())
	}

	// Another window, which still shows the photo dropped, adds one more:
	// it changes the message as the first edit left it.
	late := uuid.New()
	if edited, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("гляди"), Added: message.Photos{late}}); err != nil || !slices.Equal(edited.Photos(), message.Photos{kept, added, late}) {
		t.Fatalf("the photos after an edit from another window = %v, %v", edited.Photos(), err)
	}
	if len(photos.published) != 2 || len(photos.deleted) != 1 {
		t.Fatalf("media heard %+v, %+v", photos.published, photos.deleted)
	}

	// All photos away: the words stay. Without words, nothing would.
	all := message.Photos{kept, added, late}
	photos.published, photos.deleted = nil, nil
	if _, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Removed: all}); !errors.Is(err, message.ErrTextEmpty) {
		t.Fatalf("an edit to nothing: %v", err)
	}
	if edited, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("слова"), Removed: all}); err != nil || len(edited.Photos()) != 0 {
		t.Fatalf("the photos taken away = %v, %v", edited.Photos(), err)
	}
	if len(photos.published) != 0 || len(photos.deleted) != 1 || len(photos.deleted[0].photos) != 3 {
		t.Fatalf("media heard %+v, %+v", photos.published, photos.deleted)
	}
	if photos.outsideTx != 0 {
		t.Fatalf("%d photo calls outside the transaction", photos.outsideTx)
	}

	// A message that shares a photo of an album takes none of its own.
	shared, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: seal("фото"), SharedPhoto: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Edit(ctx, ann, shared.ID(), message.Revision{Sealed: seal("фото"), Added: message.Photos{added}}); !errors.Is(err, message.ErrSharedPhotoAlone) {
		t.Fatalf("photos for a shared photo: %v", err)
	}
	if _, err = svc.Edit(ctx, ann, shared.ID(), message.Revision{Sealed: seal("фото!")}); err != nil {
		t.Fatalf("an edit of its words: %v", err)
	}
}

func TestMessageService_DeleteMany(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewMessageService(service.Deps{
		Messages: store.Messages(), Dialogs: store.Dialogs(), Keys: sameKeys{}, ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(), Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	photo := uuid.New()

	fromAnn := send(t, svc, ann, bob, "раз", "два", "три")
	fromBob := send(t, svc, bob, ann, "ответ")[0]
	withPhoto, err := svc.Send(ctx, ann, bob, service.Draft{Photos: message.Photos{photo}})
	if err != nil {
		t.Fatal(err)
	}
	toCat := send(t, svc, ann, cat, "кошке")[0]

	history := func(user, peer uuid.UUID) []string {
		t.Helper()

		page, err := svc.History(ctx, user, peer, uuid.Nil, 20)
		if err != nil {
			t.Fatal(err)
		}

		out := make([]string, len(page.Messages))
		for i, m := range page.Messages {
			out[i] = opened(m)
			if m.IsDeleted() {
				out[i] = "-"
			}
		}

		return out
	}

	// All or none: the peer's message cannot be taken back, a message of
	// someone else's dialog is not there, and nothing changes.
	for name, c := range map[string]struct {
		ids []uuid.UUID
		err error
	}{
		"the peer's":       {[]uuid.UUID{fromAnn[0].ID(), fromBob.ID()}, message.ErrCannotDelete},
		"none":             {nil, message.ErrNoMessages},
		"unknown":          {[]uuid.UUID{fromAnn[0].ID(), uuid.New()}, message.ErrNotFound},
		"not ann's at all": {[]uuid.UUID{fromAnn[0].ID(), send(t, svc, bob, cat, "чужое")[0].ID()}, message.ErrNotFound},
	} {
		if err := svc.DeleteMany(ctx, ann, c.ids, true); !errors.Is(err, c.err) {
			t.Fatalf("%s: %v, want %v", name, err, c.err)
		}
	}
	many := make([]uuid.UUID, message.MaxBatch+1)
	for i := range many {
		many[i] = uuid.New()
	}
	if err := svc.DeleteMany(ctx, ann, many, false); !errors.Is(err, message.ErrTooManyMessages) {
		t.Fatalf("too many: %v", err)
	}
	if got := history(bob, ann); !slices.Equal(got, []string{"раз", "два", "три", "ответ", ""}) {
		t.Fatalf("a failed batch changed the dialog: %v", got)
	}

	annEvents, cancelAnn := svc.Subscribe(ann)
	defer cancelAnn()
	bobEvents, cancelBob := svc.Subscribe(bob)
	defer cancelBob()

	// For herself: her own, bob's and one of another dialog, one named
	// twice. Bob keeps his side and hears nothing.
	if err := svc.DeleteMany(ctx, ann, []uuid.UUID{fromAnn[0].ID(), fromBob.ID(), toCat.ID(), fromAnn[0].ID()}, false); err != nil {
		t.Fatalf("DeleteMany for herself: %v", err)
	}
	if got := history(ann, bob); !slices.Equal(got, []string{"два", "три", ""}) {
		t.Fatalf("ann's side = %v", got)
	}
	if got := history(bob, ann); len(got) != 5 {
		t.Fatalf("bob's side = %v", got)
	}
	if got := history(ann, cat); len(got) != 0 {
		t.Fatalf("ann's dialog with cat = %v", got)
	}
	for range 3 {
		if e := <-annEvents; e.Deleted == nil {
			t.Fatalf("ann hears %+v, want a deleted message", e)
		}
	}

	// For everyone: both hear what each message is now, the photo goes.
	if err := svc.DeleteMany(ctx, ann, []uuid.UUID{fromAnn[1].ID(), withPhoto.ID()}, true); err != nil {
		t.Fatalf("DeleteMany for everyone: %v", err)
	}
	if got := history(bob, ann); !slices.Equal(got, []string{"раз", "-", "три", "ответ", "-"}) {
		t.Fatalf("bob's side = %v", got)
	}
	for _, events := range []<-chan service.Event{annEvents, bobEvents} {
		for range 2 {
			if e := <-events; e.Edited == nil || !e.Edited.IsDeleted() {
				t.Fatalf("heard %+v, want a message taken back", e)
			}
		}
	}
	if len(photos.deleted) != 1 || !slices.Equal(photos.deleted[0].photos, []uuid.UUID{photo}) || photos.outsideTx != 0 {
		t.Fatalf("deleted photos = %+v, %d outside the transaction", photos.deleted, photos.outsideTx)
	}

	// Taken back already, one fails the batch.
	if err := svc.DeleteMany(ctx, ann, []uuid.UUID{fromAnn[2].ID(), fromAnn[1].ID()}, true); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("taking back twice: %v", err)
	}
	if got := history(bob, ann); got[2] != "три" {
		t.Fatalf("the failed batch took a message back: %v", got)
	}

	// Hidden by bob too, ann's first message goes for good.
	if err := svc.DeleteMany(ctx, bob, []uuid.UUID{fromAnn[0].ID(), fromBob.ID()}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Messages().Get(ctx, fromAnn[0].ID()); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a message hidden by both: %v", err)
	}
}

func TestMessageService_Typing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()

	annEvents, cancelAnn := svc.Subscribe(ann)
	defer cancelAnn()
	bobEvents, cancelBob := svc.Subscribe(bob)
	defer cancelBob()

	if err := svc.Typing(ctx, bob, bob); !errors.Is(err, message.ErrToYourself) {
		t.Fatalf("typing to yourself: %v", err)
	}

	if err := svc.Typing(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}

	want := service.Typing{UserID: bob}
	if e := <-annEvents; e.Typing == nil || *e.Typing != want {
		t.Fatalf("ann got %+v, want typing %+v", e, want)
	}

	// The one who types is not told.
	select {
	case e := <-bobEvents:
		t.Fatalf("unexpected event %+v", e)
	default:
	}
}
