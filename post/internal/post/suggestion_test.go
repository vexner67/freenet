package post_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestSuggestion(t *testing.T) {
	t.Parallel()

	club, ann := uuid.New(), uuid.New()
	photo := uuid.New()
	content, err := post.NewContent("Котик нашёлся", post.Photos{photo})
	if err != nil {
		t.Fatal(err)
	}

	s, err := post.NewSuggestion(club, ann, content, time.Now())
	if err != nil || s.WallID() != club || s.AuthorID() != ann || s.Content().Text() != "Котик нашёлся" {
		t.Fatalf("NewSuggestion = %+v, %v", s, err)
	}

	repost, _ := post.NewRepost(uuid.New(), "")
	if _, err = post.NewSuggestion(club, ann, repost, time.Now()); !errors.Is(err, post.ErrInvalidRepost) {
		t.Fatalf("a suggested repost: %v", err)
	}

	if !s.CanBeWithdrawnBy(ann) || s.CanBeWithdrawnBy(uuid.New()) {
		t.Fatal("only the author withdraws it by themselves")
	}

	later := time.Now().Add(time.Hour)
	p, err := s.Published("Котик нашёлся!", true, later)
	if err != nil {
		t.Fatal(err)
	}
	if p.WallID() != club || p.AuthorID() != ann || !p.FromWall() || !p.Signed() || p.Content().Text() != "Котик нашёлся!" ||
		len(p.Content().Photos()) != 1 || !p.CreatedAt().Equal(later.UTC().Truncate(time.Microsecond)) || p.ID() == s.ID() {
		t.Fatalf("published = %+v", p)
	}

	// The published post is the community's: its author neither edits nor
	// deletes it, and is not told it is theirs.
	if p.BelongsTo(ann) || p.CanBeDeletedBy(ann) || !p.Suggested() {
		t.Fatal("the author keeps the published post")
	}
	if _, err = p.Edited(ann, "правка", nil, later); !errors.Is(err, post.ErrCannotEdit) {
		t.Fatalf("the author edits the published post: %v", err)
	}

	// The admin may drop the words of a suggestion with photos, not of one
	// without.
	if _, err = s.Published("", false, later); err != nil {
		t.Fatalf("photos only: %v", err)
	}
	words, _ := post.NewContent("Только слова", nil)
	s2, _ := post.NewSuggestion(club, ann, words, time.Now())
	if _, err = s2.Published("", false, later); !errors.Is(err, post.ErrTextEmpty) {
		t.Fatalf("nothing left: %v", err)
	}
}
