package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/world"
)

const (
	noteFound  = "found_state"
	noteDonate = "donate"
	noteRefund = "refund"
)

// Found founds the state f in the open world for Rules.FoundCost votes of
// its ruler and returns its ID. The votes are paid first; a state that
// could not be made after all gives them back.
func (s *GameService) Found(ctx context.Context, f world.Founding) (uuid.UUID, error) {
	c, err := s.current(ctx)
	if err != nil {
		return uuid.Nil, errs.Wrap("s.current", err)
	}

	g, err := s.games.Game(ctx, c.world.ID(), false)
	if err != nil {
		return uuid.Nil, errs.Wrap("s.games.Game", err)
	}

	if f.ID, err = uuid.NewV7(); err != nil {
		return uuid.Nil, errs.Wrap("uuid.NewV7", err)
	}

	if err = g.CheckFound(c.atlas, c.rules, f); err != nil {
		return uuid.Nil, errs.Wrap("g.CheckFound", err)
	}

	err = s.paid(ctx, VoteOp{UserID: f.RulerID, Votes: c.rules.FoundCost, Note: noteFound, Settled: true}, func() error {
		return s.change(ctx, c, func(g *world.Game) error { return g.Found(c.atlas, c.rules, f, s.now()) })
	})
	if err != nil {
		return uuid.Nil, errs.Wrap("s.paid", err)
	}

	return f.ID, nil
}

// Donate puts votes of userID into the treasury of their state.
func (s *GameService) Donate(ctx context.Context, userID uuid.UUID, votes int64) error {
	c, err := s.current(ctx)
	if err != nil {
		return errs.Wrap("s.current", err)
	}

	g, err := s.games.Game(ctx, c.world.ID(), false)
	if err != nil {
		return errs.Wrap("s.games.Game", err)
	}

	if err = g.CheckDonate(userID, votes); err != nil {
		return errs.Wrap("g.CheckDonate", err)
	}

	err = s.paid(ctx, VoteOp{UserID: userID, Votes: votes, Note: noteDonate}, func() error {
		return s.change(ctx, c, func(g *world.Game) error { return g.Donate(userID, votes, s.now()) })
	})

	return errs.Wrap("s.paid", err)
}

// paid takes the votes of op, then does what they pay for; when that
// fails, the votes go back.
func (s *GameService) paid(ctx context.Context, op VoteOp, do func() error) error {
	if op.Votes <= 0 {
		return do()
	}

	var err error
	if op.OpID, err = uuid.NewV7(); err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	if _, err = s.wallets.Charge(ctx, op); err != nil {
		return errs.Wrap("s.wallets.Charge", err)
	}

	failed := do()
	if failed == nil {
		return nil
	}

	refund := VoteOp{UserID: op.UserID, Votes: op.Votes, Note: noteRefund}
	if refund.OpID, err = uuid.NewV7(); err == nil {
		// Without a cancelled ctx: the votes are owed whatever the caller.
		_, err = s.wallets.Pay(context.WithoutCancel(ctx), refund)
	}

	return errors.Join(failed, errs.Wrap("s.wallets.Pay", err))
}

// Edit changes the state of its ruler userID.
func (s *GameService) Edit(ctx context.Context, userID uuid.UUID, e world.Edit) error {
	return s.act(ctx, func(_ current, g *world.Game, _ time.Time) error {
		return errs.Wrap("g.EditState", g.EditState(userID, e))
	})
}

func (s *GameService) Apply(ctx context.Context, userID, stateID uuid.UUID) error {
	return s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Apply", g.Apply(userID, stateID, now))
	})
}

func (s *GameService) Withdraw(ctx context.Context, userID uuid.UUID) error {
	return s.act(ctx, func(_ current, g *world.Game, _ time.Time) error {
		return errs.Wrap("g.Withdraw", g.Withdraw(userID))
	})
}

func (s *GameService) Answer(ctx context.Context, actorID, applicantID uuid.UUID, accept bool) error {
	return s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Answer", g.Answer(actorID, applicantID, accept, now))
	})
}

func (s *GameService) Leave(ctx context.Context, userID uuid.UUID) error {
	return s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Leave", g.Leave(userID, now))
	})
}

func (s *GameService) Expel(ctx context.Context, actorID, targetID uuid.UUID) error {
	return s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Expel", g.Expel(actorID, targetID, now))
	})
}

func (s *GameService) Appoint(ctx context.Context, actorID, targetID uuid.UUID, role world.Role) error {
	return s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Appoint", g.Appoint(actorID, targetID, role, now))
	})
}

func (s *GameService) Abdicate(ctx context.Context, actorID, heirID uuid.UUID) error {
	return s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Abdicate", g.Abdicate(actorID, heirID, now))
	})
}

func (s *GameService) Colonize(ctx context.Context, actorID uuid.UUID, province int) error {
	return s.act(ctx, func(c current, g *world.Game, now time.Time) error {
		return errs.Wrap("g.Colonize", g.Colonize(c.atlas, c.rules, actorID, province, now))
	})
}

// PurgeUser forgets userID, whose page is gone, in the open world.
func (s *GameService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	err := s.act(ctx, func(_ current, g *world.Game, now time.Time) error {
		g.Purge(userID, now)
		return nil
	})
	if errors.Is(err, world.ErrNoWorld) {
		return nil
	}

	return err
}

// Tick counts the hours of the open world up to now and returns how many;
// no open world counts none.
func (s *GameService) Tick(ctx context.Context) (int, error) {
	hours := 0
	err := s.act(ctx, func(c current, g *world.Game, now time.Time) error {
		hours = g.Tick(c.atlas, c.rules, now)
		return nil
	})
	if errors.Is(err, world.ErrNoWorld) {
		return 0, nil
	}

	return hours, err
}

// MakeClubs makes the groups of the states that have none yet and
// returns how many it made.
func (s *GameService) MakeClubs(ctx context.Context) (int, error) {
	c, err := s.current(ctx)
	if errors.Is(err, world.ErrNoWorld) {
		return 0, nil
	}
	if err != nil {
		return 0, errs.Wrap("s.current", err)
	}

	g, err := s.games.Game(ctx, c.world.ID(), false)
	if err != nil {
		return 0, errs.Wrap("s.games.Game", err)
	}

	made := 0
	for _, st := range g.WithoutClub() {
		clubID, err := s.clubs.CreateStateClub(ctx, st.ID, st.RulerID, st.Name.String(), st.Motto.String())
		if err != nil {
			return made, errs.Wrap("s.clubs.CreateStateClub", err)
		}
		if clubID == uuid.Nil {
			continue // no club service to make groups
		}

		err = s.change(ctx, c, func(g *world.Game) error {
			g.SetClub(st.ID, clubID)
			return nil
		})
		if err != nil {
			return made, errs.Wrap("s.change", err)
		}
		made++
	}

	return made, nil
}
