package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
)

func TestSend_Sticker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	m, err := f.dialogs.Send(ctx, ann, bob, service.Draft{Sticker: "cat_hi"})
	if err != nil || m.Sticker() != "cat_hi" {
		t.Fatalf("Send a sticker = %+v, %v", m, err)
	}

	// Nothing goes with a sticker.
	for name, d := range map[string]service.Draft{
		"words":     {Sticker: "cat_hi", Sealed: seal("привет")},
		"photos":    {Sticker: "cat_hi", Photos: message.Photos{uuid.New()}},
		"a post":    {Sticker: "cat_hi", Post: uuid.New()},
		"a photo":   {Sticker: "cat_hi", SharedPhoto: uuid.New()},
		"forwarded": {Sticker: "cat_hi", Forward: []uuid.UUID{m.ID()}},
	} {
		if _, err = f.dialogs.Send(ctx, ann, bob, d); !errors.Is(err, message.ErrStickerAlone) {
			t.Errorf("a sticker with %s: %v", name, err)
		}
	}

	// It is not edited.
	if _, err = f.dialogs.Edit(ctx, ann, m.ID(), message.Revision{Sealed: seal("привет")}); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("Edit a sticker: %v", err)
	}

	// Forwarded, it is the sticker still, in a chat as well.
	chatID := chatOf(t, f.svc, ann, bob, cat)
	forward, err := f.svc.Send(ctx, ann, chatID, service.Draft{Forward: []uuid.UUID{m.ID()}})
	if err == nil {
		t.Fatalf("a dialog forwarded into a chat = %+v", forward)
	}

	inChat, err := f.svc.Send(ctx, bob, chatID, service.Draft{Sticker: "smile_wink"})
	if err != nil || inChat.Sticker() != "smile_wink" {
		t.Fatalf("Send a sticker into a chat = %+v, %v", inChat, err)
	}
	if _, err = f.svc.Send(ctx, bob, chatID, service.Draft{Sticker: "smile_wink", Text: "привет"}); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("a sticker with words in a chat: %v", err)
	}

	carried, err := f.dialogs.Send(ctx, cat, bob, service.Draft{Forward: []uuid.UUID{inChat.ID()}})
	if err != nil || len(carried.Forwarded()) != 1 || carried.Forwarded()[0].Sticker != "smile_wink" {
		t.Fatalf("a sticker of a chat forwarded = %+v, %v", carried, err)
	}
}
