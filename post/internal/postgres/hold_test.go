package postgres_test

import (
	"testing"

	"github.com/vexner67/freenet/post/internal/holdtest"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestHoldRepository(t *testing.T) {
	pool := newPool(t)
	topics := postgres.NewTopicRepository(pool)
	albums := postgres.NewAlbumRepository(pool)

	holdtest.Run(t, holdtest.Storage{
		Posts:         postgres.NewPostRepository(pool),
		Likes:         postgres.NewLikeRepository(pool),
		Comments:      postgres.NewCommentRepository(pool),
		CommentLikes:  postgres.NewCommentLikeRepository(pool),
		Polls:         postgres.NewPollRepository(pool),
		Topics:        topics,
		Messages:      topics.Messages(),
		Albums:        albums,
		Photos:        albums.Photos(),
		PhotoLikes:    albums.PhotoLikes(),
		PhotoComments: albums.PhotoComments(),
		Holders:       []service.ReactionHolder{postgres.NewHoldRepository(pool)},
	})
}

func TestHoldRepository_Written(t *testing.T) {
	pool := newPool(t)
	topics := postgres.NewTopicRepository(pool)
	albums := postgres.NewAlbumRepository(pool)
	notes := postgres.NewNoteRepository(pool)

	held := postgres.NewHoldRepository(pool)

	holdtest.RunWritten(t, holdtest.Written{
		Posts:         postgres.NewPostRepository(pool),
		Likes:         postgres.NewLikeRepository(pool),
		Comments:      postgres.NewCommentRepository(pool),
		CommentLikes:  postgres.NewCommentLikeRepository(pool),
		Topics:        topics,
		Messages:      topics.Messages(),
		Albums:        albums,
		Photos:        albums.Photos(),
		PhotoLikes:    albums.PhotoLikes(),
		PhotoComments: albums.PhotoComments(),
		Notes:         notes,
		NoteComments:  notes.Comments(),
		Holders:       []service.ReactionHolder{held},
		Pages:         []service.Pages{held},
	})
}
