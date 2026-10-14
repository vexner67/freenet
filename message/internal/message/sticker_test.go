package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewSticker(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "Cat", "cat hi", "кот", "cat-hi", strings.Repeat("a", 33)} {
		if _, err := message.NewSticker(name); !errors.Is(err, message.ErrInvalidSticker) {
			t.Errorf("NewSticker(%q) = %v", name, err)
		}
	}

	for _, name := range []string{"cat_hi", "smile_2", strings.Repeat("a", 32)} {
		if got, err := message.NewSticker(name); err != nil || got.String() != name {
			t.Errorf("NewSticker(%q) = %q, %v", name, got, err)
		}
	}
}

func TestStickerMessage(t *testing.T) {
	t.Parallel()

	if _, err := message.NewStickerContent(""); !errors.Is(err, message.ErrInvalidSticker) {
		t.Fatalf("no sticker: %v", err)
	}

	ann, bob := uuid.New(), uuid.New()
	sent := time.Now()

	content, err := message.NewStickerContent("cat_hi")
	if err != nil || content.Sticker() != "cat_hi" {
		t.Fatalf("NewStickerContent = %+v, %v", content, err)
	}
	m, err := message.New(ann, bob, content, sent)
	if err != nil || m.Sticker() != "cat_hi" || m.Sealed() != "" {
		t.Fatalf("New = %+v, %v", m, err)
	}

	// A sticker is not edited into words or photos.
	if _, err = m.Edited(ann, message.Revision{Sealed: seal("привет")}, sent); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("Edited: %v", err)
	}

	// Carried on, it is the sticker still.
	if carried := m.Carried(); len(carried) != 1 || carried[0].Sticker != "cat_hi" || carried[0].SenderID != ann {
		t.Fatalf("Carried = %+v", carried)
	}

	// Taken back, it is nothing.
	if gone, err := m.TakenBack(ann, sent.Add(time.Minute)); err != nil || gone.Sticker() != "" {
		t.Fatalf("TakenBack = %+v, %v", gone, err)
	}

	chat, err := message.NewChatMessage(uuid.New(), ann, content, sent)
	if err != nil || chat.Sticker() != "cat_hi" {
		t.Fatalf("NewChatMessage = %+v, %v", chat, err)
	}
	if _, err = chat.Edited(ann, message.Revision{Text: "привет"}, sent); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("chat message Edited: %v", err)
	}
}
