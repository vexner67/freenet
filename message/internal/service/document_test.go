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

func TestSend_Documents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	docs := message.Documents{uuid.New(), uuid.New()}

	// Documents alone are a message, and go with words too.
	alone, err := f.dialogs.Send(ctx, ann, bob, service.Draft{Documents: docs})
	if err != nil || !slices.Equal(alone.Documents(), docs) {
		t.Fatalf("Send documents = %+v, %v", alone, err)
	}
	worded, err := f.dialogs.Send(ctx, ann, bob, service.Draft{Sealed: seal("отчёт"), Documents: docs})
	if err != nil || !slices.Equal(worded.Documents(), docs) || worded.Sealed() == "" {
		t.Fatalf("Send documents with words = %+v, %v", worded, err)
	}

	if _, err = f.dialogs.Send(ctx, ann, bob, service.Draft{Sticker: "cat_hi", Documents: docs}); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("a sticker with documents: %v", err)
	}

	// The edit keeps them, even taking the words away.
	edited, err := f.dialogs.Edit(ctx, ann, worded.ID(), message.Revision{})
	if err != nil || !slices.Equal(edited.Documents(), docs) || edited.Sealed() != "" {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}

	// Forwarded, they go with the message, in a chat as well.
	chatID := chatOf(t, f.svc, ann, bob, cat)
	inChat, err := f.svc.Send(ctx, bob, chatID, service.Draft{Text: "вот", Documents: docs[:1]})
	if err != nil || !slices.Equal(inChat.Documents(), docs[:1]) {
		t.Fatalf("Send documents into a chat = %+v, %v", inChat, err)
	}

	carried, err := f.dialogs.Send(ctx, cat, bob, service.Draft{Forward: []uuid.UUID{inChat.ID()}})
	if err != nil || len(carried.Forwarded()) != 1 || !slices.Equal(carried.Forwarded()[0].Documents, docs[:1]) {
		t.Fatalf("documents of a chat forwarded = %+v, %v", carried, err)
	}
}
