package message_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
)

func TestNewDocuments(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	many := make([]uuid.UUID, 11)
	for i := range many {
		many[i] = uuid.New()
	}

	for name, tc := range map[string]struct {
		ids  []uuid.UUID
		want error
	}{
		"none":     {nil, nil},
		"one":      {[]uuid.UUID{id}, nil},
		"ten":      {many[:10], nil},
		"eleven":   {many, message.ErrTooManyDocuments},
		"twice":    {[]uuid.UUID{id, id}, message.ErrDuplicateDocument},
		"a nil id": {[]uuid.UUID{uuid.Nil}, message.ErrInvalidDocument},
	} {
		if _, err := message.NewDocuments(tc.ids); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}

	if docs, _ := message.NewDocuments(nil); docs != nil {
		t.Errorf("none = %v, want nil", docs)
	}
}

func TestDocumentContent(t *testing.T) {
	t.Parallel()

	docs := message.Documents{uuid.New()}

	if _, err := message.NewDocumentContent(nil); !errors.Is(err, message.ErrTextEmpty) {
		t.Fatalf("no documents: %v", err)
	}

	content, err := message.NewDocumentContent(docs)
	if err != nil || len(content.Documents()) != 1 {
		t.Fatalf("NewDocumentContent = %+v, %v", content, err)
	}

	sticker, _ := message.NewStickerContent("cat_hi")
	if _, err = sticker.Attaching(docs); !errors.Is(err, message.ErrStickerAlone) {
		t.Fatalf("a sticker with documents: %v", err)
	}
	if same, err := sticker.Attaching(nil); err != nil || same.Sticker() != "cat_hi" {
		t.Fatalf("a sticker with no documents = %+v, %v", same, err)
	}

	worded, _ := message.NewContent("вот", nil, uuid.Nil, nil)
	if worded, err = worded.Attaching(docs); err != nil || len(worded.Documents()) != 1 || worded.Text() != "вот" {
		t.Fatalf("words with documents = %+v, %v", worded, err)
	}
}
