package post_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewPostponed(t *testing.T) {
	t.Parallel()

	ann, club := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	words, _ := post.NewContent("Завтра в 10", nil)
	repost, _ := post.NewRepost(uuid.New(), "", nil)
	share, _ := post.NewPhotoShare(uuid.New(), "")
	polled := post.NewPolled("", nil)

	tests := []struct {
		name  string
		draft post.PostponedDraft
		want  error
	}{
		{"own wall", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: now.Add(time.Hour)}, nil},
		{"for a community", post.PostponedDraft{WallID: club, AuthorID: ann, Content: words, FromWall: true, Signed: true, PublishAt: now.Add(time.Hour)}, nil},
		{"a year ahead", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: now.Add(post.MaxPostponedAhead)}, nil},
		{"someone else's wall", post.PostponedDraft{WallID: club, AuthorID: ann, Content: words, PublishAt: now.Add(time.Hour)}, post.ErrPostponedElsewhere},
		{"own wall for a community", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, FromWall: true, PublishAt: now.Add(time.Hour)}, post.ErrPostponedElsewhere},
		{"signed for oneself", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, Signed: true, PublishAt: now.Add(time.Hour)}, post.ErrPostponedElsewhere},
		{"now", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: now}, post.ErrPublishTime},
		{"past", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: now.Add(-time.Hour)}, post.ErrPublishTime},
		{"over a year", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: now.Add(post.MaxPostponedAhead + time.Second)}, post.ErrPublishTime},
		{"repost", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: repost, PublishAt: now.Add(time.Hour)}, post.ErrPostponedContent},
		{"shared photo", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: share, PublishAt: now.Add(time.Hour)}, post.ErrPostponedContent},
		{"poll", post.PostponedDraft{WallID: ann, AuthorID: ann, Content: polled, PublishAt: now.Add(time.Hour)}, post.ErrPostponedContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := post.NewPostponed(tt.draft, now)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewPostponed = %v, want %v", err, tt.want)
			}
			if err == nil && (!p.PublishAt().Equal(tt.draft.PublishAt) || !p.CreatedAt().Equal(now) || p.ID() == uuid.Nil) {
				t.Fatalf("NewPostponed = %+v", p)
			}
		})
	}
}

func TestPostponed_Published(t *testing.T) {
	t.Parallel()

	ann, club, photo := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	content, _ := post.NewContent("Завтра в 10", post.Photos{photo})

	own, err := post.NewPostponed(post.PostponedDraft{
		WallID: ann, AuthorID: ann, Content: content, NSFW: true, PublishAt: now.Add(time.Hour),
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	later := now.Add(time.Hour)
	p, err := own.Published(later)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() == own.ID() || p.WallID() != ann || p.AuthorID() != ann || p.FromWall() || !p.NSFW() ||
		!p.CreatedAt().Equal(later.UTC().Truncate(time.Microsecond)) || len(p.Content().Photos()) != 1 {
		t.Fatalf("published = %+v", p)
	}

	forClub, err := post.NewPostponed(post.PostponedDraft{
		WallID: club, AuthorID: ann, Content: content, FromWall: true, Signed: true, PublishAt: now.Add(time.Hour),
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	if p, err = forClub.Published(later); err != nil || !p.FromWall() || !p.Signed() || p.WallID() != club || p.NSFW() {
		t.Fatalf("published for the community = %+v, %v", p, err)
	}

	// The author changes a post for their own wall by themselves; one for
	// a community is the admins', though the author takes it back.
	if !own.ChangedBy(ann) || own.ChangedBy(uuid.New()) || forClub.ChangedBy(ann) {
		t.Fatal("ChangedBy")
	}
	if !forClub.CanBeDeletedBy(ann) || forClub.CanBeDeletedBy(uuid.New()) {
		t.Fatal("CanBeDeletedBy")
	}
}

func TestPostponed_Edited(t *testing.T) {
	t.Parallel()

	ann, photo := uuid.New(), uuid.New()
	now := time.Now()
	content, _ := post.NewContent("Завтра", post.Photos{photo})

	p, err := post.NewPostponed(post.PostponedDraft{WallID: ann, AuthorID: ann, Content: content, PublishAt: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}

	named := post.Mentions{uuid.New()}
	edited, err := p.Edited(post.PostponedEdit{Text: "", Mentions: named, NSFW: true, PublishAt: now.Add(2 * time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID() != p.ID() || edited.Content().Text() != "" || len(edited.Content().Photos()) != 1 || !edited.NSFW() ||
		len(edited.Mentions()) != 1 || !edited.PublishAt().Equal(now.Add(2*time.Hour).UTC().Truncate(time.Microsecond)) {
		t.Fatalf("edited = %+v", edited)
	}
	if p.Content().Text() != "Завтра" || p.NSFW() {
		t.Fatal("Edited changed the post itself")
	}

	if _, err = p.Edited(post.PostponedEdit{Text: "x", PublishAt: now.Add(-time.Minute)}, now); !errors.Is(err, post.ErrPublishTime) {
		t.Fatalf("edited into the past: %v", err)
	}

	words, _ := post.NewContent("Только слова", nil)
	p, _ = post.NewPostponed(post.PostponedDraft{WallID: ann, AuthorID: ann, Content: words, PublishAt: now.Add(time.Hour)}, now)
	if _, err = p.Edited(post.PostponedEdit{PublishAt: now.Add(time.Hour)}, now); !errors.Is(err, post.ErrTextEmpty) {
		t.Fatalf("edited empty: %v", err)
	}
}
