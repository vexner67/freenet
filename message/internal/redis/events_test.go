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

	carried := message.Forwarded{SenderID: uuid.New(), Text: "смотри", Photos: message.Photos{uuid.New()}, Post: uuid.New(), CreatedAt: sent.CreatedAt().Add(-time.Hour)}
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

	edit, err := sent.Edited(ann, "привет!", sent.CreatedAt().Add(time.Minute))
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
