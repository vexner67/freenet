package inmem_test

import (
	"testing"

	"github.com/vexner67/freenet/post/internal/holdtest"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/service"
)

func TestHolds(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	topics := inmem.NewTopicRepository()
	albums := inmem.NewAlbumRepository()

	holdtest.Run(t, holdtest.Storage{
		Posts:         store.Posts(),
		Likes:         store.Likes(),
		Comments:      store.Comments(),
		CommentLikes:  store.CommentLikes(),
		Polls:         store.Polls(),
		Topics:        topics,
		Messages:      topics.Messages(),
		Albums:        albums,
		Photos:        albums.Photos(),
		PhotoLikes:    albums.PhotoLikes(),
		PhotoComments: albums.PhotoComments(),
		Holders:       []service.ReactionHolder{store.Holds(), topics, albums},
	})
}

func TestHolds_Written(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	topics := inmem.NewTopicRepository()
	albums := inmem.NewAlbumRepository()
	notes := inmem.NewNoteRepository()

	holdtest.RunWritten(t, holdtest.Written{
		Posts:         store.Posts(),
		Likes:         store.Likes(),
		Comments:      store.Comments(),
		CommentLikes:  store.CommentLikes(),
		Topics:        topics,
		Messages:      topics.Messages(),
		Albums:        albums,
		Photos:        albums.Photos(),
		PhotoLikes:    albums.PhotoLikes(),
		PhotoComments: albums.PhotoComments(),
		Notes:         notes,
		NoteComments:  notes.Comments(),
		Holders:       []service.ReactionHolder{store.Holds(), topics, albums, notes},
	})
}
