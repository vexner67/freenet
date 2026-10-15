package redis_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/redis"
	"github.com/vexner67/freenet/message/internal/service"
)

func TestEventCodec_RepliesAndReactions(t *testing.T) {
	t.Parallel()

	var codec redis.EventCodec
	ann, bob := uuid.New(), uuid.New()
	asked := uuid.New()
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	reactions := message.Reactions{{UserID: bob, Reaction: message.ReactionHeart}, {UserID: ann, Reaction: message.ReactionFire}}

	answer := message.Restore(message.Stored{
		ID: uuid.New(), SenderID: ann, RecipientID: bob, CreatedAt: at,
		Content: message.RestoreContent("", nil, uuid.Nil, nil).WithSealed(seal("иду")).Answering(asked),
	}).Quoting(message.Quote{ID: asked, SenderID: bob, Photos: 2, Found: true}).Reacting(reactions)

	data, err := codec.Encode(bob, service.Event{Edited: answer})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	_, e, err := codec.Decode(data)
	if err != nil || e.Edited == nil {
		t.Fatalf("Decode: %+v, %v", e, err)
	}

	got := e.Edited
	if q, ok := got.Quote(); got.ReplyTo() != asked || !ok || q.SenderID != bob || q.Photos != 2 || !q.Found {
		t.Fatalf("the quote round trip = %+v, %v", q, ok)
	}
	if r := got.Reactions(); len(r) != 2 || r[0] != reactions[0] || r[1] != reactions[1] {
		t.Fatalf("the reactions round trip = %+v", r)
	}

	// A message of a chat quotes words, and one answered gone for good is
	// known only by its ID.
	gone := uuid.New()
	chatAnswer := message.RestoreChatMessage(message.StoredChatMessage{
		ID: uuid.New(), ChatID: uuid.New(), SenderID: ann, CreatedAt: at,
		Content: message.RestoreContent("я", nil, uuid.Nil, nil).Answering(gone),
	}).Quoting(message.Lost(gone))

	if data, err = codec.Encode(bob, service.Event{Chat: &service.ChatEvent{Edited: chatAnswer}}); err != nil {
		t.Fatalf("Encode a chat's: %v", err)
	}
	if _, e, err = codec.Decode(data); err != nil || e.Chat == nil || e.Chat.Edited == nil {
		t.Fatalf("Decode a chat's: %+v, %v", e, err)
	}
	if q, ok := e.Chat.Edited.Quote(); !ok || q.ID != gone || q.Found || e.Chat.Edited.Reactions() != nil {
		t.Fatalf("a chat's quote round trip = %+v, %v", q, ok)
	}

	// The actions of pinning and of the photo pass between replicas too.
	for _, action := range []message.Action{message.ActionPin, message.ActionUnpin, message.ActionPhoto, message.ActionPhotoRemove} {
		told := message.RestoreChatMessage(message.StoredChatMessage{ID: uuid.New(), ChatID: uuid.New(), SenderID: ann, Action: action, CreatedAt: at})
		if data, err = codec.Encode(bob, service.Event{Chat: &service.ChatEvent{Message: told}}); err != nil {
			t.Fatalf("Encode %s: %v", action, err)
		}
		if _, e, err = codec.Decode(data); err != nil || e.Chat.Message.Action() != action {
			t.Fatalf("%s round trip: %+v, %v", action, e.Chat, err)
		}
	}
}
