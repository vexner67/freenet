package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// MaxExpired is how many stories one round of StoryService.DeleteExpired
// deletes; the rest wait for the next.
const MaxExpired = 100

// StoryRepository keeps the stories people tell.
type StoryRepository interface {
	// Save returns post.ErrTooManyStories when post.MaxStories stories of
	// the author are live already.
	Save(ctx context.Context, s *post.Story) error
	// Get and Delete return post.ErrStoryNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Story, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// Shown returns the live stories that audience.Viewer sees, the
	// oldest first: of the author, or, for uuid.Nil, of the viewer and of
	// the people they follow. A page deleted, a wall closed to the viewer,
	// the people hidden and ignored are left out.
	Shown(ctx context.Context, authorID uuid.UUID, audience StoryAudience, now time.Time) ([]*post.Story, error)
	// Seen returns a live story the viewer sees; post.ErrStoryNotFound.
	Seen(ctx context.Context, id, viewerID uuid.UUID, now time.Time) (*post.Story, error)
	// Expired returns up to limit stories gone by now, the oldest first.
	Expired(ctx context.Context, now time.Time, limit int) ([]*post.Story, error)
}

// StoryViewRepository keeps who viewed which story.
type StoryViewRepository interface {
	// View tells that viewerID viewed the story at; once is enough.
	View(ctx context.Context, storyID, viewerID uuid.UUID, at time.Time) error
	// Viewed tells which of the stories viewerID viewed.
	Viewed(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error)
	// Viewers returns up to limit views of a story, the latest first,
	// after the cursor (nil: from the latest); those of deleted pages are
	// left out.
	Viewers(ctx context.Context, storyID uuid.UUID, after *ViewCursor, limit int) ([]StoryView, error)
	// Count counts the views of the stories, but those of deleted pages.
	Count(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error)
}

// StoryLikeRepository keeps who liked which story.
type StoryLikeRepository interface {
	// Like tells whether the like is new; post.ErrStoryNotFound when the
	// story is gone.
	Like(ctx context.Context, storyID, userID uuid.UUID, at time.Time) (bool, error)
	// Unlike tells whether there was a like to take.
	Unlike(ctx context.Context, storyID, userID uuid.UUID) (bool, error)
	// Liked tells which of the stories userID liked.
	Liked(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error)
	// Count counts the likes of the stories, but those of deleted pages.
	Count(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error)
}

// StoryCommentRepository keeps the comments under stories.
type StoryCommentRepository interface {
	// Save returns post.ErrStoryNotFound when the story is gone.
	Save(ctx context.Context, c *post.StoryComment) error
	// Get and Delete return post.ErrStoryCommentNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.StoryComment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit comments of a story after the ID (uuid.Nil:
	// from the first), the oldest first; those of deleted pages are left
	// out.
	List(ctx context.Context, storyID, after uuid.UUID, limit int) ([]*post.StoryComment, error)
	// Count counts the comments of the stories, but those of deleted pages.
	Count(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error)
}

// StoryAudience is who looks at stories: Viewer, without the stories of
// the people Hidden (the blacklist) and Ignored (the news leave them out).
type StoryAudience struct {
	Viewer  uuid.UUID
	Hidden  []uuid.UUID
	Ignored []uuid.UUID
}

// StoryView is one viewer of a story.
type StoryView struct {
	ViewerID uuid.UUID
	ViewedAt time.Time
}

// ViewCursor is where a page of viewers ends.
type ViewCursor struct {
	ViewedAt time.Time
	ViewerID uuid.UUID
}

// StoryPage is stories shown to a viewer: which they viewed and liked,
// how many liked and commented each and, of their own, how many viewed
// them.
type StoryPage struct {
	Stories  []*post.Story
	Viewed   map[uuid.UUID]bool
	Views    map[uuid.UUID]int
	Liked    map[uuid.UUID]bool
	Likes    map[uuid.UUID]int
	Comments map[uuid.UUID]int
}

// StoryCommentPage is a page of the comments of a story; Total counts them
// all, Next is where the next page starts (uuid.Nil: none).
type StoryCommentPage struct {
	Comments []*post.StoryComment
	Next     uuid.UUID
	Total    int
}

// ViewerPage is a page of the viewers of a story; Total counts them all.
type ViewerPage struct {
	Views []StoryView
	Next  *ViewCursor
	Total int
}

// StoryService keeps the stories, VK's "истории": a photo shown for a
// day to those who read its author.
type StoryService struct {
	stories  StoryRepository
	views    StoryViewRepository
	likes    StoryLikeRepository
	comments StoryCommentRepository
	tx       TxManager
	photos   PhotoKeeper
	notifier Notifier
	now      func() time.Time
}

// StoryDeps are what StoryService works with.
type StoryDeps struct {
	Stories  StoryRepository
	Views    StoryViewRepository
	Likes    StoryLikeRepository
	Comments StoryCommentRepository
	Tx       TxManager
	Photos   PhotoKeeper
	// Notifier tells the author of a story of its likes and comments.
	Notifier Notifier
	// Now is the clock; nil: time.Now.
	Now func() time.Time
}

func NewStoryService(d StoryDeps) *StoryService {
	now := d.Now
	if now == nil {
		now = time.Now
	}

	return &StoryService{
		stories: d.Stories, views: d.Views, likes: d.Likes, comments: d.Comments,
		tx: d.Tx, photos: d.Photos, notifier: d.Notifier, now: now,
	}
}

// Tell tells a story of authorID with a photo the caller found theirs; the
// photo is kept for good at once, as the story is told.
func (s *StoryService) Tell(ctx context.Context, authorID, photoID uuid.UUID, text post.StoryText) (*post.Story, error) {
	story, err := post.NewStory(authorID, photoID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewStory", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.stories.Save(ctx, story); err != nil {
			return errs.Wrap("s.stories.Save", err)
		}

		return errs.Wrap("s.photos.Publish", s.photos.Publish(ctx, authorID, []uuid.UUID{photoID}))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return story, nil
}

// Feed is the stories above the news of audience.Viewer: theirs first,
// then of the people they follow, those with a story not viewed yet
// first, each person's latest story first among them; each person's
// stories are together, the oldest first, as they are watched.
func (s *StoryService) Feed(ctx context.Context, audience StoryAudience) (StoryPage, error) {
	if len(audience.Hidden) > MaxFeedHidden || len(audience.Ignored) > MaxFeedIgnored {
		return StoryPage{}, ErrTooManyAuthors
	}

	stories, err := s.stories.Shown(ctx, uuid.Nil, audience, s.now())
	if err != nil {
		return StoryPage{}, errs.Wrap("s.stories.Shown", err)
	}

	page, err := s.page(ctx, audience.Viewer, stories)
	if err != nil {
		return StoryPage{}, errs.Wrap("s.page", err)
	}

	page.Stories = feedOrder(page.Stories, audience.Viewer, page.Viewed)

	return page, nil
}

// feedOrder groups the stories by their authors, in the order of Feed.
func feedOrder(stories []*post.Story, viewerID uuid.UUID, viewed map[uuid.UUID]bool) []*post.Story {
	type group struct {
		stories []*post.Story
		fresh   bool
	}

	groups := map[uuid.UUID]*group{}
	var authors []uuid.UUID
	for _, story := range stories {
		g, ok := groups[story.AuthorID()]
		if !ok {
			g = &group{}
			groups[story.AuthorID()] = g
			authors = append(authors, story.AuthorID())
		}

		g.stories = append(g.stories, story)
		g.fresh = g.fresh || !viewed[story.ID()]
	}

	// The latest story of each: its last, as they come the oldest first.
	latest := func(id uuid.UUID) time.Time { return groups[id].stories[len(groups[id].stories)-1].CreatedAt() }

	slices.SortStableFunc(authors, func(a, b uuid.UUID) int {
		switch {
		case (a == viewerID) != (b == viewerID):
			return boolOrder(a == viewerID)
		case groups[a].fresh != groups[b].fresh:
			return boolOrder(groups[a].fresh)
		default:
			return latest(b).Compare(latest(a))
		}
	})

	ordered := make([]*post.Story, 0, len(stories))
	for _, id := range authors {
		ordered = append(ordered, groups[id].stories...)
	}

	return ordered
}

// boolOrder puts what is true first.
func boolOrder(first bool) int {
	if first {
		return -1
	}

	return 1
}

// User is the live stories of authorID that the viewer sees, the oldest
// first.
func (s *StoryService) User(ctx context.Context, audience StoryAudience, authorID uuid.UUID) (StoryPage, error) {
	if len(audience.Hidden) > MaxFeedHidden {
		return StoryPage{}, ErrTooManyAuthors
	}

	stories, err := s.stories.Shown(ctx, authorID, StoryAudience{Viewer: audience.Viewer, Hidden: audience.Hidden}, s.now())
	if err != nil {
		return StoryPage{}, errs.Wrap("s.stories.Shown", err)
	}

	page, err := s.page(ctx, audience.Viewer, stories)
	if err != nil {
		return StoryPage{}, errs.Wrap("s.page", err)
	}

	return page, nil
}

// page tells which of the stories the viewer viewed and counts the views
// of their own.
func (s *StoryService) page(ctx context.Context, viewerID uuid.UUID, stories []*post.Story) (StoryPage, error) {
	page := StoryPage{Stories: stories}
	if len(stories) == 0 {
		return page, nil
	}

	var all, own []uuid.UUID
	for _, story := range stories {
		all = append(all, story.ID())
		if story.Mine(viewerID) {
			own = append(own, story.ID())
		}
	}

	var err error
	if page.Viewed, err = s.views.Viewed(ctx, viewerID, all); err != nil {
		return StoryPage{}, errs.Wrap("s.views.Viewed", err)
	}

	if page.Liked, err = s.likes.Liked(ctx, viewerID, all); err != nil {
		return StoryPage{}, errs.Wrap("s.likes.Liked", err)
	}

	if page.Likes, err = s.likes.Count(ctx, all); err != nil {
		return StoryPage{}, errs.Wrap("s.likes.Count", err)
	}

	if page.Comments, err = s.comments.Count(ctx, all); err != nil {
		return StoryPage{}, errs.Wrap("s.comments.Count", err)
	}

	if len(own) > 0 {
		if page.Views, err = s.views.Count(ctx, own); err != nil {
			return StoryPage{}, errs.Wrap("s.views.Count", err)
		}
	}

	return page, nil
}

// View tells that viewerID viewed a story they see; their own is not
// counted, but it is viewed too.
func (s *StoryService) View(ctx context.Context, viewerID, id uuid.UUID) error {
	now := s.now()

	story, err := s.stories.Seen(ctx, id, viewerID, now)
	if err != nil {
		return errs.Wrap("s.stories.Seen", err)
	}

	return errs.Wrap("s.views.View", s.views.View(ctx, story.ID(), viewerID, now))
}

// Viewers pages who viewed a story, the latest first, for its author
// alone; the author is not among them.
func (s *StoryService) Viewers(ctx context.Context, requesterID, id uuid.UUID, after *ViewCursor, size int) (ViewerPage, error) {
	story, err := s.stories.Get(ctx, id)
	if err != nil {
		return ViewerPage{}, errs.Wrap("s.stories.Get", err)
	}

	if err = story.CheckViewers(requesterID); err != nil {
		return ViewerPage{}, errs.Wrap("story.CheckViewers", err)
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	views, err := s.views.Viewers(ctx, id, after, size+1)
	if err != nil {
		return ViewerPage{}, errs.Wrap("s.views.Viewers", err)
	}

	counts, err := s.views.Count(ctx, []uuid.UUID{id})
	if err != nil {
		return ViewerPage{}, errs.Wrap("s.views.Count", err)
	}

	page := ViewerPage{Views: views, Total: counts[id]}
	if len(views) > size {
		page.Views = views[:size]
		last := views[size-1]
		page.Next = &ViewCursor{ViewedAt: last.ViewedAt, ViewerID: last.ViewerID}
	}

	return page, nil
}

// Delete deletes a story with its photo, for its author.
func (s *StoryService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	story, err := s.stories.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.stories.Get", err)
	}

	if err = story.CheckDelete(requesterID); err != nil {
		return errs.Wrap("story.CheckDelete", err)
	}

	return errs.Wrap("s.drop", s.drop(ctx, story))
}

// DeleteExpired deletes up to MaxExpired stories whose day is over, with
// their photos, and tells how many. One that fails waits for the next
// round without holding the others.
func (s *StoryService) DeleteExpired(ctx context.Context) (int, error) {
	expired, err := s.stories.Expired(ctx, s.now(), MaxExpired)
	if err != nil {
		return 0, errs.Wrap("s.stories.Expired", err)
	}

	var (
		deleted int
		failed  []error
	)
	for _, story := range expired {
		err := s.drop(ctx, story)
		switch {
		case err == nil:
			deleted++
		case !errors.Is(err, post.ErrStoryNotFound):
			failed = append(failed, err)
		}
	}

	return deleted, errs.Wrap("s.drop", errors.Join(failed...))
}

// drop deletes a story and, in the same transaction, its photo and the
// notifications of its likes and comments, which go with it.
func (s *StoryService) drop(ctx context.Context, story *post.Story) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.stories.Delete(ctx, story.ID()); err != nil {
			return errs.Wrap("s.stories.Delete", err)
		}

		if err := s.notifier.ForgetPost(ctx, story.ID()); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, story.AuthorID(), []uuid.UUID{story.PhotoID()}))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Get is a live story viewerID sees, with what the page tells of it.
func (s *StoryService) Get(ctx context.Context, viewerID, id uuid.UUID) (StoryPage, error) {
	story, err := s.stories.Seen(ctx, id, viewerID, s.now())
	if err != nil {
		return StoryPage{}, errs.Wrap("s.stories.Seen", err)
	}

	return s.page(ctx, viewerID, []*post.Story{story})
}

// Like likes a story userID sees; its author hears of it, but of their
// own. Liking again changes nothing.
func (s *StoryService) Like(ctx context.Context, userID, id uuid.UUID) error {
	now := s.now()

	story, err := s.stories.Seen(ctx, id, userID, now)
	if err != nil {
		return errs.Wrap("s.stories.Seen", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		added, err := s.likes.Like(ctx, story.ID(), userID, now)
		if err != nil || !added || !story.Notifies(userID) {
			return errs.Wrap("s.likes.Like", err)
		}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, storyLiked(story, userID)))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Unlike takes userID's like of a story and its notification back.
func (s *StoryService) Unlike(ctx context.Context, userID, id uuid.UUID) error {
	story, err := s.stories.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.stories.Get", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		removed, err := s.likes.Unlike(ctx, story.ID(), userID)
		if err != nil || !removed || !story.Notifies(userID) {
			return errs.Wrap("s.likes.Unlike", err)
		}

		return errs.Wrap("s.notifier.Retract", s.notifier.Retract(ctx, storyLiked(story, userID)))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

func storyLiked(story *post.Story, userID uuid.UUID) Event {
	return Event{Kind: EventStoryLike, RecipientID: story.AuthorID(), ActorID: userID, PostID: story.ID()}
}

// Comment comments a story authorID sees; its author hears of it, but of
// their own.
func (s *StoryService) Comment(ctx context.Context, authorID, id uuid.UUID, text post.CommentText) (*post.StoryComment, error) {
	now := s.now()

	story, err := s.stories.Seen(ctx, id, authorID, now)
	if err != nil {
		return nil, errs.Wrap("s.stories.Seen", err)
	}

	c, err := post.NewStoryComment(story.ID(), authorID, text, now)
	if err != nil {
		return nil, errs.Wrap("post.NewStoryComment", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.comments.Save(ctx, c); err != nil {
			return errs.Wrap("s.comments.Save", err)
		}

		if !story.Notifies(authorID) {
			return nil
		}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, Event{
			Kind: EventStoryComment, RecipientID: story.AuthorID(), ActorID: authorID, PostID: story.ID(), CommentID: c.ID(),
		}))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return c, nil
}

// Comments pages the comments of a story viewerID sees, the oldest first.
func (s *StoryService) Comments(ctx context.Context, viewerID, id, after uuid.UUID, size int) (StoryCommentPage, error) {
	story, err := s.stories.Seen(ctx, id, viewerID, s.now())
	if err != nil {
		return StoryCommentPage{}, errs.Wrap("s.stories.Seen", err)
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	comments, err := s.comments.List(ctx, story.ID(), after, size+1)
	if err != nil {
		return StoryCommentPage{}, errs.Wrap("s.comments.List", err)
	}

	counts, err := s.comments.Count(ctx, []uuid.UUID{story.ID()})
	if err != nil {
		return StoryCommentPage{}, errs.Wrap("s.comments.Count", err)
	}

	page := StoryCommentPage{Comments: comments, Total: counts[story.ID()]}
	if len(comments) > size {
		page.Comments = comments[:size]
		page.Next = comments[size-1].ID()
	}

	return page, nil
}

// DeleteComment deletes a comment of a story, for its author or the
// author of the story, with its notification.
func (s *StoryService) DeleteComment(ctx context.Context, requesterID, id uuid.UUID) error {
	c, err := s.comments.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.comments.Get", err)
	}

	story, err := s.stories.Get(ctx, c.StoryID())
	if errors.Is(err, post.ErrStoryNotFound) {
		return post.ErrStoryCommentNotFound
	}
	if err != nil {
		return errs.Wrap("s.stories.Get", err)
	}

	if err = c.CheckDelete(requesterID, story); err != nil {
		return errs.Wrap("c.CheckDelete", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.comments.Delete(ctx, c.ID()); err != nil {
			return errs.Wrap("s.comments.Delete", err)
		}

		return errs.Wrap("s.notifier.ForgetComment", s.notifier.ForgetComment(ctx, c.ID()))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}
