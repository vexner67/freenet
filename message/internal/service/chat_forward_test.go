package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
)

// A message is carried on from a dialog into a chat, from a chat into a
// dialog and from a chat into another: only by who has it.
func TestChatService_Forward(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)
	other := chatOf(t, f.svc, ann, cat, dan)

	photo := uuid.New()

	private, err := f.dialogs.Send(ctx, bob, ann, service.Draft{Text: "только тебе", Photos: message.Photos{photo}})
	if err != nil {
		t.Fatal(err)
	}

	said, err := f.svc.Send(ctx, cat, chatID, service.Draft{Text: "всем привет"})
	if err != nil {
		t.Fatal(err)
	}

	carrying, err := f.svc.Send(ctx, ann, chatID, service.Draft{Forward: []uuid.UUID{said.ID(), private.ID(), private.ID()}})
	if err != nil {
		t.Fatalf("a dialog message and a chat message into a chat: %v", err)
	}

	forwarded := carrying.Forwarded()
	if len(forwarded) != 2 || forwarded[0].SenderID != bob || forwarded[0].Text != "только тебе" || forwarded[1].SenderID != cat || forwarded[1].Text != "всем привет" {
		t.Fatalf("carried on = %+v", forwarded)
	}
	// The photo is kept once more, under a new ID, with the message.
	if len(forwarded[0].Photos) != 1 || forwarded[0].Photos[0] == photo {
		t.Fatalf("the photo carried on = %v, want a copy of %v", forwarded[0].Photos, photo)
	}
	if len(f.photos.copied) != 1 || f.photos.copied[0].owner != bob || f.photos.copied[0].copies[0].From != photo ||
		f.photos.copied[0].copies[0].To != forwarded[0].Photos[0] || f.photos.outsideTx != 0 {
		t.Fatalf("media copied %+v, outside the transaction %d", f.photos.copied, f.photos.outsideTx)
	}

	t.Run("deeper", func(t *testing.T) {
		// What carried messages on carries them a level deeper, into a dialog too.
		again, err := f.dialogs.Send(ctx, bob, dan, service.Draft{Text: "гляди", Forward: []uuid.UUID{carrying.ID()}})
		if err != nil {
			t.Fatalf("a chat message into a dialog: %v", err)
		}

		got := again.Forwarded()
		if len(got) != 3 || got[0].SenderID != ann || got[0].Depth != 0 || got[1].Depth != 1 || got[2].Depth != 1 || got[1].Photos[0] == forwarded[0].Photos[0] {
			t.Fatalf("carried on again = %+v", got)
		}

		into, err := f.svc.Send(ctx, cat, other, service.Draft{Forward: []uuid.UUID{said.ID()}})
		if err != nil || len(into.Forwarded()) != 1 {
			t.Fatalf("a chat message into another chat = %+v, %v", into, err)
		}
	})

	t.Run("only what one has", func(t *testing.T) {
		created, err := f.svc.History(ctx, ann, chatID, uuid.Nil, 20)
		if err != nil || !created.Messages[0].IsAction() {
			t.Fatalf("History = %+v, %v", created, err)
		}

		for name, id := range map[string]uuid.UUID{
			"a message of a chat one is not in":  said.ID(),
			"a message of someone else's dialog": private.ID(),
			"a message made up":                  uuid.Must(uuid.NewV7()),
		} {
			if _, err := f.svc.Send(ctx, dan, other, service.Draft{Forward: []uuid.UUID{id}}); !errors.Is(err, message.ErrForwardNotFound) {
				t.Errorf("%s: %v", name, err)
			}
		}

		if _, err := f.svc.Send(ctx, ann, chatID, service.Draft{Forward: []uuid.UUID{created.Messages[0].ID()}}); !errors.Is(err, message.ErrForwardNotFound) {
			t.Errorf("what happened in the chat is carried on: %v", err)
		}

		// Who was removed keeps the chat as it was, and no more.
		if err := f.svc.Kick(ctx, ann, chatID, cat); err != nil {
			t.Fatal(err)
		}

		after, err := f.svc.Send(ctx, ann, chatID, service.Draft{Text: "без кати"})
		if err != nil {
			t.Fatal(err)
		}

		if _, err = f.dialogs.Send(ctx, cat, dan, service.Draft{Forward: []uuid.UUID{after.ID()}}); !errors.Is(err, message.ErrForwardNotFound) {
			t.Errorf("a message written after one was removed: %v", err)
		}
		if _, err = f.dialogs.Send(ctx, cat, dan, service.Draft{Forward: []uuid.UUID{said.ID()}}); err != nil {
			t.Errorf("a message one saw before being removed: %v", err)
		}
	})

	t.Run("taken back", func(t *testing.T) {
		deleted := len(f.photos.deleted)

		if err := f.svc.Delete(ctx, ann, carrying.ID()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		// The copy it carried on goes with it, under who sent the photo.
		if got := f.photos.deleted[deleted:]; len(got) != 1 || got[0].owner != bob || !slices.Equal(got[0].photos, []uuid.UUID(forwarded[0].Photos)) {
			t.Fatalf("media deleted %+v", got)
		}

		if _, err := f.svc.Send(ctx, bob, chatID, service.Draft{Forward: []uuid.UUID{carrying.ID()}}); !errors.Is(err, message.ErrForwardNotFound) {
			t.Errorf("a message taken back is carried on: %v", err)
		}
	})
}

func TestChatService_Share(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)
	post, albumPhoto := uuid.New(), uuid.New()

	shared, err := f.svc.Send(ctx, ann, chatID, service.Draft{Post: post})
	if err != nil || shared.Post() != post || shared.Text() != "" {
		t.Fatalf("a post alone = %+v, %v", shared, err)
	}

	photo, err := f.svc.Send(ctx, ann, chatID, service.Draft{Text: "гляньте", SharedPhoto: albumPhoto})
	if err != nil || photo.SharedPhoto() != albumPhoto {
		t.Fatalf("a photo of an album = %+v, %v", photo, err)
	}

	if _, err = f.svc.Send(ctx, ann, chatID, service.Draft{SharedPhoto: albumPhoto, Post: post}); !errors.Is(err, message.ErrSharedPhotoAlone) {
		t.Fatalf("a photo of an album with a post: %v", err)
	}

	// The words go, the post stays.
	edited, err := f.svc.Edit(ctx, ann, shared.ID(), message.Revision{Text: "читайте"})
	if err != nil || edited.Post() != post || edited.Text() != "читайте" {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}
	if edited, err = f.svc.Edit(ctx, ann, shared.ID(), message.Revision{}); err != nil || edited.Post() != post {
		t.Fatalf("Edit to the post alone = %+v, %v", edited, err)
	}

	details, err := f.svc.Get(ctx, bob, chatID)
	if err != nil || details.View.LastMessage == nil || details.View.LastMessage.SharedPhoto() != albumPhoto {
		t.Fatalf("the last message = %+v, %v", details.View.LastMessage, err)
	}

	if err = f.svc.Delete(ctx, ann, shared.ID()); err != nil {
		t.Fatal(err)
	}

	gone, _, err := f.svc.Message(ctx, bob, shared.ID())
	if err != nil || !gone.IsDeleted() || gone.Post() != uuid.Nil {
		t.Fatalf("taken back = %+v, %v", gone, err)
	}
}

func TestChatService_EditPhotos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)
	kept, dropped, added := uuid.New(), uuid.New(), uuid.New()

	m, err := f.svc.Send(ctx, ann, chatID, service.Draft{Text: "фото", Photos: message.Photos{kept, dropped}})
	if err != nil {
		t.Fatal(err)
	}

	published, deleted := len(f.photos.published), len(f.photos.deleted)

	edited, err := f.svc.Edit(ctx, ann, m.ID(), message.Revision{Text: "фото", Removed: message.Photos{dropped}, Added: message.Photos{added}})
	if err != nil || !slices.Equal(edited.Photos(), message.Photos{kept, added}) {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}

	if got := f.photos.published[published:]; len(got) != 1 || got[0].owner != ann || !slices.Equal(got[0].photos, []uuid.UUID{added}) {
		t.Fatalf("media keeps %+v", got)
	}
	if got := f.photos.deleted[deleted:]; len(got) != 1 || !slices.Equal(got[0].photos, []uuid.UUID{dropped}) {
		t.Fatalf("media deletes %+v", got)
	}
	if f.photos.outsideTx != 0 {
		t.Fatalf("media heard %d times outside the transaction", f.photos.outsideTx)
	}

	// An edit that names nothing keeps the photos: only the words change.
	if edited, err = f.svc.Edit(ctx, ann, m.ID(), message.Revision{}); err != nil || len(edited.Photos()) != 2 || edited.Text() != "" {
		t.Fatalf("Edit to photos alone = %+v, %v", edited, err)
	}

	if _, err = f.svc.Edit(ctx, ann, m.ID(), message.Revision{Removed: message.Photos{kept, added}}); !errors.Is(err, message.ErrTextEmpty) {
		t.Fatalf("an edit to nothing: %v", err)
	}

	// Who left the chat edits nothing there.
	theirs, err := f.svc.Send(ctx, cat, chatID, service.Draft{Text: "пока"})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.Leave(ctx, cat, chatID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Edit(ctx, cat, theirs.ID(), message.Revision{Text: "вернусь", Added: message.Photos{uuid.New()}}); !errors.Is(err, message.ErrNotInChat) {
		t.Fatalf("an edit by who left: %v", err)
	}
}

func TestChatService_Search(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)
	other := chatOf(t, f.svc, ann, bob, dan)

	say := func(sender, chat uuid.UUID, d service.Draft) *message.ChatMessage {
		t.Helper()

		m, err := f.svc.Send(ctx, sender, chat, d)
		if err != nil {
			t.Fatalf("Send: %v", err)
		}

		return m
	}

	first := say(bob, chatID, service.Draft{Text: "идём в Поход"})
	say(cat, chatID, service.Draft{Text: "а я дома"})
	elsewhere := say(dan, other, service.Draft{Text: "какой поход?"})
	carrying := say(ann, other, service.Draft{Text: "вот", Forward: []uuid.UUID{first.ID()}})

	q, err := message.NewQuery("поход")
	if err != nil {
		t.Fatal(err)
	}

	ids := func(page service.ChatSearchPage) []uuid.UUID {
		out := make([]uuid.UUID, len(page.Messages))
		for i, m := range page.Messages {
			out[i] = m.ID()
		}

		return out
	}

	// The title of the chat ("Поход", in what happened) is not a message.
	all, err := f.svc.Search(ctx, ann, uuid.Nil, q, uuid.Nil, 20)
	if err != nil || !slices.Equal(ids(all), []uuid.UUID{carrying.ID(), elsewhere.ID(), first.ID()}) || all.NextCursor != uuid.Nil {
		t.Fatalf("all the chats = %v, %v", ids(all), err)
	}

	one, err := f.svc.Search(ctx, ann, chatID, q, uuid.Nil, 20)
	if err != nil || !slices.Equal(ids(one), []uuid.UUID{first.ID()}) {
		t.Fatalf("one chat = %v, %v", ids(one), err)
	}

	page, err := f.svc.Search(ctx, ann, uuid.Nil, q, uuid.Nil, 2)
	if err != nil || len(page.Messages) != 2 || page.NextCursor != elsewhere.ID() {
		t.Fatalf("the first page = %v, %v, %v", ids(page), page.NextCursor, err)
	}
	if page, err = f.svc.Search(ctx, ann, uuid.Nil, q, page.NextCursor, 2); err != nil || !slices.Equal(ids(page), []uuid.UUID{first.ID()}) || page.NextCursor != uuid.Nil {
		t.Fatalf("the second page = %v, %v", ids(page), err)
	}

	if cats, err := f.svc.Search(ctx, cat, uuid.Nil, q, uuid.Nil, 20); err != nil || !slices.Equal(ids(cats), []uuid.UUID{first.ID()}) {
		t.Fatalf("who is in one chat finds %v, %v", ids(cats), err)
	}
	if _, err = f.svc.Search(ctx, cat, other, q, uuid.Nil, 20); !errors.Is(err, message.ErrChatNotFound) {
		t.Fatalf("a search of a chat one is not in: %v", err)
	}

	// Who left finds what they were shown, and no more.
	if err = f.svc.Leave(ctx, cat, chatID); err != nil {
		t.Fatal(err)
	}
	say(bob, chatID, service.Draft{Text: "поход отменён"})

	if cats, err := f.svc.Search(ctx, cat, uuid.Nil, q, uuid.Nil, 20); err != nil || !slices.Equal(ids(cats), []uuid.UUID{first.ID()}) {
		t.Fatalf("who left finds %v, %v", ids(cats), err)
	}

	// A message taken back says nothing to find.
	if err = f.svc.Delete(ctx, bob, first.ID()); err != nil {
		t.Fatal(err)
	}
	if one, err = f.svc.Search(ctx, cat, chatID, q, uuid.Nil, 20); err != nil || len(one.Messages) != 0 {
		t.Fatalf("after taking back = %v, %v", ids(one), err)
	}
}

func TestChatService_Around(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)
	other := chatOf(t, f.svc, ann, bob, cat)

	sent := make([]*message.ChatMessage, 7)
	for i := range sent {
		m, err := f.svc.Send(ctx, bob, chatID, service.Draft{Text: "слово"})
		if err != nil {
			t.Fatal(err)
		}

		sent[i] = m
	}

	page, err := f.svc.Around(ctx, ann, chatID, sent[3].ID(), 4)
	if err != nil || len(page.Messages) != 5 || page.Messages[0].ID() != sent[1].ID() || page.Messages[4].ID() != sent[5].ID() || page.NextCursor != sent[1].ID() {
		t.Fatalf("Around = %+v, %v", page, err)
	}

	// The beginning of the chat: nothing older remains.
	if page, err = f.svc.Around(ctx, ann, chatID, sent[0].ID(), 20); err != nil || page.NextCursor != uuid.Nil || len(page.Messages) != 8 {
		t.Fatalf("Around the first = %d messages, cursor %v, %v", len(page.Messages), page.NextCursor, err)
	}

	if _, err = f.svc.Around(ctx, ann, other, sent[3].ID(), 4); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a message of another chat: %v", err)
	}
	if _, err = f.svc.Around(ctx, uuid.New(), chatID, sent[3].ID(), 4); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("a stranger: %v", err)
	}
}

// The copies of others' photos a purged user's messages carried on go
// with the messages; their own media erases with the user.
func TestChatService_PurgeUser_ForwardedPhotos(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)

	theirs, err := f.svc.Send(ctx, bob, chatID, service.Draft{Photos: message.Photos{uuid.New()}})
	if err != nil {
		t.Fatal(err)
	}
	own, err := f.svc.Send(ctx, ann, chatID, service.Draft{Photos: message.Photos{uuid.New()}})
	if err != nil {
		t.Fatal(err)
	}

	carrying, err := f.svc.Send(ctx, ann, chatID, service.Draft{Forward: []uuid.UUID{theirs.ID(), own.ID()}})
	if err != nil {
		t.Fatal(err)
	}

	deleted := len(f.photos.deleted)

	if err = f.svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}

	got := f.photos.deleted[deleted:]
	if len(got) != 1 || got[0].owner != bob || !slices.Equal(got[0].photos, []uuid.UUID(carrying.Forwarded()[0].Photos)) || f.photos.outsideTx != 0 {
		t.Fatalf("media deleted %+v, outside the transaction %d", got, f.photos.outsideTx)
	}
}
