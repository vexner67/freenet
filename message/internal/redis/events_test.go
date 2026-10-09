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
		ID: uuid.New(), SenderID: ann, RecipientID: bob, Content: message.RestoreContent(text, message.Photos{uuid.New()}, uuid.Nil), CreatedAt: time.Date(2026, 10, 7, 12, 0, 0, 123000, time.UTC),
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

	if _, err = codec.Encode(ann, service.Event{}); err == nil {
		t.Error("Encode of an empty event = nil, want an error")
	}
	for _, bad := range []string{`not json`, `{"user_id":"` + ann.String() + `"}`, `{"message":{"text":"   "}}`} {
		if _, _, err = codec.Decode([]byte(bad)); err == nil {
			t.Errorf("Decode(%s) = nil, want an error", bad)
		}
	}
}
