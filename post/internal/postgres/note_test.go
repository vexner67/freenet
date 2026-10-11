package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestNoteRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	notes := postgres.NewNoteRepository(pool)
	comments := notes.Comments()

	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	write := func(author uuid.UUID, title post.NoteTitle) *post.Note {
		t.Helper()

		n, err := post.NewNote(author, title, "Коты спят.\nИ мурчат 🐈", now)
		if err != nil {
			t.Fatal(err)
		}
		if err = notes.Save(ctx, n); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return n
	}

	first := write(ann, post.NoteTitle(strings.Repeat("я", 256)))
	second := write(ann, "Вторая")
	third := write(ann, "Третья")
	write(bob, "Чужая")

	got, err := notes.Get(ctx, first.ID())
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, first)
	}
	if _, err = notes.Get(ctx, uuid.New()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("Get of no note: %v", err)
	}

	page, err := notes.List(ctx, ann, uuid.Nil, 2)
	if err != nil || len(page) != 2 || page[0].ID() != third.ID() || page[1].ID() != second.ID() {
		t.Fatalf("List = %v, %v", page, err)
	}
	if page, err = notes.List(ctx, ann, second.ID(), 2); err != nil || len(page) != 1 || page[0].ID() != first.ID() {
		t.Fatalf("the next page = %v, %v", page, err)
	}
	if total, err := notes.Count(ctx, ann); err != nil || total != 3 {
		t.Fatalf("Count = %d, %v", total, err)
	}

	edited, err := second.Edited(ann, "Про собак", "Собаки лают.", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = notes.Update(ctx, edited); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, err = notes.Get(ctx, second.ID()); err != nil || !reflect.DeepEqual(got, edited) {
		t.Fatalf("the edited note = %+v, %v; want %+v", got, err, edited)
	}

	gone, err := post.NewNote(ann, "Нет", "Нет.", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = notes.Update(ctx, gone); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("Update of no note: %v", err)
	}
	if err = notes.Delete(ctx, gone.ID()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("Delete of no note: %v", err)
	}

	say := func(noteID, author uuid.UUID, text post.CommentText) *post.NoteComment {
		t.Helper()

		c, err := post.NewNoteComment(noteID, author, text, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = comments.Save(ctx, c); err != nil {
			t.Fatalf("save a comment: %v", err)
		}

		return c
	}

	c1 := say(first.ID(), bob, "первый")
	c2 := say(first.ID(), ann, "второй")
	c3 := say(first.ID(), bob, "третий")

	orphan, err := post.NewNoteComment(uuid.New(), bob, "никуда", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Save(ctx, orphan); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("a comment under no note: %v", err)
	}

	if got, err = notes.Get(ctx, first.ID()); err != nil || got.Comments() != 3 {
		t.Fatalf("the note counts %d comments, %v; want 3", got.Comments(), err)
	}

	if stored, err := comments.Get(ctx, c1.ID()); err != nil || !reflect.DeepEqual(stored, c1) {
		t.Fatalf("Get of a comment = %+v, %v", stored, err)
	}
	if _, err = comments.Get(ctx, uuid.New()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("Get of no comment: %v", err)
	}

	listed, err := comments.List(ctx, first.ID(), uuid.Nil, 2)
	if err != nil || len(listed) != 2 || listed[0].ID() != c1.ID() || listed[1].ID() != c2.ID() {
		t.Fatalf("the comments = %v, %v", listed, err)
	}
	if listed, err = comments.List(ctx, first.ID(), c2.ID(), 2); err != nil || len(listed) != 1 || listed[0].ID() != c3.ID() {
		t.Fatalf("the next comments = %v, %v", listed, err)
	}

	if many, err := comments.GetMany(ctx, []uuid.UUID{c1.ID(), uuid.New(), c3.ID()}); err != nil || len(many) != 2 {
		t.Fatalf("GetMany = %v, %v", many, err)
	}

	if err = comments.Delete(ctx, c2.ID()); err != nil {
		t.Fatalf("delete a comment: %v", err)
	}
	if err = comments.Delete(ctx, c2.ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("delete a comment twice: %v", err)
	}
	if got, err = notes.Get(ctx, first.ID()); err != nil || got.Comments() != 2 {
		t.Fatalf("the note counts %d comments, %v; want 2", got.Comments(), err)
	}

	// A deleted note takes its comments.
	if err = notes.Delete(ctx, first.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err = comments.Get(ctx, c1.ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("a comment of a deleted note: %v", err)
	}
}

// Erasing a user deletes their notes and the comments they left, and
// keeps the counters of the notes that stay right.
func TestNoteRepository_DeleteByUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	notes := postgres.NewNoteRepository(pool)
	comments := notes.Comments()

	ann, bob := uuid.New(), uuid.New()
	now := time.Now()

	write := func(author uuid.UUID) *post.Note {
		t.Helper()

		n, err := post.NewNote(author, "Заметка", "Текст.", now)
		if err != nil {
			t.Fatal(err)
		}
		if err = notes.Save(ctx, n); err != nil {
			t.Fatal(err)
		}

		return n
	}
	say := func(noteID, author uuid.UUID) *post.NoteComment {
		t.Helper()

		c, err := post.NewNoteComment(noteID, author, "слова", now)
		if err != nil {
			t.Fatal(err)
		}
		if err = comments.Save(ctx, c); err != nil {
			t.Fatal(err)
		}

		return c
	}

	annsNote, bobsNote := write(ann), write(bob)
	say(annsNote.ID(), bob)
	say(annsNote.ID(), bob)
	stays := say(annsNote.ID(), ann)
	underBobs := say(bobsNote.ID(), ann)
	say(bobsNote.ID(), bob)

	if err := notes.DeleteByUser(ctx, bob); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}

	if _, err := notes.Get(ctx, bobsNote.ID()); !errors.Is(err, post.ErrNoteNotFound) {
		t.Fatalf("the note of the erased: %v", err)
	}
	if _, err := comments.Get(ctx, underBobs.ID()); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("a comment under the note of the erased: %v", err)
	}

	left, err := notes.Get(ctx, annsNote.ID())
	if err != nil || left.Comments() != 1 {
		t.Fatalf("the note that stays counts %d, %v; want 1", left.Comments(), err)
	}
	if listed, err := comments.List(ctx, annsNote.ID(), uuid.Nil, 10); err != nil || len(listed) != 1 || listed[0].ID() != stays.ID() {
		t.Fatalf("the comments that stay = %v, %v", listed, err)
	}
}
