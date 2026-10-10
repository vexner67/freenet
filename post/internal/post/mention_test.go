package post_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewMentions(t *testing.T) {
	t.Parallel()

	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	m, err := post.NewMentions([]uuid.UUID{bob, uuid.Nil, ann, bob, cat})
	if err != nil || !slices.Equal(m, post.Mentions{bob, ann, cat}) {
		t.Fatalf("NewMentions = %v, %v; want each once, in order", m, err)
	}

	if got := m.Heard(ann, cat); !slices.Equal(got, []uuid.UUID{bob}) {
		t.Fatalf("Heard = %v, want bob: not the author, nor who hears anyway", got)
	}

	if none, err := post.NewMentions(nil); err != nil || len(none.Heard(ann)) != 0 {
		t.Fatalf("no mentions = %v, %v", none, err)
	}

	many := make([]uuid.UUID, post.MaxMentions+1)
	for i := range many {
		many[i] = uuid.New()
	}
	if _, err = post.NewMentions(many[:post.MaxMentions]); err != nil {
		t.Fatalf("ten mentions: %v", err)
	}
	if _, err = post.NewMentions(many); !errors.Is(err, post.ErrTooManyMentions) {
		t.Fatalf("eleven mentions: %v", err)
	}
}
