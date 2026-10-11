package redis_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/redis"
	"github.com/vexner67/freenet/message/internal/service"
)

func TestEventCodec(t *testing.T) {
	t.Parallel()

	var codec redis.EventCodec
	ann, bob := uuid.New(), uuid.New()

	text, err := message.NewText("привет\nкак дела? 👋")
	if err != nil {
		t.Fatal(err)
	}
	sent := message.Restore(message.Stored{
		ID: uuid.New(), SenderID: ann, RecipientID: bob, Content: message.RestoreContent(text, message.Photos{uuid.New()}, uuid.Nil, nil), CreatedAt: time.Date(2026, 10, 7, 12, 0, 0, 123000, time.UTC),
	})

	data, err := codec.Encode(bob, service.Event{Message: sent})
	if err != nil {
		t.Fatalf("Encode message: %v", err)
	}

	key, e, err := codec.Decode(data)
	if err != nil {
		t.Fatalf("Decode message: %v", err)
	}
	got := e.Message
	if key != bob || got == nil || e.Read != nil || got.ID() != sent.ID() || got.SenderID() != ann || got.RecipientID() != bob ||
		got.Text() != text || !got.CreatedAt().Equal(sent.CreatedAt()) {
		t.Fatalf("message round trip = %v %+v", key, got)
	}

	read := &service.ReadMark{ReaderID: bob, PeerID: ann, UpTo: sent.ID()}
	if data, err = codec.Encode(ann, service.Event{Read: read}); err != nil {
		t.Fatalf("Encode read: %v", err)
	}
	if key, e, err = codec.Decode(data); err != nil || key != ann || e.Message != nil || e.Read == nil || *e.Read != *read {
		t.Fatalf("read round trip = %v %+v %v", key, e.Read, err)
	}

	deleted := &service.Deleted{ID: sent.ID(), SenderID: ann, RecipientID: bob}
	if data, err = codec.Encode(bob, service.Event{Deleted: deleted}); err != nil {
		t.Fatalf("Encode deleted: %v", err)
	}
	if key, e, err = codec.Decode(data); err != nil || key != bob || e.Message != nil || e.Read != nil || e.Deleted == nil || *e.Deleted != *deleted {
		t.Fatalf("deleted round trip = %v %+v %v", key, e.Deleted, err)
	}

	share, err := message.NewPhotoShare("", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	shared := message.Restore(message.Stored{ID: uuid.New(), SenderID: ann, RecipientID: bob, Content: share, CreatedAt: sent.CreatedAt()})
	if data, err = codec.Encode(bob, service.Event{Message: shared}); err != nil {
		t.Fatalf("Encode a shared photo: %v", err)
	}
	if _, e, err = codec.Decode(data); err != nil || e.Message == nil || e.Message.SharedPhoto() != shared.SharedPhoto() || e.Message.Text() != "" {
		t.Fatalf("a shared photo round trip = %+v, %v", e.Message, err)
	}

	carried := message.Forwarded{
		SenderID: uuid.New(), Text: "смотри", Photos: message.Photos{uuid.New()}, Post: uuid.New(), SharedPhoto: uuid.New(),
		CreatedAt: sent.CreatedAt().Add(-time.Hour),
	}
	forward := message.Restore(message.Stored{
		ID: uuid.New(), SenderID: ann, RecipientID: bob, Content: message.RestoreContent("", nil, uuid.Nil, message.Forwards{carried}), CreatedAt: sent.CreatedAt(),
	})
	if data, err = codec.Encode(bob, service.Event{Message: forward}); err != nil {
		t.Fatalf("Encode forward: %v", err)
	}
	if _, e, err = codec.Decode(data); err != nil || e.Message == nil || len(e.Message.Forwarded()) != 1 {
		t.Fatalf("forward round trip = %+v, %v", e.Message, err)
	}
	if got := e.Message.Forwarded()[0]; got.SenderID != carried.SenderID || got.Text != carried.Text || got.Photos[0] != carried.Photos[0] ||
		got.Post != carried.Post || !got.CreatedAt.Equal(carried.CreatedAt) {
		t.Fatalf("forwarded = %+v, want %+v", got, carried)
	}

	gone, err := sent.TakenBack(ann, sent.CreatedAt().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if data, err = codec.Encode(bob, service.Event{Edited: gone}); err != nil {
		t.Fatalf("Encode taken back: %v", err)
	}
	if _, e, err = codec.Decode(data); err != nil || e.Edited == nil || !e.Edited.IsDeleted() || e.Edited.Text() != "" ||
		!e.Edited.DeletedAt().Equal(gone.DeletedAt()) {
		t.Fatalf("taken back round trip = %+v, %v", e.Edited, err)
	}

	edit, err := sent.Edited(ann, message.Revision{Text: "привет!"}, sent.CreatedAt().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if data, err = codec.Encode(bob, service.Event{Edited: edit}); err != nil {
		t.Fatalf("Encode edited: %v", err)
	}
	if key, e, err = codec.Decode(data); err != nil || key != bob || e.Message != nil || e.Edited == nil ||
		e.Edited.ID() != sent.ID() || e.Edited.Text() != "привет!" || !e.Edited.EditedAt().Equal(edit.EditedAt()) || len(e.Edited.Photos()) != 1 {
		t.Fatalf("edited round trip = %v %+v %v", key, e.Edited, err)
	}

	if _, err = codec.Encode(ann, service.Event{}); err == nil {
		t.Error("Encode of an empty event = nil, want an error")
	}
	for _, bad := range []string{`not json`, `{"user_id":"` + ann.String() + `"}`, `{"message":{"text":"   "}}`} {
		if _, _, err = codec.Decode([]byte(bad)); err == nil {
			t.Errorf("Decode(%s) = nil, want an error", bad)
		}
	}
}

func TestEventCodec_Chat(t *testing.T) {
	t.Parallel()

	var codec redis.EventCodec
	ann, bob, chat := uuid.New(), uuid.New(), uuid.New()
	at := time.Date(2026, 10, 11, 12, 0, 0, 123000, time.UTC)

	written := message.RestoreChatMessage(message.StoredChatMessage{
		ID: uuid.New(), ChatID: chat, SenderID: ann, Text: "привет 👋", Photos: message.Photos{uuid.New()}, CreatedAt: at, EditedAt: at.Add(time.Minute),
	})

	data, err := codec.Encode(bob, service.Event{Chat: &service.ChatEvent{Message: written}})
	if err != nil {
		t.Fatalf("Encode a message: %v", err)
	}

	key, e, err := codec.Decode(data)
	if err != nil || key != bob || e.Message != nil || e.Chat == nil || e.Chat.Message == nil {
		t.Fatalf("Decode a message = %v, %+v, %v", key, e, err)
	}
	if got := e.Chat.Message; got.ID() != written.ID() || got.ChatID() != chat || got.SenderID() != ann || got.Text() != written.Text() ||
		len(got.Photos()) != 1 || !got.CreatedAt().Equal(at) || !got.EditedAt().Equal(written.EditedAt()) || got.IsAction() {
		t.Fatalf("message round trip = %+v", got)
	}

	kicked := message.RestoreChatMessage(message.StoredChatMessage{
		ID: uuid.New(), ChatID: chat, SenderID: ann, Action: message.ActionKick, TargetID: bob, CreatedAt: at, DeletedAt: at,
	})
	if data, err = codec.Encode(bob, service.Event{Chat: &service.ChatEvent{Edited: kicked}}); err != nil {
		t.Fatalf("Encode an action: %v", err)
	}
	if _, e, err = codec.Decode(data); err != nil || e.Chat == nil || e.Chat.Edited == nil || e.Chat.Edited.Action() != message.ActionKick ||
		e.Chat.Edited.TargetID() != bob || !e.Chat.Edited.IsDeleted() {
		t.Fatalf("action round trip = %+v, %v", e.Chat, err)
	}

	read := &service.ChatRead{ChatID: chat, UpTo: written.ID()}
	if data, err = codec.Encode(ann, service.Event{Chat: &service.ChatEvent{Read: read}}); err != nil {
		t.Fatalf("Encode read: %v", err)
	}
	if key, e, err = codec.Decode(data); err != nil || key != ann || e.Chat == nil || e.Chat.Read == nil || *e.Chat.Read != *read {
		t.Fatalf("read round trip = %v, %+v, %v", key, e.Chat, err)
	}

	// What comes from Redis is checked as any input is.
	for name, bad := range map[string]string{
		"an unknown action": `{"user_id":"` + ann.String() + `","chat":{"message":{"action":"explode"}}}`,
		"nothing":           `{"user_id":"` + ann.String() + `","chat":{}}`,
	} {
		if _, _, err = codec.Decode([]byte(bad)); err == nil {
			t.Errorf("%s was decoded", name)
		}
	}
}
