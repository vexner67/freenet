package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// MaxDue is how many postponed posts one round of PostponedService.PublishDue
// publishes; the rest wait for the next.
const MaxDue = 100

type PostponedRepository interface {
	// Save returns post.ErrTooManyPostponed when post.MaxPostponed posts
	// wait for the wall already.
	Save(ctx context.Context, p *post.Postponed) error
	// Get, Update and Delete return post.ErrPostponedNotFound; Delete does
	// when the post came out or was deleted meanwhile.
	Get(ctx context.Context, id uuid.UUID) (*post.Postponed, error)
	Update(ctx context.Context, p *post.Postponed) error
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns all the posts that wait for a wall, the soonest first.
	List(ctx context.Context, wallID uuid.UUID) ([]*post.Postponed, error)
	// Due returns up to limit posts whose time has come by now, the
	// soonest first.
	Due(ctx context.Context, now time.Time, limit int) ([]*post.Postponed, error)
}

// PostPoster writes the post a postponed one becomes (PostService.Posted).
type PostPoster interface {
	Posted(ctx context.Context, p *post.Post, mentions post.Mentions, claim func(context.Context) error) error
}

// PostponedService keeps the posts that wait for their time, VK's
// "отложенные записи", and publishes them when it comes.
type PostponedService struct {
	postponed  PostponedRepository
	poster     PostPoster
	tx         TxManager
	photos     PhotoKeeper
	moderators WallModerators
	now        func() time.Time
}

// PostponedDeps are what PostponedService works with.
type PostponedDeps struct {
	Postponed  PostponedRepository
	Poster     PostPoster
	Tx         TxManager
	Photos     PhotoKeeper
	Moderators WallModerators
	// Now is the clock; nil: time.Now.
	Now func() time.Time
}

func NewPostponedService(d PostponedDeps) *PostponedService {
	now := d.Now
	if now == nil {
		now = time.Now
	}

	return &PostponedService{
		postponed:  d.Postponed,
		poster:     d.Poster,
		tx:         d.Tx,
		photos:     d.Photos,
		moderators: d.Moderators,
		now:        now,
	}
}

// Postpone keeps d to publish at its time. Its photos are kept for good
// at once, as a suggestion's: a post may wait longer than media keeps
// unpublished photos. The caller has checked that the author may speak
// for the community and that the photos are theirs.
func (s *PostponedService) Postpone(ctx context.Context, d post.PostponedDraft) (*post.Postponed, error) {
	p, err := post.NewPostponed(d, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewPostponed", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.postponed.Save(ctx, p); err != nil {
			return errs.Wrap("s.postponed.Save", err)
		}

		if photos := d.Content.Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Publish", s.photos.Publish(ctx, d.AuthorID, photos))
		}

		return nil
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return p, nil
}

// List is every post that waits for a wall, the soonest first: for the
// owner of a user's wall and for the admins of a community.
func (s *PostponedService) List(ctx context.Context, viewerID, wallID uuid.UUID) ([]*post.Postponed, error) {
	ok, err := moderates(ctx, s.moderators, wallID, viewerID, viewerID == wallID)
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if !ok {
		return nil, post.ErrCannotSeePostponed
	}

	found, err := s.postponed.List(ctx, wallID)
	if err != nil {
		return nil, errs.Wrap("s.postponed.List", err)
	}

	return found, nil
}

// Get is one postponed post, whole, for those who see it in the list and
// for its author. Others are told there is none.
func (s *PostponedService) Get(ctx context.Context, viewerID, id uuid.UUID) (*post.Postponed, error) {
	p, err := s.postponed.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.postponed.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, p.WallID(), viewerID, p.CanBeDeletedBy(viewerID))
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if !ok {
		return nil, post.ErrPostponedNotFound
	}

	return p, nil
}

// Edit changes what a postponed post says and when it comes out; see
// changeable for who may.
func (s *PostponedService) Edit(ctx context.Context, requesterID, id uuid.UUID, edit post.PostponedEdit) (*post.Postponed, error) {
	p, err := s.changeable(ctx, requesterID, id)
	if err != nil {
		return nil, errs.Wrap("s.changeable", err)
	}

	edited, err := p.Edited(edit, s.now())
	if err != nil {
		return nil, errs.Wrap("p.Edited", err)
	}

	if err = s.postponed.Update(ctx, edited); err != nil {
		return nil, errs.Wrap("s.postponed.Update", err)
	}

	return edited, nil
}

// Publish publishes a postponed post at once ("Опубликовать сейчас"); see
// changeable for who may.
func (s *PostponedService) Publish(ctx context.Context, requesterID, id uuid.UUID) (*post.Post, error) {
	p, err := s.changeable(ctx, requesterID, id)
	if err != nil {
		return nil, errs.Wrap("s.changeable", err)
	}

	published, err := s.publish(ctx, p)
	if err != nil {
		return nil, errs.Wrap("s.publish", err)
	}

	return published, nil
}

// changeable returns the postponed post id for requesterID to change: the
// author of a post for their own wall, the admins of the community one
// speaks for. Others are told there is none, if they cannot see it.
func (s *PostponedService) changeable(ctx context.Context, requesterID, id uuid.UUID) (*post.Postponed, error) {
	p, err := s.Get(ctx, requesterID, id)
	if err != nil {
		return nil, errs.Wrap("s.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, p.WallID(), requesterID, p.ChangedBy(requesterID))
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if !ok {
		return nil, post.ErrCannotPostpone
	}

	return p, nil
}

// Delete deletes a postponed post with its photos: its author takes it
// back, or an admin of the community it speaks for.
func (s *PostponedService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	p, err := s.Get(ctx, requesterID, id)
	if err != nil {
		return errs.Wrap("s.Get", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.postponed.Delete(ctx, id); err != nil {
			return errs.Wrap("s.postponed.Delete", err)
		}

		if photos := p.Content().Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, p.AuthorID(), photos))
		}

		return nil
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// PublishDue publishes up to MaxDue postponed posts whose time has come
// and tells how many. One that fails waits for the next round without
// holding the others; a post that came out meanwhile, published at once
// or by another replica, is not published twice.
func (s *PostponedService) PublishDue(ctx context.Context) (int, error) {
	due, err := s.postponed.Due(ctx, s.now(), MaxDue)
	if err != nil {
		return 0, errs.Wrap("s.postponed.Due", err)
	}

	var (
		published int
		failed    []error
	)
	for _, p := range due {
		_, err := s.publish(ctx, p)
		switch {
		case err == nil:
			published++
		case !errors.Is(err, post.ErrPostponedNotFound):
			failed = append(failed, err)
		}
	}

	return published, errs.Wrap("s.publish", errors.Join(failed...))
}

// publish makes p a post, dated now: the post is saved in the transaction
// that deletes p.
func (s *PostponedService) publish(ctx context.Context, p *post.Postponed) (*post.Post, error) {
	published, err := p.Published(s.now())
	if err != nil {
		return nil, errs.Wrap("p.Published", err)
	}

	claim := func(ctx context.Context) error {
		return errs.Wrap("s.postponed.Delete", s.postponed.Delete(ctx, p.ID()))
	}

	if err = s.poster.Posted(ctx, published, p.Mentions(), claim); err != nil {
		return nil, errs.Wrap("s.poster.Posted", err)
	}

	return published, nil
}
