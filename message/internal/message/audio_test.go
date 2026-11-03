package message_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewAudios(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	tooMany := make([]uuid.UUID, 11)
	for i := range tooMany {
		tooMany[i] = uuid.New()
	}

	for _, tt := range []struct {
		name string
		ids  []uuid.UUID
		want error
	}{
		{"none", nil, nil},
		{"one", []uuid.UUID{id}, nil},
		{"too many", tooMany, message.ErrTooManyAudios},
		{"twice", []uuid.UUID{id, id}, message.ErrDuplicateAudio},
		{"nil", []uuid.UUID{uuid.Nil}, message.ErrInvalidAudio},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := message.NewAudios(tt.ids); !errors.Is(err, tt.want) {
				t.Fatalf("NewAudios = %v, want %v", err, tt.want)
			}
		})
	}

	if _, err := message.NewAttachedContent(nil, nil); !errors.Is(err, message.ErrTextEmpty) {
		t.Fatalf("nothing attached: %v", err)
	}
	content, err := message.NewAttachedContent(nil, message.Audios{id})
	if err != nil || len(content.Audios()) != 1 {
		t.Fatalf("NewAttachedContent = %+v, %v", content, err)
	}
	sticker, err := message.NewStickerContent("cat_hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sticker.Attaching(nil, message.Audios{id}); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("a sticker with songs: %v", err)
	}
}
