package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

type fakeDatingNotifier struct{ matches [][2]uuid.UUID }

func (f *fakeDatingNotifier) Matched(_ context.Context, first, second uuid.UUID) error {
	f.matches = append(f.matches, [2]uuid.UUID{first, second})

	return nil
}

func TestDating(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	notifier := &fakeDatingNotifier{}
	dating := service.NewDatingService(w.users, inmem.NewDatingRepository(w.users), notifier, inmem.TxManager{})
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	people := map[string]user.Gender{"ann": user.GenderFemale, "bob": user.GenderMale, "cid": user.GenderMale, "dan": user.GenderMale}
	ids := map[string]uuid.UUID{}
	for nick, g := range people {
		ids[nick] = w.person(t, nick)
		p := profile(t, nick)
		p.Gender = g
		if _, err := w.userSvc.UpdateProfile(ctx, ids[nick], p, ""); err != nil {
			t.Fatal(err)
		}
	}
	ann, bob, cid, dan := ids["ann"], ids["bob"], ids["cid"], ids["dan"]
	card := func(id uuid.UUID, seek user.Gender, on bool) {
		t.Helper()

		c, err := user.NewDatingCard(id, user.DatingLove, seek, "", on, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = dating.SaveCard(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	// Without a profile there is no card.
	stranger := w.person(t, "eve")
	if err := dating.SaveCard(ctx, user.DatingCard{UserID: stranger, Goal: user.DatingChat, On: true}); !errors.Is(err, user.ErrDatingProfile) {
		t.Fatalf("a card without a profile: %v", err)
	}

	// Before a card: no deck; answering is refused.
	view, err := dating.Dating(ctx, ann, now)
	if err != nil || view.Card != nil || view.Deck != nil {
		t.Fatalf("Dating before a card = %+v, %v", view, err)
	}
	if _, err = dating.Answer(ctx, service.DatingAnswer{UserID: ann, TargetID: bob, Like: true, Now: now}); !errors.Is(err, user.ErrDatingOff) {
		t.Fatalf("an answer without a card: %v", err)
	}

	// Ann seeks men; Bob seeks women, Cid seeks men, Dan is off.
	card(ann, user.GenderMale, true)
	card(bob, user.GenderFemale, true)
	card(cid, user.GenderMale, true)
	card(dan, user.GenderNone, false)
	if view, err = dating.Dating(ctx, ann, now); err != nil || view.Card == nil || len(view.Deck) != 1 || view.Deck[0].UserID != bob {
		t.Fatalf("the deck of Ann = %+v, %v", view.Deck, err)
	}

	if _, err = dating.Answer(ctx, service.DatingAnswer{UserID: ann, TargetID: ann, Like: true, Now: now}); !errors.Is(err, user.ErrDatingSelf) {
		t.Fatalf("liking oneself: %v", err)
	}
	if _, err = dating.Answer(ctx, service.DatingAnswer{UserID: ann, TargetID: dan, Like: true, Now: now}); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("liking a card that is off: %v", err)
	}

	matched, err := dating.Answer(ctx, service.DatingAnswer{UserID: ann, TargetID: bob, Like: true, Now: now})
	if err != nil || matched || len(notifier.matches) != 0 {
		t.Fatalf("the first like = %v, %v", matched, err)
	}
	if _, err = dating.Answer(ctx, service.DatingAnswer{UserID: ann, TargetID: bob, Like: true, Now: now}); !errors.Is(err, user.ErrDatingTwice) {
		t.Fatalf("a second like: %v", err)
	}
	if view, err = dating.Dating(ctx, ann, now); err != nil || len(view.Deck) != 0 {
		t.Fatalf("the deck after the like = %+v, %v", view.Deck, err)
	}

	later := now.Add(time.Hour)
	matched, err = dating.Answer(ctx, service.DatingAnswer{UserID: bob, TargetID: ann, Like: true, Now: later})
	if err != nil || !matched || len(notifier.matches) != 1 || notifier.matches[0] != [2]uuid.UUID{bob, ann} {
		t.Fatalf("the like back = %v, %v, %v", matched, err, notifier.matches)
	}
	if view, err = dating.Dating(ctx, ann, later); err != nil || len(view.Matches) != 1 || view.Matches[0].UserID != bob || !view.Matches[0].At.Equal(later) {
		t.Fatalf("the matches of Ann = %+v, %v", view.Matches, err)
	}

	// A skip is no match; a deleted page leaves the deck.
	card(dan, user.GenderNone, true)
	if matched, err = dating.Answer(ctx, service.DatingAnswer{UserID: cid, TargetID: dan, Like: false, Now: now}); err != nil || matched {
		t.Fatalf("a skip = %v, %v", matched, err)
	}
	if view, err = dating.Dating(ctx, bob, now); err != nil || len(view.Deck) != 0 {
		t.Fatalf("the deck of Bob = %+v, %v", view.Deck, err)
	}
	// Dan seeks anyone: Ann, now seeking anyone too, and Cid, who seeks
	// men; Bob seeks women.
	card(ann, user.GenderNone, true)
	if view, err = dating.Dating(ctx, dan, now); err != nil || len(view.Deck) != 2 {
		t.Fatalf("the deck of Dan = %+v, %v", view.Deck, err)
	}
	if _, err = w.accounts.Deactivate(ctx, ann, ""); err != nil {
		t.Fatal(err)
	}
	if view, err = dating.Dating(ctx, dan, now); err != nil || len(view.Deck) != 1 || view.Deck[0].UserID != cid {
		t.Fatalf("the deck of Dan after Ann left = %+v, %v", view.Deck, err)
	}
}
