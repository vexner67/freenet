package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// What a deleted page wrote the lists leave out; read by its ID it is not
// found either, whoever asks, and is back with the page.
func TestDeletedPages_ByID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob, club := uuid.New(), uuid.New(), uuid.New()

	store := inmem.NewStore()
	notesRepo := inmem.NewNoteRepository()
	holders := []service.ReactionHolder{store.Holds(), notesRepo}

	posts := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{}, Notifier: &fakeNotifier{},
		Photos: &fakePhotos{}, Moderators: fakeModerators{}, Switches: store.Posts(), Polls: store.Polls(), Pages: store.Holds(),
	})
	comments := service.NewCommentService(service.CommentDeps{
		Posts: store.Posts(), Comments: store.Comments(), Likes: store.CommentLikes(), Tx: fakeTx{}, Notifier: &fakeNotifier{},
		Photos: &fakePhotos{}, Moderators: fakeModerators{}, Pages: store.Holds(),
	})
	notes := service.NewNoteService(service.NoteDeps{Notes: notesRepo, Comments: notesRepo.Comments(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Pages: notesRepo})

	say := func(wallID, authorID uuid.UUID, fromWall bool) *post.Post {
		t.Helper()

		p, err := posts.Create(ctx, service.CreateInput{WallID: wallID, AuthorID: authorID, Content: post.RestoreContent("привет", nil, uuid.Nil), ClubWall: fromWall, FromWall: fromWall})
		if err != nil {
			t.Fatal(err)
		}

		return p
	}

	own, onAnns, forClub, anns := say(bob, bob, false), say(ann, bob, false), say(club, bob, true), say(ann, ann, false)
	onBobs := say(bob, ann, false)

	bobs, err := comments.Create(ctx, service.CommentInput{AuthorID: bob, PostID: anns.ID(), Content: post.RestoreCommentContent("и я", nil)})
	if err != nil {
		t.Fatal(err)
	}
	hers, err := comments.Create(ctx, service.CommentInput{AuthorID: ann, PostID: anns.ID(), Content: post.RestoreCommentContent("да", nil)})
	if err != nil {
		t.Fatal(err)
	}

	note, err := notes.Create(ctx, bob, "Про котов", "Коты спят.")
	if err != nil {
		t.Fatal(err)
	}
	noted, err := notes.Comment(ctx, bob, note.ID(), "ещё")
	if err != nil {
		t.Fatal(err)
	}

	all := []uuid.UUID{own.ID(), onAnns.ID(), forClub.ID(), anns.ID(), onBobs.ID()}

	check := func(when string, gone bool) {
		t.Helper()

		// His posts and those on his wall; what speaks for the club is the
		// club's, and Ann's own post stays.
		for name, p := range map[string]*post.Post{"his own": own, "his on a wall": onAnns, "hers on his wall": onBobs} {
			if _, _, err := posts.Get(ctx, ann, p.ID()); errors.Is(err, post.ErrNotFound) != gone || (!gone && err != nil) {
				t.Errorf("%s: the post %s: %v", when, name, err)
			}
		}
		for name, p := range map[string]*post.Post{"for the club": forClub, "hers": anns} {
			if _, _, err := posts.Get(ctx, ann, p.ID()); err != nil {
				t.Errorf("%s: the post %s: %v", when, name, err)
			}
		}

		want := len(all)
		if gone {
			want = 2
		}
		if found, err := posts.GetMany(ctx, ann, all); err != nil || len(found) != want {
			t.Errorf("%s: GetMany = %d posts, %v; want %d", when, len(found), err, want)
		}

		// A post nobody is shown is not liked.
		if _, err := posts.Like(ctx, ann, own.ID()); errors.Is(err, post.ErrNotFound) != gone {
			t.Errorf("%s: a like of his post: %v", when, err)
		}

		want = 2
		if gone {
			want = 1
		}
		if found, err := comments.GetMany(ctx, []uuid.UUID{bobs.ID(), hers.ID()}); err != nil || len(found) != want {
			t.Errorf("%s: comments = %d, %v; want %d", when, len(found), err, want)
		}

		if _, err := notes.Get(ctx, note.ID()); errors.Is(err, post.ErrNoteNotFound) != gone || (!gone && err != nil) {
			t.Errorf("%s: the note: %v", when, err)
		}
		if _, err := notes.CommentByID(ctx, noted.ID()); errors.Is(err, post.ErrCommentNotFound) != gone || (!gone && err != nil) {
			t.Errorf("%s: the comment of the note: %v", when, err)
		}

		want = 1
		if gone {
			want = 0
		}
		if found, err := notes.CommentsByID(ctx, []uuid.UUID{noted.ID()}); err != nil || len(found) != want {
			t.Errorf("%s: comments of the note = %d, %v; want %d", when, len(found), err, want)
		}
	}

	check("everyone is here", false)

	for _, h := range holders {
		if err := h.Hold(ctx, bob); err != nil {
			t.Fatal(err)
		}
	}
	check("bob deleted the page", true)

	for _, h := range holders {
		if err := h.Release(ctx, bob); err != nil {
			t.Fatal(err)
		}
	}
	check("bob is back", false)
}
