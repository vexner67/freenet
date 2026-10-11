package post_test

import (
	"errors"
	"strings"
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

	// The archive shows the posts put away, the other tabs the rest.
	away := post.Restore(post.Stored{ID: uuid.New(), WallID: ann, AuthorID: ann, Content: content, Archived: true})
	if !post.WallArchive.Matches(away) || post.WallAll.Matches(away) || post.WallOwner.Matches(away) || post.WallArchive.Matches(own) {
		t.Error("an archived post is in the archive only, and nothing else is")
	}
	if !away.Archived() || !away.OnWallOf(ann) || away.OnWallOf(bob) {
		t.Error("ann's archived post is on ann's wall")
	}

	for _, s := range []string{"", "owner", "others", "archive"} {
		if _, err := post.NewWallFilter(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	if _, err := post.NewWallFilter("mine"); err != post.ErrInvalidWallFilter {
		t.Errorf("mine: %v", err)
	}
}

func TestWallQuery(t *testing.T) {
	t.Parallel()

	ann := uuid.New()
	p, _ := post.New(ann, ann, post.RestoreContent("Еду на МОРЕ, 100% отдых", nil, uuid.Nil), time.Now())

	for in, want := range map[string]bool{"": true, "  море ": true, "100%": true, "горы": false, "м_ре": false} {
		q, err := post.NewWallQuery(in)
		if err != nil {
			t.Fatalf("NewWallQuery(%q): %v", in, err)
		}
		if q.Matches(p) != want {
			t.Errorf("%q matches = %v, want %v", in, q.Matches(p), want)
		}
	}

	if _, err := post.NewWallQuery(strings.Repeat("я", 100)); err != nil {
		t.Errorf("100 characters: %v", err)
	}
	if _, err := post.NewWallQuery(strings.Repeat("я", 101)); !errors.Is(err, post.ErrWallQueryTooLong) {
		t.Errorf("101 characters: %v", err)
	}
}

func TestWallYear(t *testing.T) {
	t.Parallel()

	ann := uuid.New()
	// The last second of 2024 in UTC is 2025 already in Moscow: the year
	// is that of UTC.
	at := time.Date(2024, time.December, 31, 23, 59, 59, 0, time.UTC).In(time.FixedZone("MSK", 3*60*60))
	p, _ := post.New(ann, ann, post.RestoreContent("с наступающим", nil, uuid.Nil), at)

	for year, want := range map[int]bool{0: true, 2024: true, 2025: false, 2023: false} {
		y, err := post.NewWallYear(year)
		if err != nil {
			t.Fatalf("NewWallYear(%d): %v", year, err)
		}
		if y.Matches(p) != want {
			t.Errorf("%d matches = %v, want %v", year, y.Matches(p), want)
		}
	}
	if post.YearOf(at) != 2024 {
		t.Errorf("YearOf = %d", post.YearOf(at))
	}

	from, to, ok := post.WallYear(2024).Span()
	if !ok || !from.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Span = %v, %v, %v", from, to, ok)
	}
	if _, _, ok = post.WallYear(0).Span(); ok {
		t.Error("every year has a span")
	}

	for _, year := range []int{-1, 1969, 10000} {
		if _, err := post.NewWallYear(year); !errors.Is(err, post.ErrInvalidWallYear) {
			t.Errorf("NewWallYear(%d): %v", year, err)
		}
	}
	for _, year := range []int{1970, 9999} {
		if _, err := post.NewWallYear(year); err != nil {
			t.Errorf("NewWallYear(%d): %v", year, err)
		}
	}
}
