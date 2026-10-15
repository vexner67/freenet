package inmem

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// storyView is one view of a story.
type storyView struct {
	viewerID uuid.UUID
	at       time.Time
}

// StoryRepository keeps the stories in the Store, where it learns whom
// each viewer reads, whose page is deleted and which walls are closed.
type StoryRepository struct {
	s *Store
}

func (s *Store) Stories() *StoryRepository {
	return &StoryRepository{s: s}
}

// StoryViewRepository keeps who viewed the stories of the Store.
type StoryViewRepository struct {
	s *Store
}

func (s *Store) StoryViews() *StoryViewRepository {
	return &StoryViewRepository{s: s}
}

func (r *StoryRepository) Save(ctx context.Context, story *post.Story) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	live := 0
	for _, s := range r.s.stories {
		if s.AuthorID() == story.AuthorID() && s.Live(story.CreatedAt()) {
			live++
		}
	}

	if live >= post.MaxStories {
		return post.ErrTooManyStories
	}

	if r.s.stories == nil {
		r.s.stories = make(map[uuid.UUID]*post.Story)
	}

	r.s.stories[story.ID()] = story

	return nil
}

func (r *StoryRepository) Get(ctx context.Context, id uuid.UUID) (*post.Story, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	story, ok := r.s.stories[id]
	if !ok {
		return nil, post.ErrStoryNotFound
	}

	return story, nil
}

func (r *StoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.stories[id]; !ok {
		return post.ErrStoryNotFound
	}

	delete(r.s.stories, id)
	delete(r.s.storyViews, id)

	return nil
}

func (r *StoryRepository) Shown(ctx context.Context, authorID uuid.UUID, audience service.StoryAudience, now time.Time) ([]*post.Story, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return r.matching(func(s *post.Story) bool {
		if !r.shown(s, audience.Viewer, now) || slices.Contains(audience.Hidden, s.AuthorID()) {
			return false
		}

		if authorID != uuid.Nil {
			return s.AuthorID() == authorID
		}

		if s.AuthorID() == audience.Viewer {
			return true
		}

		club, read := r.s.sources[audience.Viewer][s.AuthorID()]

		return read && !club && !slices.Contains(audience.Ignored, s.AuthorID())
	}), nil
}

func (r *StoryRepository) Seen(ctx context.Context, id, viewerID uuid.UUID, now time.Time) (*post.Story, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	story, ok := r.s.stories[id]
	if !ok || !r.shown(story, viewerID, now) {
		return nil, post.ErrStoryNotFound
	}

	return story, nil
}

func (r *StoryRepository) Expired(ctx context.Context, now time.Time, limit int) ([]*post.Story, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	expired := r.matching(func(s *post.Story) bool { return !s.Live(now) })

	return expired[:min(limit, len(expired))], nil
}

// DeleteByUser deletes the stories the user told and their views of
// others'.
func (r *StoryRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for id, s := range r.s.stories {
		if s.AuthorID() == userID {
			delete(r.s.stories, id)
			delete(r.s.storyViews, id)
		}
	}

	for id, views := range r.s.storyViews {
		r.s.storyViews[id] = slices.DeleteFunc(views, func(v storyView) bool { return v.viewerID == userID })
	}

	return nil
}

// shown tells whether the viewer sees a story at now. The caller holds the
// lock.
func (r *StoryRepository) shown(s *post.Story, viewerID uuid.UUID, now time.Time) bool {
	return s.Live(now) && !r.s.hidden[s.AuthorID()] && !r.s.wallClosedTo(s.AuthorID(), viewerID)
}

// matching returns the stories that pick picks, the oldest first; the
// caller holds the lock.
func (r *StoryRepository) matching(pick func(*post.Story) bool) []*post.Story {
	var found []*post.Story
	for _, s := range r.s.stories {
		if pick(s) {
			found = append(found, s)
		}
	}

	slices.SortFunc(found, func(a, b *post.Story) int { return compareIDs(a.ID(), b.ID()) })

	return found
}

func (r *StoryViewRepository) View(ctx context.Context, storyID, viewerID uuid.UUID, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.stories[storyID]; !ok {
		return post.ErrStoryNotFound
	}

	views := r.s.storyViews[storyID]
	if slices.ContainsFunc(views, func(v storyView) bool { return v.viewerID == viewerID }) {
		return nil
	}

	if r.s.storyViews == nil {
		r.s.storyViews = make(map[uuid.UUID][]storyView)
	}

	r.s.storyViews[storyID] = append(views, storyView{viewerID: viewerID, at: at.UTC().Truncate(time.Microsecond)})

	return nil
}

func (r *StoryViewRepository) Viewed(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	viewed := make(map[uuid.UUID]bool)
	for _, id := range ids {
		if slices.ContainsFunc(r.s.storyViews[id], func(v storyView) bool { return v.viewerID == viewerID }) {
			viewed[id] = true
		}
	}

	return viewed, nil
}

func (r *StoryViewRepository) Viewers(ctx context.Context, storyID uuid.UUID, after *service.ViewCursor, limit int) ([]service.StoryView, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	views := r.counted(storyID)
	slices.SortFunc(views, func(a, b service.StoryView) int {
		if c := b.ViewedAt.Compare(a.ViewedAt); c != 0 {
			return c
		}

		return compareIDs(b.ViewerID, a.ViewerID)
	})

	if after != nil {
		views = slices.DeleteFunc(views, func(v service.StoryView) bool {
			c := v.ViewedAt.Compare(after.ViewedAt)

			return c > 0 || (c == 0 && compareIDs(v.ViewerID, after.ViewerID) >= 0)
		})
	}

	return views[:min(limit, len(views))], nil
}

func (r *StoryViewRepository) Count(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	counts := make(map[uuid.UUID]int)
	for _, id := range ids {
		if n := len(r.counted(id)); n > 0 {
			counts[id] = n
		}
	}

	return counts, nil
}

// counted are the views of a story by others than its author whose page is
// not deleted. The caller holds the lock.
func (r *StoryViewRepository) counted(storyID uuid.UUID) []service.StoryView {
	story, ok := r.s.stories[storyID]
	if !ok {
		return nil
	}

	var views []service.StoryView
	for _, v := range r.s.storyViews[storyID] {
		if story.CountsView(v.viewerID) && !r.s.hidden[v.viewerID] {
			views = append(views, service.StoryView{ViewerID: v.viewerID, ViewedAt: v.at})
		}
	}

	return views
}
