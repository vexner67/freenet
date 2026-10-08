package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewTopicTitle(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in, want string
		err      error
	}{
		{"  Знакомство  ", "Знакомство", nil},
		{"Кто\nгде\tживёт", "Кто где живёт", nil},
		{" \n ", "", post.ErrTitleEmpty},
		{strings.Repeat("я", 127), strings.Repeat("я", 127), nil},
		{strings.Repeat("я", 128), "", post.ErrTitleTooLong},
	} {
		got, err := post.NewTopicTitle(tt.in)
		if !errors.Is(err, tt.err) || got.String() != tt.want {
			t.Errorf("NewTopicTitle(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestTopic(t *testing.T) {
	t.Parallel()

	club, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	own, err := post.NewTopic(club, ann, "Знакомство", false, now)
	if err != nil {
		t.Fatal(err)
	}
	if own.FromWall() || own.Closed() || own.Pinned() || !own.UpdatedAt().Equal(own.CreatedAt()) || !post.RestoreTopic(own.Stored()).CreatedAt().Equal(own.CreatedAt()) {
		t.Fatalf("topic = %+v", own.Stored())
	}

	// The author renames and closes it, but does not pin it.
	closed, err := own.Changed(ann, false, post.TopicChanges{Title: "Знакомимся", Closed: true})
	if err != nil || closed.Title() != "Знакомимся" || !closed.Closed() {
		t.Fatalf("Changed = %+v, %v", closed, err)
	}
	if _, err = own.Changed(ann, false, post.TopicChanges{Title: "x", Pinned: true}); !errors.Is(err, post.ErrCannotPinTopic) {
		t.Fatalf("the author pins: %v", err)
	}
	if _, err = own.Changed(bob, false, post.TopicChanges{Title: "x"}); !errors.Is(err, post.ErrCannotChangeTopic) {
		t.Fatalf("a stranger changes: %v", err)
	}
	pinned, err := own.Changed(bob, true, post.TopicChanges{Title: own.Title(), Pinned: true})
	if err != nil || !pinned.Pinned() {
		t.Fatalf("an admin pins: %+v, %v", pinned, err)
	}
	if _, err = own.Changed(bob, true, post.TopicChanges{Title: own.Title(), Restricted: true}); !errors.Is(err, post.ErrRestrictOwnTopic) {
		t.Fatalf("a user's topic restricted: %v", err)
	}

	// Nobody writes in a closed topic; only the admins in a restricted one.
	if err = closed.CheckWrite(true); !errors.Is(err, post.ErrTopicClosed) {
		t.Fatalf("closed: %v", err)
	}
	news, _ := post.NewTopic(club, bob, "Новости", true, now)
	restricted, err := news.Changed(bob, true, post.TopicChanges{Title: news.Title(), Restricted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = restricted.CheckWrite(false); !errors.Is(err, post.ErrTopicRestricted) {
		t.Fatalf("restricted, a member: %v", err)
	}
	if err = restricted.CheckWrite(true); err != nil {
		t.Fatalf("restricted, an admin: %v", err)
	}
	if !restricted.CanBeChangedBy(bob, false) || restricted.CanBeChangedBy(ann, false) {
		t.Fatal("the author of the community's topic still changes it, others do not")
	}
}

func TestTopicMessage(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	topic, _ := post.NewTopic(uuid.New(), ann, "Знакомство", false, time.Now())
	content, _ := post.NewCommentContent("Привет", nil)

	m, err := post.NewTopicMessage(topic, bob, content, false, time.Now())
	if err != nil || m.TopicID() != topic.ID() || m.AuthorID() != bob || m.FromWall() {
		t.Fatalf("message = %+v, %v", m, err)
	}
	if !m.CanBeDeletedBy(bob, false) || m.CanBeDeletedBy(ann, false) || !m.CanBeDeletedBy(ann, true) {
		t.Fatal("its author or the admins delete it, the topic's author does not")
	}
}
