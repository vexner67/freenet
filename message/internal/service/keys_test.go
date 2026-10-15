package service_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/inmem"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/pubsub"
)

// everyoneKey is the key every user has in the tests that are not about
// keys: a message sealed by anyone for anyone is sealed right.
var everyoneKey = keyOf("everyone")

func keyOf(name string) message.Key {
	sum := sha256.Sum256([]byte(name))
	return message.Key(sum[:])
}

type sameKeys struct{}

func (sameKeys) Get(context.Context, uuid.UUID) (message.Key, error) { return everyoneKey, nil }
func (sameKeys) Set(context.Context, uuid.UUID, message.Key) error   { return nil }
func (sameKeys) Delete(context.Context, uuid.UUID) error             { return nil }

// sealedBy is text as a browser with the key from would seal it for the
// key to: the server never opens it, so the bytes only have to look right.
func sealedBy(from, to message.Key, text string) message.Sealed {
	b := []byte{1}
	b = append(b, from.Bytes()...)
	b = append(b, to.Bytes()...)
	b = append(b, make([]byte, 12)...)
	b = append(b, text...)
	b = append(b, make([]byte, 16)...)

	return message.Sealed(b)
}

func seal(text string) message.Sealed {
	return sealedBy(everyoneKey, everyoneKey, text)
}

// opened is what a sealed message says, as the browser opens it.
func opened(m *message.Message) string {
	s := m.Sealed()
	if s == "" {
		return ""
	}

	return string(s[1+2*message.KeySize+12 : len(s)-16])
}

func TestMessageService_Keys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	photos := &fakePhotos{}
	svc := service.NewMessageService(service.Deps{
		Messages: store.Messages(), Reactions: store.Reactions(), Dialogs: store.Dialogs(), Keys: store.Keys(), ChatMessages: store.ChatMessages(), ChatMembers: store.ChatMembers(),
		Events: pubsub.New[uuid.UUID, service.Event](64), Tx: fakeTx{}, Photos: photos,
	})
	ann, bob := uuid.New(), uuid.New()
	annKey, bobKey := keyOf("ann"), keyOf("bob")

	if _, err := svc.Key(ctx, ann); !errors.Is(err, message.ErrNoKey) {
		t.Fatalf("no key yet: %v", err)
	}

	// Nobody writes to a person who has no key yet.
	sealed := sealedBy(annKey, bobKey, "привет")
	if err := svc.SetKey(ctx, ann, annKey); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: sealed}); !errors.Is(err, message.ErrKeyChanged) {
		t.Fatalf("to a person without a key: %v", err)
	}

	if err := svc.SetKey(ctx, bob, bobKey); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Key(ctx, bob); err != nil || got != bobKey {
		t.Fatalf("Key = %x, %v", got, err)
	}

	m, err := svc.Send(ctx, ann, bob, service.Draft{Sealed: sealed})
	if err != nil || m.Sealed() != sealed {
		t.Fatalf("Send = %+v, %v", m, err)
	}

	// Plain words do not go into a dialog, nor do words sealed with keys
	// the two do not have any more.
	if _, err = svc.Send(ctx, ann, bob, service.Draft{Text: "открыто"}); !errors.Is(err, message.ErrPlainText) {
		t.Fatalf("plain words: %v", err)
	}
	for name, s := range map[string]message.Sealed{
		"swapped":        sealedBy(bobKey, annKey, "x"),
		"an old key":     sealedBy(keyOf("ann before"), bobKey, "x"),
		"someone else's": sealedBy(annKey, keyOf("cat"), "x"),
	} {
		if _, err = svc.Send(ctx, ann, bob, service.Draft{Sealed: s}); !errors.Is(err, message.ErrKeyChanged) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// A message of photos alone says nothing to seal.
	if _, err = svc.Send(ctx, ann, bob, service.Draft{Photos: message.Photos{uuid.New()}}); err != nil {
		t.Fatalf("photos alone: %v", err)
	}

	// A new key of bob leaves the old messages as they are, and the new
	// ones must be sealed with it, an edit too.
	newBob := keyOf("bob anew")
	if err = svc.SetKey(ctx, bob, newBob); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: sealedBy(annKey, bobKey, "пока")}); !errors.Is(err, message.ErrKeyChanged) {
		t.Fatalf("an edit with the old key: %v", err)
	}
	edited, err := svc.Edit(ctx, ann, m.ID(), message.Revision{Sealed: sealedBy(annKey, newBob, "пока"), BaseSealed: &sealed})
	if err != nil || edited.Sealed().RecipientKey() != newBob {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}
	if _, err = svc.Edit(ctx, ann, m.ID(), message.Revision{Text: "пока"}); !errors.Is(err, message.ErrPlainText) {
		t.Fatalf("an edit in plain words: %v", err)
	}

	// Erased, a page takes its key along.
	if err = svc.PurgeUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Key(ctx, bob); !errors.Is(err, message.ErrNoKey) {
		t.Fatalf("the key of an erased page: %v", err)
	}
	if got, err := svc.Key(ctx, ann); err != nil || got != annKey {
		t.Fatalf("another's key = %x, %v", got, err)
	}
}

// A message of a dialog keeps its words sealed: it is not carried on into
// a chat, where everyone would read them.
func TestChatService_ForwardFromDialog(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	chatID := chatOf(t, f.svc, ann, bob, cat)

	private, err := f.dialogs.Send(ctx, bob, ann, service.Draft{Sealed: seal("только тебе")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.Send(ctx, ann, chatID, service.Draft{Forward: []uuid.UUID{private.ID()}}); !errors.Is(err, message.ErrForwardIntoChat) {
		t.Fatalf("a dialog's message into a chat: %v", err)
	}
	if _, err = f.svc.Send(ctx, ann, chatID, service.Draft{Sealed: seal("всем")}); !errors.Is(err, message.ErrSealedInChat) {
		t.Fatalf("sealed words in a chat: %v", err)
	}
}
