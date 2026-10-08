package notification_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/notification"
)

func TestEvent_Validate(t *testing.T) {
	t.Parallel()

	ann, bob, post, comment := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name  string
		event notification.Event
		err   error
	}{
		{"like", notification.Event{Kind: notification.KindLike, RecipientID: ann, ActorID: bob, PostID: post}, nil},
		{"comment", notification.Event{Kind: notification.KindComment, RecipientID: ann, ActorID: bob, PostID: post, CommentID: comment}, nil},
		{"follow", notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: bob}, nil},
		{"wall post", notification.Event{Kind: notification.KindWallPost, RecipientID: ann, ActorID: bob, PostID: post}, nil},
		{"wall post without the post", notification.Event{Kind: notification.KindWallPost, RecipientID: ann, ActorID: bob}, notification.ErrTargetMismatch},
		{"repost", notification.Event{Kind: notification.KindRepost, RecipientID: ann, ActorID: bob, PostID: post}, nil},
		{"repost without the post", notification.Event{Kind: notification.KindRepost, RecipientID: ann, ActorID: bob}, notification.ErrTargetMismatch},
		{"wall post with a comment", notification.Event{Kind: notification.KindWallPost, RecipientID: ann, ActorID: bob, PostID: post, CommentID: comment}, notification.ErrTargetMismatch},
		{"like without a post", notification.Event{Kind: notification.KindLike, RecipientID: ann, ActorID: bob}, notification.ErrTargetMismatch},
		{"like with a comment", notification.Event{Kind: notification.KindLike, RecipientID: ann, ActorID: bob, PostID: post, CommentID: comment}, notification.ErrTargetMismatch},
		{"comment without the comment", notification.Event{Kind: notification.KindComment, RecipientID: ann, ActorID: bob, PostID: post}, notification.ErrTargetMismatch},
		{"reply", notification.Event{Kind: notification.KindCommentReply, RecipientID: ann, ActorID: bob, PostID: post, CommentID: comment}, nil},
		{"reply without the answer", notification.Event{Kind: notification.KindCommentReply, RecipientID: ann, ActorID: bob, PostID: post}, notification.ErrTargetMismatch},
		{"comment like", notification.Event{Kind: notification.KindCommentLike, RecipientID: ann, ActorID: bob, PostID: post, CommentID: comment}, nil},
		{"comment like without the post", notification.Event{Kind: notification.KindCommentLike, RecipientID: ann, ActorID: bob, CommentID: comment}, notification.ErrTargetMismatch},
		{"follow with a post", notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: bob, PostID: post}, notification.ErrTargetMismatch},
		{"unknown kind", notification.Event{Kind: "poke", RecipientID: ann, ActorID: bob, PostID: post}, notification.ErrUnknownKind},
		{"no recipient", notification.Event{Kind: notification.KindFollow, ActorID: bob}, notification.ErrNoRecipient},
		{"no actor", notification.Event{Kind: notification.KindFollow, RecipientID: ann}, notification.ErrNoActor},
		{"own action", notification.Event{Kind: notification.KindLike, RecipientID: ann, ActorID: ann, PostID: post}, notification.ErrActorRecipient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.event.Validate(); !errors.Is(err, tt.err) {
				t.Fatalf("Validate = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	e := notification.Event{Kind: notification.KindFollow, RecipientID: uuid.New(), ActorID: uuid.New()}
	now := time.Date(2026, 10, 5, 18, 0, 0, 123456789, time.FixedZone("MSK", 3*3600))

	n, err := notification.New(e, now)
	if err != nil {
		t.Fatal(err)
	}
	if n.Event() != e {
		t.Fatalf("event = %+v", n.Event())
	}
	if n.CreatedAt().Location() != time.UTC || n.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", n.CreatedAt())
	}

	later, err := notification.New(e, now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= n.ID().String() {
		t.Fatal("IDs must grow, notifications are ordered by them")
	}

	if _, err = notification.New(notification.Event{Kind: notification.KindFollow}, now); err == nil {
		t.Fatal("an invalid event makes no notification")
	}
}
