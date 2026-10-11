package postgres_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func faveIDs(faves []service.Fave) []uuid.UUID {
	ids := make([]uuid.UUID, len(faves))
	for i, f := range faves {
		ids[i] = f.ID
	}

	return ids
}

func TestFaves(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	likes := postgres.NewLikeRepository(pool)
	comments := postgres.NewCommentRepository(pool)
	commentLikes := postgres.NewCommentLikeRepository(pool)
	albums := postgres.NewAlbumRepository(pool)

	ann, bob := uuid.New(), uuid.New()

	write := func(wall, author uuid.UUID, text string) *post.Post {
		t.Helper()

		p, err := post.New(wall, author, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return p
	}

	first, second, third := write(ann, ann, "раз"), write(ann, ann, "два"), write(ann, ann, "три")

	t.Run("posts", func(t *testing.T) {
		for _, p := range []*post.Post{second, third, first} {
			if _, err := likes.Like(ctx, p.ID(), bob); err != nil {
				t.Fatalf("Like: %v", err)
			}
		}
		// Somebody else's like is not Bob's fave.
		if _, err := likes.Like(ctx, first.ID(), ann); err != nil {
			t.Fatal(err)
		}

		page, err := likes.LikedBy(ctx, bob, nil, 2)
		if err != nil || !slices.Equal(faveIDs(page), []uuid.UUID{first.ID(), third.ID()}) {
			t.Fatalf("LikedBy = %v, %v", page, err)
		}

		rest, err := likes.LikedBy(ctx, bob, &service.FaveCursor{LikedAt: page[1].LikedAt, ID: page[1].ID}, 2)
		if err != nil || !slices.Equal(faveIDs(rest), []uuid.UUID{second.ID()}) {
			t.Fatalf("the next page = %v, %v", rest, err)
		}

		if n, err := likes.CountLikedBy(ctx, bob); err != nil || n != 3 {
			t.Fatalf("CountLikedBy = %d, %v", n, err)
		}

		if err = posts.SetArchived(ctx, third.ID(), true); err != nil {
			t.Fatal(err)
		}

		page, err = likes.LikedBy(ctx, bob, nil, 10)
		if err != nil || !slices.Equal(faveIDs(page), []uuid.UUID{first.ID(), second.ID()}) {
			t.Fatalf("without the archived = %v, %v", page, err)
		}
		if n, err := likes.CountLikedBy(ctx, bob); err != nil || n != 2 {
			t.Fatalf("CountLikedBy without the archived = %d, %v", n, err)
		}

		// A table this small is read whole unless told otherwise.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err = tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			t.Fatal(err)
		}

		rows, err := tx.Query(ctx, `EXPLAIN SELECT post_id FROM post_likes WHERE user_id = $1 ORDER BY created_at DESC, post_id DESC LIMIT 20`, bob)
		if err != nil {
			t.Fatal(err)
		}

		var plan []string
		for rows.Next() {
			var line string
			if err = rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, line)
		}
		rows.Close()

		if !strings.Contains(strings.Join(plan, "\n"), "post_likes_faves_idx") {
			t.Fatalf("the faves are not read by their index:\n%s", strings.Join(plan, "\n"))
		}
	})

	t.Run("comments", func(t *testing.T) {
		comment := func(p *post.Post, text string) *post.Comment {
			t.Helper()

			c, err := post.NewComment(p.ID(), ann, post.RestoreCommentContent(post.CommentText(text), nil), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = comments.Save(ctx, c); err != nil {
				t.Fatalf("Save: %v", err)
			}

			return c
		}

		one, two, hidden := comment(first, "раз"), comment(first, "два"), comment(third, "в архиве")
		for _, c := range []*post.Comment{two, hidden, one} {
			if _, err := commentLikes.Like(ctx, c.ID(), bob); err != nil {
				t.Fatalf("Like: %v", err)
			}
		}

		page, err := commentLikes.LikedBy(ctx, bob, nil, 1)
		if err != nil || !slices.Equal(faveIDs(page), []uuid.UUID{one.ID()}) {
			t.Fatalf("LikedBy = %v, %v", page, err)
		}

		rest, err := commentLikes.LikedBy(ctx, bob, &service.FaveCursor{LikedAt: page[0].LikedAt, ID: page[0].ID}, 5)
		if err != nil || !slices.Equal(faveIDs(rest), []uuid.UUID{two.ID()}) {
			t.Fatalf("the next page = %v, %v", rest, err)
		}

		if n, err := commentLikes.CountLikedBy(ctx, bob); err != nil || n != 2 {
			t.Fatalf("CountLikedBy = %d, %v", n, err)
		}
	})

	t.Run("photos", func(t *testing.T) {
		a, err := post.NewAlbum(ann, "Лето", "", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = albums.Save(ctx, a); err != nil {
			t.Fatal(err)
		}

		photos := newAlbumPhotos(t, a, ann, 3, time.Now())
		if err = albums.Photos().Add(ctx, a.ID(), photos, time.Now()); err != nil {
			t.Fatalf("Add: %v", err)
		}

		for _, p := range []*post.AlbumPhoto{photos[1], photos[0]} {
			if _, err = albums.PhotoLikes().Like(ctx, p.ID(), bob); err != nil {
				t.Fatalf("Like: %v", err)
			}
		}

		page, err := albums.PhotoLikes().LikedBy(ctx, bob, nil, 1)
		if err != nil || !slices.Equal(faveIDs(page), []uuid.UUID{photos[0].ID()}) {
			t.Fatalf("LikedBy = %v, %v", page, err)
		}

		rest, err := albums.PhotoLikes().LikedBy(ctx, bob, &service.FaveCursor{LikedAt: page[0].LikedAt, ID: page[0].ID}, 5)
		if err != nil || !slices.Equal(faveIDs(rest), []uuid.UUID{photos[1].ID()}) {
			t.Fatalf("the next page = %v, %v", rest, err)
		}

		if n, err := albums.PhotoLikes().CountLikedBy(ctx, bob); err != nil || n != 2 {
			t.Fatalf("CountLikedBy = %d, %v", n, err)
		}

		many, err := albums.Photos().GetMany(ctx, []uuid.UUID{photos[2].ID(), uuid.New(), photos[0].ID()})
		if err != nil || len(many) != 2 {
			t.Fatalf("GetMany = %v, %v", many, err)
		}
		for _, p := range many {
			if p.AlbumID() != a.ID() || p.AuthorID() != ann {
				t.Fatalf("a photo read back = %+v", p)
			}
		}
	})
}

func TestPostRepository_ByAuthor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	posts := postgres.NewPostRepository(newPool(t))
	ann, bob := uuid.New(), uuid.New()

	write := func(wall, author uuid.UUID, text string) *post.Post {
		t.Helper()

		p, err := post.New(wall, author, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return p
	}

	own := write(ann, ann, "у себя")
	guest := write(bob, ann, "у Боба")
	write(ann, bob, "Боб у Анны")
	last := write(ann, ann, "ещё у себя")

	ids := func(found []*post.Post) []uuid.UUID {
		out := make([]uuid.UUID, len(found))
		for i, p := range found {
			out[i] = p.ID()
		}

		return out
	}

	page, err := posts.ListByAuthor(ctx, ann, uuid.Nil, 2)
	if err != nil || !slices.Equal(ids(page), []uuid.UUID{last.ID(), guest.ID()}) {
		t.Fatalf("ListByAuthor = %v, %v", ids(page), err)
	}
	if page, err = posts.ListByAuthor(ctx, ann, guest.ID(), 2); err != nil || !slices.Equal(ids(page), []uuid.UUID{own.ID()}) {
		t.Fatalf("the next page = %v, %v", ids(page), err)
	}
	if n, err := posts.CountByAuthor(ctx, ann); err != nil || n != 3 {
		t.Fatalf("CountByAuthor = %d, %v", n, err)
	}

	// Archived on her own wall it stays hers, on Bob's it is his alone.
	for _, p := range []*post.Post{own, guest} {
		if err = posts.SetArchived(ctx, p.ID(), true); err != nil {
			t.Fatal(err)
		}
	}

	page, err = posts.ListByAuthor(ctx, ann, uuid.Nil, 10)
	if err != nil || !slices.Equal(ids(page), []uuid.UUID{last.ID(), own.ID()}) {
		t.Fatalf("after the archives = %v, %v", ids(page), err)
	}
	if n, err := posts.CountByAuthor(ctx, ann); err != nil || n != 2 {
		t.Fatalf("CountByAuthor after the archives = %d, %v", n, err)
	}
}
