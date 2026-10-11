package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// MaxPollBatch is how many posts' polls are read at once: those of a page
// of posts and of the posts they share.
const MaxPollBatch = 2 * MaxPageSize

// PollRepository keeps polls with their votes, counted in the poll by the
// statements that add and remove them. A poll goes with its post.
type PollRepository interface {
	Save(ctx context.Context, p *post.Poll) error
	// Get returns post.ErrPollNotFound. Lock is Get that holds the poll
	// until the transaction of ctx ends, so that one's votes are counted
	// one after another.
	Get(ctx context.Context, postID uuid.UUID) (*post.Poll, error)
	Lock(ctx context.Context, postID uuid.UUID) (*post.Poll, error)
	// GetMany returns the polls of these posts that ask one.
	GetMany(ctx context.Context, postIDs []uuid.UUID) ([]*post.Poll, error)
	// Chosen is what userID chose in the polls of these posts, where they
	// voted.
	Chosen(ctx context.Context, userID uuid.UUID, postIDs []uuid.UUID) (map[uuid.UUID]post.PollChoice, error)
	// Vote returns post.ErrPollVoted when userID has chosen an option of
	// choice already.
	Vote(ctx context.Context, postID, userID uuid.UUID, choice post.PollChoice, at time.Time) error
	Retract(ctx context.Context, postID, userID uuid.UUID) error
	// Voters returns up to limit of those who chose the option at place,
	// newest first, after the cursor (nil: from the newest).
	Voters(ctx context.Context, postID uuid.UUID, place int, after *LikerCursor, limit int) ([]Liker, error)
}

// PollPostGetter finds the post a poll is asked by: post.ErrNotFound.
type PollPostGetter interface {
	Get(ctx context.Context, id uuid.UUID) (*post.Post, error)
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error)
}

// PollView is a poll as someone sees it: Chosen is what they chose in it,
// nothing for one who has not voted and for an anonymous viewer.
type PollView struct {
	Poll   *post.Poll
	Chosen post.PollChoice
	// Ended: the poll takes no votes anymore.
	Ended bool
}

type PollService struct {
	polls      PollRepository
	posts      PollPostGetter
	moderators WallModerators
	tx         TxManager
	now        func() time.Time
}

// PollDeps are what PollService works with.
type PollDeps struct {
	Polls      PollRepository
	Posts      PollPostGetter
	Moderators WallModerators
	Tx         TxManager
	// Now is the clock; nil: time.Now.
	Now func() time.Time
}

func NewPollService(d PollDeps) *PollService {
	now := d.Now
	if now == nil {
		now = time.Now
	}

	return &PollService{
		polls:      d.Polls,
		posts:      d.Posts,
		moderators: d.Moderators,
		tx:         d.Tx,
		now:        now,
	}
}

// Of returns the polls of the posts that ask one, as viewerID sees them
// (uuid.Nil: anonymous), in no particular order, at most MaxPollBatch.
// The poll of a post put away in an archive is shown to those who keep it
// only, as the post is.
func (s *PollService) Of(ctx context.Context, viewerID uuid.UUID, postIDs []uuid.UUID) ([]PollView, error) {
	if len(postIDs) > MaxPollBatch {
		return nil, ErrTooManyIDs
	}

	if len(postIDs) == 0 {
		return nil, nil
	}

	posts, err := s.posts.GetMany(ctx, postIDs)
	if err != nil {
		return nil, errs.Wrap("s.posts.GetMany", err)
	}

	shown := make([]uuid.UUID, 0, len(posts))
	for _, p := range posts {
		if !p.Content().Polled() {
			continue
		}

		away, err := hidden(ctx, s.moderators, p, viewerID)
		if err != nil {
			return nil, errs.Wrap("hidden", err)
		}

		if !away {
			shown = append(shown, p.ID())
		}
	}

	if len(shown) == 0 {
		return nil, nil
	}

	polls, err := s.polls.GetMany(ctx, shown)
	if err != nil {
		return nil, errs.Wrap("s.polls.GetMany", err)
	}

	var chosen map[uuid.UUID]post.PollChoice
	if viewerID != uuid.Nil {
		if chosen, err = s.polls.Chosen(ctx, viewerID, shown); err != nil {
			return nil, errs.Wrap("s.polls.Chosen", err)
		}
	}

	now := s.now()

	views := make([]PollView, 0, len(polls))
	for _, p := range polls {
		views = append(views, PollView{Poll: p, Chosen: chosen[p.PostID()], Ended: p.Ended(now)})
	}

	return views, nil
}

// Vote casts the vote of userID in the poll of a post they are shown:
// one option, or several where the poll lets. A vote is cast once and
// changed by taking it back first, as in openvk. It returns the poll as
// it stands then.
func (s *PollService) Vote(ctx context.Context, userID, postID uuid.UUID, places []int) (PollView, error) {
	if err := s.checkSeen(ctx, userID, postID); err != nil {
		return PollView{}, err
	}

	var view PollView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, voted, err := s.locked(ctx, userID, postID)
		if err != nil {
			return err
		}

		choice, err := p.Choice(places)
		if err != nil {
			return errs.Wrap("p.Choice", err)
		}

		now := s.now()

		if err = p.CheckVote(voted != nil, now); err != nil {
			return errs.Wrap("p.CheckVote", err)
		}

		if err = s.polls.Vote(ctx, postID, userID, choice, now); err != nil {
			return errs.Wrap("s.polls.Vote", err)
		}

		view = PollView{Poll: p.Counted(choice, 1), Chosen: choice}

		return nil
	})
	if err != nil {
		return PollView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// Retract takes the vote of userID back, where the poll lets (see
// post.CheckRetract), and returns the poll as it stands then.
func (s *PollService) Retract(ctx context.Context, userID, postID uuid.UUID) (PollView, error) {
	if err := s.checkSeen(ctx, userID, postID); err != nil {
		return PollView{}, err
	}

	var view PollView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, voted, err := s.locked(ctx, userID, postID)
		if err != nil {
			return err
		}

		if err = p.CheckRetract(voted != nil, s.now()); err != nil {
			return errs.Wrap("p.CheckRetract", err)
		}

		if err = s.polls.Retract(ctx, postID, userID); err != nil {
			return errs.Wrap("s.polls.Retract", err)
		}

		view = PollView{Poll: p.Counted(voted, -1)}

		return nil
	})
	if err != nil {
		return PollView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// locked is the poll of a post, held for the transaction of ctx, and what
// userID chose in it, nil if nothing.
func (s *PollService) locked(ctx context.Context, userID, postID uuid.UUID) (*post.Poll, post.PollChoice, error) {
	p, err := s.polls.Lock(ctx, postID)
	if err != nil {
		return nil, nil, errs.Wrap("s.polls.Lock", err)
	}

	chosen, err := s.polls.Chosen(ctx, userID, []uuid.UUID{postID})
	if err != nil {
		return nil, nil, errs.Wrap("s.polls.Chosen", err)
	}

	return p, chosen[postID], nil
}

// checkSeen returns post.ErrPollNotFound unless userID is shown the post:
// what nobody is shown is not voted in either.
func (s *PollService) checkSeen(ctx context.Context, userID, postID uuid.UUID) error {
	p, err := s.posts.Get(ctx, postID)
	if err != nil {
		return errs.Wrap("s.posts.Get", err)
	}

	away, err := hidden(ctx, s.moderators, p, userID)
	if err != nil {
		return errs.Wrap("hidden", err)
	}

	if away || !p.Content().Polled() {
		return post.ErrPollNotFound
	}

	return nil
}

// Voters pages who chose the option at place, newest first, as openvk's
// "Список проголосовавших"; post.ErrPollAnonymous for a poll that does
// not show them.
func (s *PollService) Voters(ctx context.Context, postID uuid.UUID, place int, cursor *LikerCursor, size int) (LikerPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	p, err := s.polls.Get(ctx, postID)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.polls.Get", err)
	}

	if err = p.CheckVoters(place); err != nil {
		return LikerPage{}, errs.Wrap("p.CheckVoters", err)
	}

	found, err := s.polls.Voters(ctx, postID, place, cursor, size+1)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.polls.Voters", err)
	}

	return likerPage(found, size, p.Tally().Votes[place]), nil
}
