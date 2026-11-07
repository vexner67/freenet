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

	holdtest.Run(t, holdtest.Storage{
		Posts:        store.Posts(),
		Likes:        store.Likes(),
		Comments:     store.Comments(),
		CommentLikes: store.CommentLikes(),
		Polls:        store.Polls(),
		Topics:       topics,
		Messages:     topics.Messages(),
		Holders:      []service.ReactionHolder{store.Holds(), topics},
	})
}

func TestHolds_Written(t *testing.T) {
	t.Parallel()

	store := inmem.NewStore()
	topics := inmem.NewTopicRepository()

	holdtest.RunWritten(t, holdtest.Written{
		Posts:        store.Posts(),
		Likes:        store.Likes(),
		Comments:     store.Comments(),
		CommentLikes: store.CommentLikes(),
		Topics:       topics,
		Messages:     topics.Messages(),
		Holders:      []service.ReactionHolder{store.Holds(), topics},
		Pages:        []service.Pages{store.Holds(), topics},
	})
}
