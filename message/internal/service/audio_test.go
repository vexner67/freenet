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

func TestSend_Audios(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newChats()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	songs := message.Audios{uuid.New(), uuid.New()}

	// Songs alone are a message, and go with words and documents too.
	alone, err := f.dialogs.Send(ctx, ann, bob, service.Draft{Audios: songs})
	if err != nil || !slices.Equal(alone.Audios(), songs) {
		t.Fatalf("Send songs = %+v, %v", alone, err)
	}
	docs := message.Documents{uuid.New()}
	both, err := f.dialogs.Send(ctx, ann, bob, service.Draft{Documents: docs, Audios: songs})
	if err != nil || !slices.Equal(both.Audios(), songs) || !slices.Equal(both.Documents(), docs) {
		t.Fatalf("Send songs with documents = %+v, %v", both, err)
	}
	worded, err := f.dialogs.Send(ctx, ann, bob, service.Draft{Sealed: seal("послушай"), Audios: songs})
	if err != nil || !slices.Equal(worded.Audios(), songs) {
		t.Fatalf("Send songs with words = %+v, %v", worded, err)
	}

	if _, err = f.dialogs.Send(ctx, ann, bob, service.Draft{Sticker: "cat_hi", Audios: songs}); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("a sticker with songs: %v", err)
	}

	// The edit keeps them, even taking the words away.
	edited, err := f.dialogs.Edit(ctx, ann, worded.ID(), message.Revision{})
	if err != nil || !slices.Equal(edited.Audios(), songs) {
		t.Fatalf("Edit = %+v, %v", edited, err)
	}

	// An answer counts them in its quote; forwarded, they go with the message.
	chatID := chatOf(t, f.svc, ann, bob, cat)
	inChat, err := f.svc.Send(ctx, bob, chatID, service.Draft{Audios: songs[:1]})
	if err != nil || !slices.Equal(inChat.Audios(), songs[:1]) {
		t.Fatalf("Send songs into a chat = %+v, %v", inChat, err)
	}
	answer, err := f.svc.Send(ctx, cat, chatID, service.Draft{Text: "класс", ReplyTo: inChat.ID()})
	if q, ok := answer.Quote(); err != nil || !ok || q.Audios != 1 {
		t.Fatalf("the quote of songs = %+v, %v", answer, err)
	}

	carried, err := f.dialogs.Send(ctx, cat, bob, service.Draft{Forward: []uuid.UUID{inChat.ID()}})
	if err != nil || len(carried.Forwarded()) != 1 || !slices.Equal(carried.Forwarded()[0].Audios, songs[:1]) {
		t.Fatalf("songs of a chat forwarded = %+v, %v", carried, err)
	}
}
