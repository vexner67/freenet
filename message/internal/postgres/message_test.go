package postgres_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func TestRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	save := func(from, to uuid.UUID, text string) *message.Message {
		t.Helper()
		m, err := message.New(from, to, message.RestoreContent(message.Text(text), nil, uuid.Nil, nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = messages.Save(ctx, m); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return m
	}

	m1 := save(bob, ann, "раз")
	m2 := save(ann, bob, "два")
	m3 := save(bob, ann, "три")
	toCat := save(ann, cat, "кату")

	t.Run("who has written to whom", func(t *testing.T) {
		for _, tt := range []struct {
			user, peer uuid.UUID
			want       bool
		}{
			{ann, bob, true},  // bob wrote to ann
			{cat, ann, true},  // ann wrote to cat
			{ann, cat, false}, // cat never answered
			{bob, cat, false}, // no dialog at all
		} {
			if got, err := dialogs.HasIncoming(ctx, tt.user, tt.peer); err != nil || got != tt.want {
				t.Fatalf("HasIncoming(%v, %v) = %v, %v; want %v", tt.user, tt.peer, got, err, tt.want)
			}
		}
	})

	t.Run("history in both directions", func(t *testing.T) {
		got, err := messages.Get(ctx, m2.ID())
		if err != nil || !reflect.DeepEqual(got, m2) {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, m2)
		}

		newest, err := messages.ListBefore(ctx, ann, bob, uuid.Nil, 2)
		if err != nil || len(newest) != 2 || newest[0].ID() != m3.ID() || newest[1].ID() != m2.ID() {
			t.Fatalf("ListBefore = %v, %v", newest, err)
		}

		// The pair in the other order is the same dialog.
		older, err := messages.ListBefore(ctx, bob, ann, m2.ID(), 10)
		if err != nil || len(older) != 1 || older[0].ID() != m1.ID() {
			t.Fatalf("ListBefore after cursor = %v, %v", older, err)
		}

		after, err := messages.ListAfter(ctx, ann, bob, m1.ID(), 10)
		if err != nil || len(after) != 2 || after[0].ID() != m2.ID() || after[1].ID() != m3.ID() {
			t.Fatalf("ListAfter = %v, %v", after, err)
		}

		if _, err = messages.Get(ctx, uuid.New()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("Get missing: %v", err)
		}
	})

	t.Run("dialogs and read marks", func(t *testing.T) {
		list, err := dialogs.List(ctx, ann, uuid.Nil, 10)
		if err != nil || len(list) != 2 {
			t.Fatalf("List = %v, %v", list, err)
		}
		if list[0].PeerID != cat || list[1].PeerID != bob || list[1].LastMessage.ID() != m3.ID() || list[1].LastIncomingID != m3.ID() {
			t.Fatalf("dialogs = %+v", list)
		}
		if !list[1].HasUnread() || list[0].HasUnread() {
			t.Fatal("bob's dialog is unread, cat's has nothing incoming")
		}

		if n, err := dialogs.CountUnread(ctx, ann, 10); err != nil || n != 1 {
			t.Fatalf("CountUnread = %d, %v", n, err)
		}

		if err = dialogs.MarkRead(ctx, ann, bob, m3.ID()); err != nil {
			t.Fatal(err)
		}
		if err = dialogs.MarkRead(ctx, ann, bob, m1.ID()); err != nil {
			t.Fatal(err)
		}

		marks, err := dialogs.ReadMarks(ctx, ann, bob)
		if err != nil || marks.Mine != m3.ID() || marks.Theirs != uuid.Nil {
			t.Fatalf("ReadMarks = %+v, %v; the newer mark must stay", marks, err)
		}

		theirs, err := dialogs.ReadMarks(ctx, bob, ann)
		if err != nil || theirs.Theirs != m3.ID() {
			t.Fatalf("bob's marks = %+v, %v", theirs, err)
		}

		if n, _ := dialogs.CountUnread(ctx, ann, 10); n != 0 {
			t.Fatalf("unread after reading = %d", n)
		}

		page, err := dialogs.List(ctx, ann, toCat.ID(), 10)
		if err != nil || len(page) != 1 || page[0].PeerID != bob {
			t.Fatalf("List after cursor = %v, %v", page, err)
		}
	})

	// Both sides' dialogs change in the statement that saves the message,
	// so concurrent messages leave them pointing at the newest.
	t.Run("concurrent messages", func(t *testing.T) {
		dan, eve := uuid.New(), uuid.New()

		var wg sync.WaitGroup
		for i := range 20 {
			from, to := dan, eve
			if i%2 == 0 {
				from, to = eve, dan
			}
			wg.Go(func() {
				m, err := message.New(from, to, message.RestoreContent("x", nil, uuid.Nil, nil), time.Now())
				if err == nil {
					err = messages.Save(ctx, m)
				}
				if err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()

		newest, err := messages.ListBefore(ctx, dan, eve, uuid.Nil, 1)
		if err != nil || len(newest) != 1 {
			t.Fatal(err)
		}

		for _, user := range []uuid.UUID{dan, eve} {
			list, err := dialogs.List(ctx, user, uuid.Nil, 10)
			if err != nil || len(list) != 1 || list[0].LastMessage.ID() != newest[0].ID() {
				t.Fatalf("%v's dialog = %+v, %v; want the newest message", user, list, err)
			}
		}
	})
}

func TestMessageRepository_DeleteByUser(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	bobsPhoto := uuid.New()
	for _, pair := range [][2]uuid.UUID{{ann, bob}, {bob, ann}, {cat, ann}, {bob, cat}} {
		var photos message.Photos
		if pair == [2]uuid.UUID{bob, ann} {
			photos = message.Photos{bobsPhoto}
		}

		m, err := message.New(pair[0], pair[1], message.RestoreContent("привет", photos, uuid.Nil, nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = messages.Save(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// The deleted messages with photos come back, for media.
	withPhotos, err := messages.DeleteByUser(ctx, ann)
	if err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if len(withPhotos) != 1 || withPhotos[0].SenderID() != bob || !slices.Equal(withPhotos[0].Photos(), message.Photos{bobsPhoto}) {
		t.Fatalf("deleted messages with photos = %v", withPhotos)
	}

	for who, want := range map[uuid.UUID]int{ann: 0, bob: 1, cat: 1} {
		if got, err := dialogs.List(ctx, who, uuid.Nil, 10); err != nil || len(got) != want {
			t.Errorf("dialogs of %s = %d, %v; want %d", who, len(got), err, want)
		}
	}
	if got, err := messages.ListBefore(ctx, bob, ann, uuid.Nil, 10); err != nil || len(got) != 0 {
		t.Errorf("messages of bob and ann = %d, %v; want none", len(got), err)
	}
	if got, err := messages.ListBefore(ctx, bob, cat, uuid.Nil, 10); err != nil || len(got) != 1 {
		t.Errorf("messages of bob and cat = %d, %v; want 1", len(got), err)
	}
}

func TestMessageRepository_SharedPost(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	ann, bob, post := uuid.New(), uuid.New(), uuid.New()

	m, err := message.New(ann, bob, message.RestoreContent("", nil, post, nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, m); err != nil {
		t.Fatalf("Save a post alone: %v", err)
	}

	got, err := messages.Get(ctx, m.ID())
	if err != nil || got.Post() != post || got.Text() != "" {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	plain, _ := message.New(ann, bob, message.RestoreContent("привет", nil, uuid.Nil, nil), time.Now())
	if err = messages.Save(ctx, plain); err != nil {
		t.Fatal(err)
	}
	if got, err = messages.Get(ctx, plain.ID()); err != nil || got.Post() != uuid.Nil || got.SharedPhoto() != uuid.Nil {
		t.Fatalf("no post = %+v, %v", got, err)
	}

	// A photo of an album alone, and carried on by another message.
	photo := uuid.New()
	share, err := message.NewPhotoShare("", photo)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := message.New(ann, bob, share, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, shared); err != nil {
		t.Fatalf("Save a shared photo alone: %v", err)
	}
	if got, err = messages.Get(ctx, shared.ID()); err != nil || got.SharedPhoto() != photo || got.Post() != uuid.Nil || got.Text() != "" {
		t.Fatalf("a shared photo = %+v, %v", got, err)
	}
	if page, err := messages.ListBefore(ctx, ann, bob, uuid.Nil, 1); err != nil || len(page) != 1 || page[0].SharedPhoto() != photo {
		t.Fatalf("the dialog shows %+v, %v", page, err)
	}

	forward, err := message.New(bob, ann, message.RestoreContent("", nil, uuid.Nil, shared.Carried()), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, forward); err != nil {
		t.Fatal(err)
	}
	if got, err = messages.Get(ctx, forward.ID()); err != nil || len(got.Forwarded()) != 1 || got.Forwarded()[0].SharedPhoto != photo {
		t.Fatalf("a forwarded shared photo = %+v, %v", got, err)
	}

	// Taken back, it shares nothing.
	gone, err := shared.TakenBack(ann, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Retract(ctx, gone); err != nil {
		t.Fatal(err)
	}
	if got, err = messages.Get(ctx, shared.ID()); err != nil || got.SharedPhoto() != uuid.Nil || !got.IsDeleted() {
		t.Fatalf("taken back = %+v, %v", got, err)
	}
}

func TestMessageRepository_Delete(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)

	save := func(from, to uuid.UUID) *message.Message {
		t.Helper()
		m, err := message.New(from, to, message.RestoreContent("x", nil, uuid.Nil, nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = messages.Save(ctx, m); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return m
	}

	last := func(user, peer uuid.UUID) message.Dialog {
		t.Helper()
		list, err := dialogs.List(ctx, user, uuid.Nil, 10)
		if err != nil || len(list) > 1 {
			t.Fatalf("List = %+v, %v", list, err)
		}
		if len(list) == 0 {
			return message.Dialog{}
		}
		return list[0]
	}

	t.Run("sides follow the messages left", func(t *testing.T) {
		ann, bob := uuid.New(), uuid.New()

		fromBob := save(bob, ann)
		older := save(ann, bob)
		newer := save(ann, bob)

		// Not the last one: the sides stay as they were.
		if err := messages.Delete(ctx, older); err != nil {
			t.Fatal(err)
		}
		if d := last(ann, bob); d.LastMessage.ID() != newer.ID() || d.LastIncomingID != fromBob.ID() {
			t.Fatalf("ann's side = %+v", d)
		}

		if err := messages.Delete(ctx, newer); err != nil {
			t.Fatal(err)
		}
		for _, user := range []uuid.UUID{ann, bob} {
			if d := last(user, uuid.Nil); d.LastMessage == nil || d.LastMessage.ID() != fromBob.ID() {
				t.Fatalf("side of %v = %+v; want bob's message last", user, d)
			}
		}
		if d := last(ann, bob); d.LastIncomingID != fromBob.ID() || !d.HasUnread() {
			t.Fatalf("ann's side = %+v; bob's message is still unread", d)
		}

		if err := messages.Delete(ctx, newer); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("delete twice: %v", err)
		}

		// Nothing left: the dialog leaves both lists and nobody wrote first.
		if err := messages.Delete(ctx, fromBob); err != nil {
			t.Fatal(err)
		}
		for _, user := range []uuid.UUID{ann, bob} {
			if d := last(user, uuid.Nil); d.LastMessage != nil {
				t.Fatalf("side of %v = %+v; want no dialog", user, d)
			}
		}
		if n, err := dialogs.CountUnread(ctx, ann, 10); err != nil || n != 0 {
			t.Fatalf("CountUnread = %d, %v", n, err)
		}
		if ok, err := dialogs.HasIncoming(ctx, ann, bob); err != nil || ok {
			t.Fatalf("HasIncoming = %v, %v", ok, err)
		}

		// A new message brings the dialog back.
		again := save(bob, ann)
		if d := last(ann, bob); d.LastMessage == nil || d.LastMessage.ID() != again.ID() || d.LastIncomingID != again.ID() {
			t.Fatalf("ann's side = %+v", d)
		}
	})

	// Deleting and sending lock both sides in the same order, so they never
	// deadlock, and the sides end up at the newest message left.
	t.Run("deleting while both write", func(t *testing.T) {
		dan, eve := uuid.New(), uuid.New()

		doomed := make([]*message.Message, 10)
		for i := range doomed {
			doomed[i] = save(dan, eve)
		}

		var wg sync.WaitGroup
		for i := range 20 {
			from, to := dan, eve
			if i%2 == 0 {
				from, to = eve, dan
			}
			wg.Go(func() {
				m, err := message.New(from, to, message.RestoreContent("x", nil, uuid.Nil, nil), time.Now())
				if err == nil {
					err = messages.Save(ctx, m)
				}
				if err != nil {
					t.Error(err)
				}
			})
		}
		for _, m := range doomed {
			wg.Go(func() {
				if err := messages.Delete(ctx, m); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()

		newest, err := messages.ListBefore(ctx, dan, eve, uuid.Nil, 1)
		if err != nil || len(newest) != 1 {
			t.Fatal(err)
		}
		for _, user := range []uuid.UUID{dan, eve} {
			if d := last(user, uuid.Nil); d.LastMessage == nil || d.LastMessage.ID() != newest[0].ID() {
				t.Fatalf("%v's dialog = %+v; want the newest message", user, d)
			}
		}
	})
}

func TestMessageRepository_EditHideSearch(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	save := func(from, to uuid.UUID, text string) *message.Message {
		t.Helper()
		m, err := message.New(from, to, message.RestoreContent(message.Text(text), nil, uuid.Nil, nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = messages.Save(ctx, m); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return m
	}
	says := func(found []*message.Message) []string {
		out := make([]string, len(found))
		for i, m := range found {
			out[i] = m.Text().String()
		}
		return out
	}
	side := func(user uuid.UUID) message.Dialog {
		t.Helper()
		list, err := dialogs.List(ctx, user, uuid.Nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range list {
			if d.PeerID == ann || d.PeerID == bob {
				return d
			}
		}
		return message.Dialog{}
	}

	first := save(ann, bob, "Еду на море")
	second := save(ann, bob, "скидка 50%")
	fromBob := save(bob, ann, "МОРЕ волнуется")
	save(cat, ann, "на море хорошо")

	// Edit: the text and when, read back.
	edited, err := first.Edited(ann, message.Revision{Text: "Еду на море!"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Update(ctx, edited); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, err := messages.Get(ctx, first.ID()); err != nil || got.Text() != "Еду на море!" || !got.EditedAt().Equal(edited.EditedAt()) {
		t.Fatalf("the edited message = %+v, %v", got, err)
	}
	if got, _ := messages.Get(ctx, second.ID()); !got.EditedAt().IsZero() {
		t.Fatal("a message never edited says it was")
	}
	// An edit that changes the photos writes them.
	photo := uuid.New()
	withPhoto, err := second.Edited(ann, message.Revision{Text: second.Text(), Added: message.Photos{photo}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Update(ctx, withPhoto); err != nil {
		t.Fatalf("Update with a photo: %v", err)
	}
	if got, err := messages.Get(ctx, second.ID()); err != nil || !slices.Equal(got.Photos(), message.Photos{photo}) || got.Text() != second.Text() {
		t.Fatalf("the message with a photo = %+v, %v", got, err)
	}
	if got, _ := messages.Get(ctx, first.ID()); len(got.Photos()) != 0 {
		t.Fatal("an edit of the words gave the message photos")
	}
	missing, _ := message.New(ann, bob, message.RestoreContent("x", nil, uuid.Nil, nil), time.Now())
	if err = messages.Update(ctx, missing); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("Update of no message: %v", err)
	}

	// Search: any case, the wildcards as themselves, one dialog or all.
	sea, _ := message.NewQuery("море")
	if found, err := messages.Search(ctx, ann, uuid.Nil, sea, uuid.Nil, 10); err != nil || !slices.Equal(says(found), []string{"на море хорошо", "МОРЕ волнуется", "Еду на море!"}) {
		t.Fatalf("море = %v, %v", says(found), err)
	}
	if found, _ := messages.Search(ctx, ann, bob, sea, uuid.Nil, 10); !slices.Equal(says(found), []string{"МОРЕ волнуется", "Еду на море!"}) {
		t.Fatalf("море with bob = %v", says(found))
	}
	if found, _ := messages.Search(ctx, ann, bob, sea, fromBob.ID(), 10); !slices.Equal(says(found), []string{"Еду на море!"}) {
		t.Fatalf("море before the cursor = %v", says(found))
	}
	// What a message carries on is searched as its own words are.
	dan := uuid.New()
	carrying, err := message.New(dan, cat, message.RestoreContent("смотри", nil, uuid.Nil, message.Forwards{
		{SenderID: ann, Text: "горы", CreatedAt: time.Now()},
		{SenderID: ann, Text: "Еду на МОРЕ", CreatedAt: time.Now()},
	}), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, carrying); err != nil {
		t.Fatal(err)
	}
	if found, err := messages.Search(ctx, dan, uuid.Nil, sea, uuid.Nil, 10); err != nil || !slices.Equal(says(found), []string{"смотри"}) {
		t.Fatalf("море among the forwarded = %v, %v", says(found), err)
	}
	peaks, _ := message.NewQuery("гор_")
	if found, _ := messages.Search(ctx, dan, cat, peaks, uuid.Nil, 10); len(found) != 0 {
		t.Fatalf("a wildcard among the forwarded = %v", says(found))
	}
	percent, _ := message.NewQuery("%")
	if found, _ := messages.Search(ctx, ann, uuid.Nil, percent, uuid.Nil, 10); !slices.Equal(says(found), []string{"скидка 50%"}) {
		t.Fatalf("%% = %v", says(found))
	}
	if found, _ := messages.Search(ctx, cat, bob, sea, uuid.Nil, 10); len(found) != 0 {
		t.Fatalf("cat finds what ann and bob wrote: %v", says(found))
	}

	// Hide: ann deletes bob's message for herself, her side falls back.
	if err = messages.Hide(ctx, fromBob, ann); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	if got, _ := messages.Get(ctx, fromBob.ID()); got.VisibleTo(ann) || !got.VisibleTo(bob) {
		t.Fatalf("after Hide = %+v", got)
	}
	if d := side(ann); d.PeerID != bob || d.LastMessage.ID() != second.ID() || d.LastIncomingID != uuid.Nil || d.HasUnread() {
		t.Fatalf("ann's side = %+v", d)
	}
	if d := side(bob); d.LastMessage.ID() != fromBob.ID() {
		t.Fatalf("bob's side = %+v", d)
	}
	if anns, _ := messages.ListBefore(ctx, ann, bob, uuid.Nil, 10); len(anns) != 2 {
		t.Fatalf("ann reads %v", says(anns))
	}
	if anns, _ := messages.ListAfter(ctx, ann, bob, first.ID(), 10); !slices.Equal(says(anns), []string{"скидка 50%"}) {
		t.Fatalf("ann reads after the first %v", says(anns))
	}
	if bobs, _ := messages.ListBefore(ctx, bob, ann, uuid.Nil, 10); len(bobs) != 3 {
		t.Fatalf("bob reads %v", says(bobs))
	}
	if found, _ := messages.Search(ctx, ann, bob, sea, uuid.Nil, 10); !slices.Equal(says(found), []string{"Еду на море!"}) {
		t.Fatalf("ann finds a hidden message: %v", says(found))
	}
	if err = messages.Hide(ctx, fromBob, cat); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a stranger hides: %v", err)
	}

	// Deleted for both, a message leaves each side to what that side has:
	// ann's does not come back to the message she hid.
	if err = messages.Delete(ctx, second); err != nil {
		t.Fatal(err)
	}
	if d := side(ann); d.LastMessage.ID() != first.ID() {
		t.Fatalf("ann's side after the delete = %+v", d.LastMessage)
	}
	if d := side(bob); d.LastMessage.ID() != fromBob.ID() {
		t.Fatalf("bob's side after the delete = %+v", d.LastMessage)
	}
}

func TestMessageRepository_ForwardAndRetract(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	carried := message.Forwards{
		{SenderID: cat, Text: "смотри", Photos: message.Photos{uuid.New()}, Post: uuid.New(), CreatedAt: now.Add(-time.Hour)},
		{SenderID: ann, Text: "ого", CreatedAt: now.Add(-time.Minute), Depth: 1},
	}

	// Forwards alone are a message.
	m, err := message.New(ann, bob, message.RestoreContent("", nil, uuid.Nil, carried), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Save(ctx, m); err != nil {
		t.Fatalf("Save forwards alone: %v", err)
	}

	got, err := messages.Get(ctx, m.ID())
	if err != nil || len(got.Forwarded()) != 2 {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	for i, f := range got.Forwarded() {
		want := carried[i]
		if f.SenderID != want.SenderID || f.Text != want.Text || !slices.Equal(f.Photos, want.Photos) || f.Post != want.Post || !f.CreatedAt.Equal(want.CreatedAt) ||
			f.Depth != want.Depth {
			t.Fatalf("forwarded[%d] = %+v, want %+v", i, f, want)
		}
	}

	// The list of dialogs reads them too.
	list, err := dialogs.List(ctx, bob, uuid.Nil, 10)
	if err != nil || len(list) != 1 || len(list[0].LastMessage.Forwarded()) != 2 {
		t.Fatalf("List = %+v, %v", list, err)
	}

	// Taken back, the message stays where it was and says nothing.
	gone, err := m.TakenBack(ann, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = messages.Retract(ctx, gone); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if err = messages.Retract(ctx, gone); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("Retract twice: %v", err)
	}

	got, err = messages.Get(ctx, m.ID())
	if err != nil || !got.IsDeleted() || !got.DeletedAt().Equal(gone.DeletedAt()) || got.Text() != "" || len(got.Forwarded()) != 0 {
		t.Fatalf("taken back = %+v, %v", got, err)
	}
	if list, err = dialogs.List(ctx, bob, uuid.Nil, 10); err != nil || len(list) != 1 || !list[0].LastMessage.IsDeleted() || !list[0].HasUnread() {
		t.Fatalf("List = %+v, %v; want the message taken back last and unread", list, err)
	}
	if ok, err := dialogs.HasIncoming(ctx, bob, ann); err != nil || !ok {
		t.Fatalf("HasIncoming = %v, %v", ok, err)
	}

	// The erased page's dialogs return what carried photos on.
	again, _ := message.New(bob, ann, message.RestoreContent("", nil, uuid.Nil, carried[:1]), now)
	if err = messages.Save(ctx, again); err != nil {
		t.Fatal(err)
	}
	withPhotos, err := messages.DeleteByUser(ctx, ann)
	if err != nil || len(withPhotos) != 1 || withPhotos[0].ID() != again.ID() || len(withPhotos[0].Forwarded()[0].Photos) != 1 {
		t.Fatalf("DeleteByUser = %+v, %v", withPhotos, err)
	}
}
