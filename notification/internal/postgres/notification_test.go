package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/postgres"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func TestRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewNotificationRepository(pool)
	marks := postgres.NewReadMarkRepository(pool)

	ann, bob := uuid.New(), uuid.New()
	post, commentID := uuid.New(), uuid.New()

	events := []notification.Event{
		{Kind: notification.KindFollow, RecipientID: ann, ActorID: bob},
		{Kind: notification.KindLike, RecipientID: ann, ActorID: bob, PostID: post},
		{Kind: notification.KindComment, RecipientID: ann, ActorID: bob, PostID: post, CommentID: commentID},
	}

	var saved []*notification.Notification
	for _, e := range append(events, events...) { // each twice: saved once
		n, err := notification.New(e, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, n); err != nil {
			t.Fatalf("Save: %v", err)
		}
		saved = append(saved, n)
	}

	t.Run("round trip and dedup", func(t *testing.T) {
		got, err := repo.Get(ctx, saved[2].ID())
		if err != nil || *got != *saved[2] {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, saved[2])
		}

		if _, err = repo.Get(ctx, saved[3].ID()); !errors.Is(err, notification.ErrNotFound) {
			t.Fatalf("a repeated event was saved: %v", err)
		}

		all, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
		if err != nil || len(all) != 3 || all[0].Event().Kind != notification.KindComment {
			t.Fatalf("List = %v, %v", all, err)
		}

		if n, err := repo.CountNewer(ctx, ann, uuid.Nil, 2); err != nil || n != 2 {
			t.Fatalf("CountNewer capped = %d, %v", n, err)
		}
	})

	t.Run("read mark", func(t *testing.T) {
		if mark, err := marks.ReadUpTo(ctx, ann); err != nil || mark != uuid.Nil {
			t.Fatalf("ReadUpTo before reading = %v, %v", mark, err)
		}

		if err := marks.MarkRead(ctx, ann, saved[1].ID()); err != nil {
			t.Fatal(err)
		}
		if err := marks.MarkRead(ctx, ann, saved[0].ID()); err != nil {
			t.Fatal(err)
		}

		mark, err := marks.ReadUpTo(ctx, ann)
		if err != nil || mark != saved[1].ID() {
			t.Fatalf("mark = %v, %v; the newer mark must stay", mark, err)
		}

		unread, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, ReadUpTo: mark, Limit: 10})
		if err != nil || len(unread) != 1 || unread[0].ID() != saved[2].ID() {
			t.Fatalf("unread = %v, %v", unread, err)
		}

		archive, err := repo.List(ctx, service.Query{RecipientID: ann, ReadUpTo: mark, Limit: 10})
		if err != nil || len(archive) != 2 || archive[0].ID() != saved[1].ID() {
			t.Fatalf("archive = %v, %v", archive, err)
		}

		older, err := repo.List(ctx, service.Query{RecipientID: ann, ReadUpTo: mark, Before: archive[0].ID(), Limit: 10})
		if err != nil || len(older) != 1 || older[0].ID() != saved[0].ID() {
			t.Fatalf("archive after cursor = %v, %v", older, err)
		}
	})

	t.Run("retract and forget", func(t *testing.T) {
		for _, e := range []notification.Event{events[0], events[2]} {
			if err := repo.Delete(ctx, e); err != nil {
				t.Fatalf("Delete(%+v): %v", e, err)
			}
		}

		if err := repo.DeleteByPost(ctx, post); err != nil {
			t.Fatal(err)
		}

		left, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
		if err != nil || len(left) != 0 {
			t.Fatalf("left = %v, %v", left, err)
		}
	})
	t.Run("wall post", func(t *testing.T) {
		wallPost := uuid.New()
		e := notification.Event{Kind: notification.KindWallPost, RecipientID: bob, ActorID: ann, PostID: wallPost}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: bob, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		for range 2 { // one notification per post
			n, err := notification.New(e, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		if got := count(); got != 1 {
			t.Fatalf("notifications about one wall post = %d, want 1", got)
		}

		if err := repo.Delete(ctx, e); err != nil || count() != 0 {
			t.Fatalf("Delete: %v, left %d", err, count())
		}

		n, err := notification.New(e, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, n); err != nil {
			t.Fatal(err)
		}
		if err = repo.DeleteByPost(ctx, wallPost); err != nil || count() != 0 {
			t.Fatalf("DeleteByPost: %v, left %d", err, count())
		}
	})
	t.Run("repost", func(t *testing.T) {
		repost := uuid.New()
		e := notification.Event{Kind: notification.KindRepost, RecipientID: bob, ActorID: ann, PostID: repost}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: bob, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		for range 2 { // one notification per repost
			n, err := notification.New(e, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		if got := count(); got != 1 {
			t.Fatalf("notifications about one repost = %d, want 1", got)
		}

		if err := repo.DeleteByPost(ctx, repost); err != nil || count() != 0 {
			t.Fatalf("a deleted repost takes its notification: %v, left %d", err, count())
		}
	})
	t.Run("gift", func(t *testing.T) {
		first, second := uuid.New(), uuid.New()

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: bob, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		before := count()

		// One notification per sent gift, however many one person sends.
		for _, gift := range []uuid.UUID{first, first, second} {
			n, err := notification.New(notification.Event{Kind: notification.KindGift, RecipientID: bob, ActorID: ann, PostID: gift}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}

		if got := count() - before; got != 2 {
			t.Fatalf("notifications about two gifts = %d, want 2", got)
		}

		// A deleted gift takes its own back.
		if err := repo.Delete(ctx, notification.Event{Kind: notification.KindGift, RecipientID: bob, ActorID: ann, PostID: first}); err != nil {
			t.Fatal(err)
		}

		if got := count() - before; got != 1 {
			t.Fatalf("notifications after a deleted gift = %d, want 1", got)
		}

		if err := repo.Delete(ctx, notification.Event{Kind: notification.KindGift, RecipientID: bob, ActorID: ann, PostID: second}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("accepted post", func(t *testing.T) {
		published, club := uuid.New(), uuid.New()
		e := notification.Event{Kind: notification.KindPostAccepted, RecipientID: bob, ActorID: club, PostID: published}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: bob, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		for range 2 { // one notification per post
			n, err := notification.New(e, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		if got := count(); got != 1 {
			t.Fatalf("notifications about one accepted post = %d, want 1", got)
		}

		if err := repo.Delete(ctx, e); err != nil || count() != 0 {
			t.Fatalf("Delete: %v, left %d", err, count())
		}
	})
	t.Run("topic message", func(t *testing.T) {
		topic, first, second, starter := uuid.New(), uuid.New(), uuid.New(), uuid.New()

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: starter, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		for range 2 { // one notification per message
			for _, message := range []uuid.UUID{first, second} {
				n, err := notification.New(notification.Event{
					Kind: notification.KindTopicMessage, RecipientID: starter, ActorID: bob, PostID: topic, CommentID: message,
				}, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if err = repo.Save(ctx, n); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}
		}
		if got := count(); got != 2 {
			t.Fatalf("notifications about two messages = %d, want 2", got)
		}

		// A deleted message takes its own, a deleted topic the rest.
		if err := repo.DeleteByComment(ctx, first); err != nil || count() != 1 {
			t.Fatalf("DeleteByComment: %v, left %d", err, count())
		}
		if err := repo.DeleteByPost(ctx, topic); err != nil || count() != 0 {
			t.Fatalf("DeleteByPost: %v, left %d", err, count())
		}
	})
	t.Run("mentions", func(t *testing.T) {
		ann, bob, post, comment, topic, message := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
		events := []notification.Event{
			{Kind: notification.KindPostMention, RecipientID: ann, ActorID: bob, PostID: post},
			{Kind: notification.KindCommentMention, RecipientID: ann, ActorID: bob, PostID: post, CommentID: comment},
			{Kind: notification.KindTopicMention, RecipientID: ann, ActorID: bob, PostID: topic, CommentID: message},
		}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		save := func() {
			t.Helper()

			for _, e := range events {
				n, err := notification.New(e, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if err = repo.Save(ctx, n); err != nil {
					t.Fatalf("Save(%s): %v", e.Kind, err)
				}
			}
		}

		save()
		save() // one per person and text
		if got := count(); got != 3 {
			t.Fatalf("mentions = %d, want 3", got)
		}

		for i, e := range events {
			if err := repo.Delete(ctx, e); err != nil || count() != 2-i {
				t.Fatalf("Delete(%s): %v, left %d", e.Kind, err, count())
			}
		}

		// The deleted comment takes the mention in it, the post its own.
		save()
		if err := repo.DeleteByComment(ctx, comment); err != nil || count() != 2 {
			t.Fatalf("DeleteByComment: %v, left %d", err, count())
		}
		if err := repo.DeleteByPost(ctx, post); err != nil || count() != 1 {
			t.Fatalf("DeleteByPost: %v, left %d", err, count())
		}
		if err := repo.DeleteByPost(ctx, topic); err != nil || count() != 0 {
			t.Fatalf("DeleteByPost(topic): %v, left %d", err, count())
		}
	})
	t.Run("mention under a photo", func(t *testing.T) {
		ann, bob, photo, comment := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		e := notification.Event{Kind: notification.KindPhotoMention, RecipientID: ann, ActorID: bob, PostID: photo, CommentID: comment}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}
		save := func() {
			t.Helper()

			n, err := notification.New(e, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}

		save()
		save() // one per person and comment
		if got := count(); got != 1 {
			t.Fatalf("mentions = %d, want 1", got)
		}
		if err := repo.Delete(ctx, e); err != nil || count() != 0 {
			t.Fatalf("Delete: %v, left %d", err, count())
		}

		// The deleted comment takes it, and so does the deleted photo.
		save()
		if err := repo.DeleteByComment(ctx, comment); err != nil || count() != 0 {
			t.Fatalf("DeleteByComment: %v, left %d", err, count())
		}
		save()
		if err := repo.DeleteByPost(ctx, photo); err != nil || count() != 0 {
			t.Fatalf("DeleteByPost: %v, left %d", err, count())
		}
	})
	t.Run("comment under a note", func(t *testing.T) {
		ann, bob, note, comment := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		e := notification.Event{Kind: notification.KindNoteComment, RecipientID: ann, ActorID: bob, PostID: note, CommentID: comment}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}
		save := func() {
			t.Helper()

			n, err := notification.New(e, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}

		save()
		save() // one per comment
		if got := count(); got != 1 {
			t.Fatalf("comments = %d, want 1", got)
		}
		if err := repo.Delete(ctx, e); err != nil || count() != 0 {
			t.Fatalf("Delete: %v, left %d", err, count())
		}

		// The deleted comment takes it, and so does the deleted note.
		save()
		if err := repo.DeleteByComment(ctx, comment); err != nil || count() != 0 {
			t.Fatalf("DeleteByComment: %v, left %d", err, count())
		}
		save()
		if err := repo.DeleteByPost(ctx, note); err != nil || count() != 0 {
			t.Fatalf("DeleteByPost: %v, left %d", err, count())
		}
	})
	t.Run("photos and topic likes", func(t *testing.T) {
		ann, bob, photo, comment, topic, message := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
		events := []notification.Event{
			{Kind: notification.KindPhotoLike, RecipientID: ann, ActorID: bob, PostID: photo},
			{Kind: notification.KindPhotoComment, RecipientID: ann, ActorID: bob, PostID: photo, CommentID: comment},
			{Kind: notification.KindTopicMessageLike, RecipientID: ann, ActorID: bob, PostID: topic, CommentID: message},
		}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: ann, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		save := func() {
			t.Helper()

			for _, e := range events {
				n, err := notification.New(e, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if err = repo.Save(ctx, n); err != nil {
					t.Fatalf("Save(%s): %v", e.Kind, err)
				}
			}
		}

		save()
		save() // one per like and comment
		if got := count(); got != 3 {
			t.Fatalf("notifications = %d, want 3", got)
		}

		for i, e := range events {
			if err := repo.Delete(ctx, e); err != nil || count() != 2-i {
				t.Fatalf("Delete(%s): %v, left %d", e.Kind, err, count())
			}
		}

		// The deleted comment takes its notification, the photo its own.
		save()
		if err := repo.DeleteByComment(ctx, comment); err != nil || count() != 2 {
			t.Fatalf("DeleteByComment: %v, left %d", err, count())
		}
		if err := repo.DeleteByPost(ctx, photo); err != nil || count() != 1 {
			t.Fatalf("DeleteByPost: %v, left %d", err, count())
		}
		if err := repo.DeleteByPost(ctx, topic); err != nil || count() != 0 {
			t.Fatalf("DeleteByPost(topic): %v, left %d", err, count())
		}
	})
	t.Run("club admin", func(t *testing.T) {
		club, admin := uuid.New(), uuid.New()
		e := notification.Event{Kind: notification.KindClubAdmin, RecipientID: admin, ActorID: club}

		count := func() int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: admin, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		for range 2 { // one notification per community and admin
			n, err := notification.New(e, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		if got := count(); got != 1 {
			t.Fatalf("notifications about one appointment = %d, want 1", got)
		}

		// A reminder of an event is another notification of the same two.
		reminder := notification.Event{Kind: notification.KindEventReminder, RecipientID: admin, ActorID: club}
		for range 2 {
			n, err := notification.New(reminder, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = repo.Save(ctx, n); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		if got := count(); got != 2 {
			t.Fatalf("with a reminder = %d, want 2", got)
		}
		if err := repo.Delete(ctx, reminder); err != nil || count() != 1 {
			t.Fatalf("Delete(reminder): %v, left %d", err, count())
		}

		if err := repo.Delete(ctx, e); err != nil || count() != 0 {
			t.Fatalf("Delete: %v, left %d", err, count())
		}
	})
	t.Run("suggested post", func(t *testing.T) {
		suggestion, club, admin := uuid.New(), uuid.New(), uuid.New()

		count := func(recipient uuid.UUID) int {
			t.Helper()

			got, err := repo.List(ctx, service.Query{RecipientID: recipient, Unread: true, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}

			return len(got)
		}

		for range 2 { // one notification per suggestion and admin
			for _, recipient := range []uuid.UUID{bob, admin} {
				e := notification.Event{Kind: notification.KindSuggestedPost, RecipientID: recipient, ActorID: club, PostID: suggestion}

				n, err := notification.New(e, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if err = repo.Save(ctx, n); err != nil {
					t.Fatalf("Save: %v", err)
				}
			}
		}
		if count(bob) != 1 || count(admin) != 1 {
			t.Fatalf("notifications about one suggestion = %d and %d, want 1 each", count(bob), count(admin))
		}

		e := notification.Event{Kind: notification.KindSuggestedPost, RecipientID: admin, ActorID: club, PostID: suggestion}
		if err := repo.Delete(ctx, e); err != nil || count(admin) != 0 || count(bob) != 1 {
			t.Fatalf("Delete takes only its recipient's: %v, left %d and %d", err, count(bob), count(admin))
		}

		if err := repo.DeleteByPost(ctx, suggestion); err != nil || count(bob) != 0 {
			t.Fatalf("the suggestion gone takes the rest: %v, left %d", err, count(bob))
		}
	})
}

func TestRepositories_DeleteByUser(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewNotificationRepository(pool)
	marks := postgres.NewReadMarkRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	var last *notification.Notification
	for _, e := range []notification.Event{
		{Kind: notification.KindFollow, RecipientID: ann, ActorID: bob},
		{Kind: notification.KindFollow, RecipientID: bob, ActorID: ann},
		{Kind: notification.KindFollow, RecipientID: cat, ActorID: bob},
	} {
		if last, err = notification.New(e, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, last); err != nil {
			t.Fatal(err)
		}
	}
	if err = marks.MarkRead(ctx, ann, uuid.Max); err != nil {
		t.Fatal(err)
	}

	if err = repo.DeleteByUser(ctx, ann); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if err = marks.Delete(ctx, ann); err != nil {
		t.Fatalf("Delete mark: %v", err)
	}

	for _, who := range []uuid.UUID{ann, bob} {
		if n, err := repo.CountNewer(ctx, who, uuid.Nil, 10); err != nil || n != 0 {
			t.Errorf("notifications of %s left: %d, %v", who, n, err)
		}
	}
	if n, err := repo.CountNewer(ctx, cat, uuid.Nil, 10); err != nil || n != 1 {
		t.Errorf("cat's notification = %d, %v; want kept", n, err)
	}
	if mark, err := marks.ReadUpTo(ctx, ann); err != nil || mark != uuid.Nil {
		t.Errorf("ann's mark = %v, %v; want none", mark, err)
	}
}

func TestRepositories_CommentReplyAndLike(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewNotificationRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	post, answer, liked := uuid.New(), uuid.New(), uuid.New()

	reply := notification.Event{Kind: notification.KindCommentReply, RecipientID: ann, ActorID: bob, PostID: post, CommentID: answer}
	likeAnswer := notification.Event{Kind: notification.KindCommentLike, RecipientID: bob, ActorID: cat, PostID: post, CommentID: answer}
	likeOther := notification.Event{Kind: notification.KindCommentLike, RecipientID: ann, ActorID: cat, PostID: post, CommentID: liked}

	for _, e := range []notification.Event{reply, likeAnswer, likeOther, reply, likeOther} { // repeats saved once
		n, err := notification.New(e, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, n); err != nil {
			t.Fatalf("Save %s: %v", e.Kind, err)
		}
	}

	count := func(userID uuid.UUID) int {
		t.Helper()

		page, err := repo.List(ctx, service.Query{RecipientID: userID, Unread: true, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		return len(page)
	}

	if got := count(ann); got != 2 {
		t.Fatalf("ann has %d notifications, want the reply and the like once each", got)
	}

	if err = repo.Delete(ctx, likeOther); err != nil {
		t.Fatal(err)
	}
	if got := count(ann); got != 1 {
		t.Fatalf("after the unlike ann has %d", got)
	}

	// The answer is deleted: its reply and its like go.
	if err = repo.DeleteByComment(ctx, answer); err != nil {
		t.Fatal(err)
	}
	if count(ann) != 0 || count(bob) != 0 {
		t.Fatalf("after deleting the answer: ann %d, bob %d", count(ann), count(bob))
	}
}

func TestRepositories_DeleteOlder(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := postgres.NewNotificationRepository(pool)
	ann := uuid.New()

	// Three from ten years ago, older than anything another test left,
	// and one of today.
	old := time.Now().Add(-10 * 365 * 24 * time.Hour)
	for i := range 3 {
		id := notification.Before(old.Add(time.Duration(i) * time.Millisecond))
		n := notification.Restore(id, notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: uuid.New()}, old)

		if err = repo.Save(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	fresh, err := notification.New(notification.Event{Kind: notification.KindFollow, RecipientID: ann, ActorID: uuid.New()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	before := notification.Before(old.Add(time.Hour))

	// A batch at a time, the oldest first.
	if n, err := repo.DeleteOlder(ctx, before, 2); err != nil || n != 2 {
		t.Fatalf("DeleteOlder = %d, %v; want a batch of two", n, err)
	}
	if _, err = repo.Get(ctx, notification.Before(old.Add(2*time.Millisecond))); err != nil {
		t.Fatalf("the newest of the old went first: %v", err)
	}
	if n, err := repo.DeleteOlder(ctx, before, 2); err != nil || n != 1 {
		t.Fatalf("DeleteOlder = %d, %v; want the last one", n, err)
	}
	if n, err := repo.DeleteOlder(ctx, before, 2); err != nil || n != 0 {
		t.Fatalf("DeleteOlder again = %d, %v", n, err)
	}
	if _, err = repo.Get(ctx, fresh.ID()); err != nil {
		t.Fatalf("the one of today: %v", err)
	}
}
