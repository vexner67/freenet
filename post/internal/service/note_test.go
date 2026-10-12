package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type noteFixture struct {
	notes    *service.NoteService
	purge    *service.PurgeService
	notifier *fakeNotifier
}

func newNoteFixture() *noteFixture {
	store := inmem.NewStore()
	repo := inmem.NewNoteRepository()
	f := &noteFixture{notifier: &fakeNotifier{}}

	f.notes = service.NewNoteService(service.NoteDeps{Notes: repo, Comments: repo.Comments(), Tx: fakeTx{}, Notifier: f.notifier})
	f.purge = service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Votes: store.Polls(), Sources: store.Sources(), Comments: store.Comments(),
		Suggestions: inmem.NewSuggestionRepository(), Topics: inmem.NewTopicRepository(), Albums: inmem.NewAlbumRepository(), Notes: repo,
		Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{},
	})

	return f
}

func TestNoteService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newNoteFixture()
	ann, bob := uuid.New(), uuid.New()

	n, err := f.notes.Create(ctx, ann, "Про котов", "Коты спят.")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got, err := f.notes.Get(ctx, n.ID()); err != nil || got.Title() != "Про котов" {
		t.Fatalf("Get = %v, %v", got, err)
	}
	if _, err = f.notes.Get(ctx, uuid.New()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("Get of no note: %v", err)
	}

	if _, err = f.notes.Change(ctx, bob, n.ID(), "Чужое", "Чужое."); !errors.Is(err, post.ErrCannotChangeNote) {
		t.Fatalf("a stranger changes: %v", err)
	}
	if _, err = f.notes.Change(ctx, ann, uuid.New(), "Нет", "Нет."); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("change of no note: %v", err)
	}

	edited, err := f.notes.Change(ctx, ann, n.ID(), "Про собак", "Собаки лают.")
	if err != nil || edited.EditedAt().IsZero() {
		t.Fatalf("Change = %v, %v", edited, err)
	}
	if got, _ := f.notes.Get(ctx, n.ID()); got.Title() != "Про собак" || got.Text() != "Собаки лают." {
		t.Fatalf("the stored note = %+v", got)
	}

	if err = f.notes.Delete(ctx, bob, n.ID()); !errors.Is(err, post.ErrCannotChangeNote) {
		t.Fatalf("a stranger deletes: %v", err)
	}
	if err = f.notes.Delete(ctx, ann, n.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err = f.notes.Get(ctx, n.ID()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("the deleted note: %v", err)
	}
	if len(f.notifier.forgotten) != 1 || f.notifier.forgotten[0] != n.ID() || f.notifier.outsideTx != 0 {
		t.Fatalf("forgotten %v, outside a transaction %d", f.notifier.forgotten, f.notifier.outsideTx)
	}
	if err = f.notes.Delete(ctx, ann, n.ID()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestNoteService_List(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newNoteFixture()
	ann, bob := uuid.New(), uuid.New()

	var ids []uuid.UUID
	for i := range 3 {
		n, err := f.notes.Create(ctx, ann, post.NoteTitle(fmt.Sprintf("Заметка %d", i)), "Текст.")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, n.ID())
	}
	if _, err := f.notes.Create(ctx, bob, "Чужая", "Текст."); err != nil {
		t.Fatal(err)
	}

	first, err := f.notes.List(ctx, ann, uuid.Nil, 2)
	if err != nil || len(first.Notes) != 2 || first.Total != 3 || first.NextCursor == uuid.Nil {
		t.Fatalf("the first page = %+v, %v", first, err)
	}
	if first.Notes[0].ID() != ids[2] || first.Notes[1].ID() != ids[1] {
		t.Fatal("the notes are not newest first")
	}

	second, err := f.notes.List(ctx, ann, first.NextCursor, 2)
	if err != nil || len(second.Notes) != 1 || second.Notes[0].ID() != ids[0] || second.NextCursor != uuid.Nil {
		t.Fatalf("the second page = %+v, %v", second, err)
	}

	if none, err := f.notes.List(ctx, uuid.New(), uuid.Nil, 0); err != nil || len(none.Notes) != 0 || none.Total != 0 {
		t.Fatalf("the notes of nobody = %+v, %v", none, err)
	}
}

func TestNoteService_Comments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newNoteFixture()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	n, err := f.notes.Create(ctx, ann, "Про котов", "Коты спят.")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = f.notes.Comment(ctx, bob, uuid.New(), "Привет"); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("a comment under no note: %v", err)
	}
	if _, err = f.notes.Comment(ctx, bob, n.ID(), ""); !errors.Is(err, post.ErrCommentEmpty) {
		t.Fatalf("an empty comment: %v", err)
	}

	own, err := f.notes.Comment(ctx, ann, n.ID(), "Сама себе")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.notifier.notified) != 0 {
		t.Fatalf("the author hears of their own comment: %v", f.notifier.notified)
	}

	bobs, err := f.notes.Comment(ctx, bob, n.ID(), "И мурчат")
	if err != nil {
		t.Fatal(err)
	}
	want := service.Event{Kind: service.EventNoteComment, RecipientID: ann, ActorID: bob, PostID: n.ID(), CommentID: bobs.ID()}
	if len(f.notifier.notified) != 1 || f.notifier.notified[0] != want || f.notifier.outsideTx != 0 {
		t.Fatalf("notified %+v, outside a transaction %d", f.notifier.notified, f.notifier.outsideTx)
	}

	cats, err := f.notes.Comment(ctx, cat, n.ID(), "Мяу")
	if err != nil {
		t.Fatal(err)
	}

	page, err := f.notes.Comments(ctx, n.ID(), uuid.Nil, 2)
	if err != nil || len(page.Comments) != 2 || page.Total != 3 || page.Comments[0].ID() != own.ID() || page.NextCursor != bobs.ID() {
		t.Fatalf("the first page = %+v, %v", page, err)
	}
	rest, err := f.notes.Comments(ctx, n.ID(), page.NextCursor, 2)
	if err != nil || len(rest.Comments) != 1 || rest.Comments[0].ID() != cats.ID() || rest.NextCursor != uuid.Nil {
		t.Fatalf("the second page = %+v, %v", rest, err)
	}
	if _, err = f.notes.Comments(ctx, uuid.New(), uuid.Nil, 2); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("the comments of no note: %v", err)
	}

	if got, err := f.notes.CommentByID(ctx, bobs.ID()); err != nil || got.Text() != "И мурчат" {
		t.Fatalf("CommentByID = %v, %v", got, err)
	}
	if many, err := f.notes.CommentsByID(ctx, []uuid.UUID{bobs.ID(), uuid.New(), cats.ID()}); err != nil || len(many) != 2 {
		t.Fatalf("CommentsByID = %v, %v", many, err)
	}
	if _, err = f.notes.CommentsByID(ctx, make([]uuid.UUID, service.MaxPageSize+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("too many IDs: %v", err)
	}

	if err = f.notes.DeleteComment(ctx, cat, bobs.ID()); !errors.Is(err, post.ErrCannotDeleteNoteComment) {
		t.Fatalf("a stranger deletes a comment: %v", err)
	}
	// Its author, and the author of the note.
	if err = f.notes.DeleteComment(ctx, bob, bobs.ID()); err != nil {
		t.Fatal(err)
	}
	if err = f.notes.DeleteComment(ctx, ann, cats.ID()); err != nil {
		t.Fatal(err)
	}
	if err = f.notes.DeleteComment(ctx, ann, cats.ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("delete a comment twice: %v", err)
	}
	if len(f.notifier.forgottenComments) != 2 || f.notifier.outsideTx != 0 {
		t.Fatalf("forgotten comments %v", f.notifier.forgottenComments)
	}

	if got, _ := f.notes.Get(ctx, n.ID()); got.Comments() != 1 {
		t.Fatalf("the note counts %d comments, want 1", got.Comments())
	}
}

// Erasing a page takes its notes with the comments under them and the
// comments it left under the notes of others, which count one less.
func TestPurgeService_Notes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newNoteFixture()
	ann, bob := uuid.New(), uuid.New()

	annsNote, err := f.notes.Create(ctx, ann, "Анна", "Текст.")
	if err != nil {
		t.Fatal(err)
	}
	bobsNote, err := f.notes.Create(ctx, bob, "Пётр", "Текст.")
	if err != nil {
		t.Fatal(err)
	}

	underAnns, err := f.notes.Comment(ctx, bob, annsNote.ID(), "от Петра")
	if err != nil {
		t.Fatal(err)
	}
	stays, err := f.notes.Comment(ctx, ann, annsNote.ID(), "от Анны")
	if err != nil {
		t.Fatal(err)
	}
	underBobs, err := f.notes.Comment(ctx, ann, bobsNote.ID(), "Анна у Петра")
	if err != nil {
		t.Fatal(err)
	}

	if err = f.purge.PurgeUser(ctx, bob); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}

	if _, err = f.notes.Get(ctx, bobsNote.ID()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("the note of the erased: %v", err)
	}
	for _, id := range []uuid.UUID{underAnns.ID(), underBobs.ID()} {
		if _, err = f.notes.CommentByID(ctx, id); !errors.Is(err, post.ErrCommentNotFound) {
			t.Fatalf("a comment that had to go: %v", err)
		}
	}

	left, err := f.notes.Get(ctx, annsNote.ID())
	if err != nil || left.Comments() != 1 {
		t.Fatalf("the note that stays = %+v, %v", left, err)
	}
	if _, err = f.notes.CommentByID(ctx, stays.ID()); err != nil {
		t.Fatalf("the comment that stays: %v", err)
	}
}
