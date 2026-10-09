package post_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestWallFilter(t *testing.T) {
	t.Parallel()

	ann, bob, club := uuid.New(), uuid.New(), uuid.New()
	content := post.RestoreContent("привет", nil, uuid.Nil)

	own, _ := post.New(ann, ann, content, time.Now())
	guest, _ := post.New(ann, bob, content, time.Now())
	member, _ := post.New(club, bob, content, time.Now())
	forClub := post.Restore(post.Stored{ID: uuid.New(), WallID: club, AuthorID: bob, Content: content, FromWall: true})

	for _, tt := range []struct {
		name          string
		p             *post.Post
		owner, others bool
	}{
		{"ann's on her wall", own, true, false},
		{"bob's on ann's wall", guest, false, true},
		{"a member's on a club's wall", member, false, true},
		{"for the club", forClub, true, false},
	} {
		if post.WallOwner.Matches(tt.p) != tt.owner || post.WallOthers.Matches(tt.p) != tt.others || !post.WallAll.Matches(tt.p) {
			t.Errorf("%s: owner %v, others %v", tt.name, post.WallOwner.Matches(tt.p), post.WallOthers.Matches(tt.p))
		}
	}

	for _, s := range []string{"", "owner", "others"} {
		if _, err := post.NewWallFilter(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	if _, err := post.NewWallFilter("mine"); err != post.ErrInvalidWallFilter {
		t.Errorf("mine: %v", err)
	}
}
