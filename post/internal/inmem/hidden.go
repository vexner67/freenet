package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// hiddenUsers are the pages their owners deleted and may still restore:
// what they wrote stays in its store, and the lists and the counts leave
// it out, as hidden_users makes them in PostgreSQL.
type hiddenUsers map[uuid.UUID]bool

// writes: whether nobody sees what the author wrote; what speaks for a
// wall is the wall's.
func (h hiddenUsers) writes(authorID uuid.UUID, fromWall bool) bool {
	return h[authorID] && !fromWall
}

// postHidden: the owner of the wall deleted the page, or the author did
// and the post does not speak for the wall. The caller holds the lock.
func (s *Store) postHidden(p *post.Post) bool {
	return s.hidden[p.WallID()] || s.hidden.writes(p.AuthorID(), p.FromWall())
}

func (s *Store) commentHidden(c *post.Comment) bool {
	return s.hidden.writes(c.AuthorID(), c.FromWall())
}

// seen is the post with the counters its readers see: without the
// comments of deleted pages and the reposts "who shared it" leaves out,
// the archived ones and those nobody sees. The caller holds the lock.
func (s *Store) seen(p *post.Post) *post.Post {
	comments, reposts := 0, 0
	for _, c := range s.comments {
		if c.PostID() == p.ID() && s.commentHidden(c) {
			comments++
		}
	}

	for _, r := range s.posts {
		if r.Content().RepostOf() == p.ID() && (r.Archived() || s.postHidden(r)) {
			reposts++
		}
	}

	if comments == 0 && reposts == 0 {
		return p
	}

	stored := p.Stored()
	stored.Counters.Comments -= comments
	stored.Counters.Reposts -= reposts

	return post.Restore(stored)
}

// seenTopic is the topic without the messages of deleted pages in its
// count. The caller holds the lock.
func (r *TopicRepository) seenTopic(s post.StoredTopic) *post.Topic {
	for _, m := range r.messages {
		if m.TopicID() == s.ID && r.hidden.writes(m.AuthorID(), m.FromWall()) {
			s.Messages--
		}
	}

	return post.RestoreTopic(s)
}

// seenAlbum is the album without the photos of deleted pages in its
// count. The caller holds the lock.
func (r *AlbumRepository) seenAlbum(s post.StoredAlbum) *post.Album {
	for _, p := range r.photos {
		if p.AlbumID() == s.ID && r.hidden[p.AuthorID()] {
			s.Photos--
		}
	}

	return post.RestoreAlbum(s)
}

// seenPhoto is the photo without the comments of deleted pages in its
// count. The caller holds the lock.
func (r *AlbumRepository) seenPhoto(p *post.AlbumPhoto) *post.AlbumPhoto {
	hidden := 0
	for _, c := range r.photoComments {
		if c.PhotoID() == p.ID() && r.hidden[c.AuthorID()] {
			hidden++
		}
	}

	if hidden == 0 {
		return p
	}

	return post.RestoreAlbumPhoto(post.StoredAlbumPhoto{
		ID: p.ID(), AlbumID: p.AlbumID(), AuthorID: p.AuthorID(), Description: p.Description(), CreatedAt: p.CreatedAt(),
		Likes: p.Likes(), Comments: p.Comments() - hidden, NSFW: p.NSFW(),
	})
}

// photoHidden: who put the photo or who owns its album deleted the page.
func (r *AlbumRepository) photoHidden(p *post.AlbumPhoto) bool {
	return r.hidden[p.AuthorID()] || r.hidden[r.albums[p.AlbumID()].OwnerID]
}

// among tells which of the users are hidden, under the lock of the store
// that keeps them.
func (h hiddenUsers) among(ctx context.Context, mu *sync.RWMutex, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	mu.RLock()
	defer mu.RUnlock()

	gone := make(map[uuid.UUID]bool)
	for _, id := range userIDs {
		if h[id] {
			gone[id] = true
		}
	}

	return gone, nil
}

// Deleted tells which of the users deleted their page: each store learns
// of them as it holds their likes.
func (r *HoldRepository) Deleted(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return r.s.hidden.among(ctx, &r.s.mu, userIDs)
}

func (r *TopicRepository) Deleted(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return r.hidden.among(ctx, &r.mu, userIDs)
}

func (r *AlbumRepository) Deleted(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return r.hidden.among(ctx, &r.mu, userIDs)
}
