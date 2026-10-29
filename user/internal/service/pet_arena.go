package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PetEventRepository keeps the boss and the tournament of each week.
type PetEventRepository interface {
	// Boss and Tournament are those of week, user.ErrNotFound before
	// anything happened to them.
	Boss(ctx context.Context, week time.Time) (*user.PetBoss, error)
	Tournament(ctx context.Context, week time.Time) (*user.PetTournament, error)
	// LockBoss and LockTournament are those of the week of fresh, fresh
	// when there was none, kept so until the end of the transaction.
	LockBoss(ctx context.Context, fresh *user.PetBoss) (*user.PetBoss, error)
	LockTournament(ctx context.Context, fresh *user.PetTournament) (*user.PetTournament, error)
	SaveBoss(ctx context.Context, b *user.PetBoss) error
	SaveTournament(ctx context.Context, t *user.PetTournament) error
}

// PetSeasonRepository keeps the seasons of the rating.
type PetSeasonRepository interface {
	// Best are the pets of the highest rating of owners neither deleted
	// nor banned at now, up to limit.
	Best(ctx context.Context, now time.Time, limit int) ([]user.PetContender, error)
	// Seasons are those closed, the last first, up to limit.
	Seasons(ctx context.Context, limit int) ([]user.PetSeasonResult, error)
	// CloseSeason keeps r, false when its season was closed already; the
	// ratings of all the pets then go to user.SeasonRating of them.
	CloseSeason(ctx context.Context, r user.PetSeasonResult) (bool, error)
}

// PetQuestRepository keeps the tasks of the day and the badges.
type PetQuestRepository interface {
	// LockQuests are the tasks of userID kept so until the end of the
	// transaction, none done for one who never did any.
	LockQuests(ctx context.Context, userID uuid.UUID) (*user.PetQuests, error)
	SaveQuests(ctx context.Context, userID uuid.UUID, q *user.PetQuests) error
	// Achievements are the badges of userID, the oldest first; Achieve
	// adds those not had yet.
	Achievements(ctx context.Context, userID uuid.UUID) ([]user.PetAchievementGot, error)
	Achieve(ctx context.Context, userID uuid.UUID, got []user.PetAchievement, now time.Time) error
}

// PetMarketRepository keeps the pets up for sale.
type PetMarketRepository interface {
	// Listings are the pets up for sale of owners neither deleted nor
	// banned at now, the cheapest first, up to limit.
	Listings(ctx context.Context, now time.Time, limit int) ([]user.PetContender, error)
	// LockListed is the pet petID and its owner kept so until the end of
	// the transaction; user.ErrPetNotFound.
	LockListed(ctx context.Context, petID uuid.UUID) (user.PetContender, error)
	// MovePet hands the pet petID to toID; user.ErrTooManyPets when they
	// keep user.MaxPets.
	MovePet(ctx context.Context, petID, toID uuid.UUID) error
}

const (
	// petListings are the pets of the market shown.
	petListings = 50
	// petSeasonsShown are the closed seasons shown.
	petSeasonsShown = 3
	// petBest are the pets read for the places of a season: enough for
	// PetSeasonTop owners of several pets each.
	petBest = user.PetSeasonTop * user.MaxPets
)

// PetArena is what the pets do together: the boss and the tournament of
// the week and the last, the places of the season now and of those
// closed, the tasks and badges of the person, and the market.
type PetArena struct {
	Boss         *user.PetBoss
	Tournament   *user.PetTournament
	Last         *user.PetTournament
	Top          []user.PetSeasonPlace
	Seasons      []user.PetSeasonResult
	Quests       *user.PetQuests
	Achievements []user.PetAchievementGot
	Market       []user.PetContender
}

// PetArenaAction is something done on the arena: hit the boss, join or
// leave the tournament, bet, put a pet up for sale, take it off or buy
// one.
type PetArenaAction string

const (
	PetBossHit    PetArenaAction = "boss"
	PetJoin       PetArenaAction = "join"
	PetLeave      PetArenaAction = "leave"
	PetBetOn      PetArenaAction = "bet"
	PetSell       PetArenaAction = "sell"
	PetUnsell     PetArenaAction = "unsell"
	PetBuy        PetArenaAction = "buy"
	petArenaVotes                = "arena"
)

// ErrPetArenaAction is an action the arena does not know.
var ErrPetArenaAction = errors.New("unknown pet arena action")

// PetArenaAct is an action of a person on a pet, with the votes of a bet
// or the price of a sale.
type PetArenaAct struct {
	Action PetArenaAction
	UserID uuid.UUID
	PetID  uuid.UUID
	Votes  int64
	Now    time.Time
}

// PetArenaDone is what an action brought: the hit of the boss, and the
// votes after it, 0 when nothing was paid.
type PetArenaDone struct {
	Bout    *user.PetBout
	Balance int64
}

// Arena is the arena as userID sees it at now, the season and the
// tournaments that are over settled first.
func (s *PetService) Arena(ctx context.Context, userID uuid.UUID, now time.Time) (PetArena, error) {
	now = now.Truncate(time.Microsecond)
	if err := s.settleArena(ctx, now); err != nil {
		return PetArena{}, errs.Wrap("s.settleArena", err)
	}
	if err := s.achieve(ctx, userID, now); err != nil {
		return PetArena{}, errs.Wrap("s.achieve", err)
	}

	var a PetArena
	var err error
	week := user.PetWeek(now)
	if a.Boss, err = s.events.Boss(ctx, week); missing(err) {
		a.Boss, err = user.NewPetBoss(now), nil
	}
	if err != nil {
		return PetArena{}, errs.Wrap("s.events.Boss", err)
	}
	if a.Tournament, err = s.events.Tournament(ctx, week); missing(err) {
		a.Tournament, err = user.NewPetTournament(now), nil
	}
	if err != nil {
		return PetArena{}, errs.Wrap("s.events.Tournament", err)
	}
	if a.Last, err = s.events.Tournament(ctx, week.AddDate(0, 0, -7)); missing(err) {
		err = nil
	}
	if err != nil {
		return PetArena{}, errs.Wrap("s.events.Tournament", err)
	}

	best, err := s.seasons.Best(ctx, now, petBest)
	if err != nil {
		return PetArena{}, errs.Wrap("s.seasons.Best", err)
	}
	a.Top = user.PetSeasonPlaces(best)

	if a.Seasons, err = s.seasons.Seasons(ctx, petSeasonsShown); err != nil {
		return PetArena{}, errs.Wrap("s.seasons.Seasons", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a.Quests, err = s.quests.LockQuests(ctx, userID)

		return errs.Wrap("s.quests.LockQuests", err)
	})
	if err != nil {
		return PetArena{}, errs.Wrap("s.tx.WithinTx", err)
	}

	if a.Achievements, err = s.quests.Achievements(ctx, userID); err != nil {
		return PetArena{}, errs.Wrap("s.quests.Achievements", err)
	}

	a.Market, err = s.market.Listings(ctx, now, petListings)

	return a, errs.Wrap("s.market.Listings", err)
}

// missing is whether err says a week had nothing happen yet.
func missing(err error) bool {
	return errors.Is(err, user.ErrNotFound)
}

// achieve gives userID the badges of what they have at now.
func (s *PetService) achieve(ctx context.Context, userID uuid.UUID, now time.Time) error {
	pets, err := s.pets.Pets(ctx, userID)
	if err != nil {
		return errs.Wrap("s.pets.Pets", err)
	}

	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	var streak int
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		q, err := s.quests.LockQuests(ctx, userID)
		streak = q.StreakAt(now)

		return errs.Wrap("s.quests.LockQuests", err)
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	got := user.EarnedAchievements(pets, u.Wallet().Votes, streak)
	if len(got) == 0 {
		return nil
	}

	return errs.Wrap("s.quests.Achieve", s.quests.Achieve(ctx, userID, got, now))
}

// settleArena closes the last season and holds the tournaments due at
// now, paying what they bring.
func (s *PetService) settleArena(ctx context.Context, now time.Time) error {
	if err := s.closeSeason(ctx, now); err != nil {
		return errs.Wrap("s.closeSeason", err)
	}

	week := user.PetWeek(now)
	for _, w := range []time.Time{week.AddDate(0, 0, -7), week} {
		if err := s.holdTournament(ctx, w, now); err != nil {
			return errs.Wrap("s.holdTournament", err)
		}
	}

	return nil
}

// closeSeason closes the season before that of now, rewarding its best,
// unless it was closed. The first time the arena opens nothing was
// fought for in a season: the last is closed with no places.
func (s *PetService) closeSeason(ctx context.Context, now time.Time) error {
	last := user.PetSeason(now).AddDate(0, -1, 0)
	closed, err := s.seasons.Seasons(ctx, 1)
	if err != nil {
		return errs.Wrap("s.seasons.Seasons", err)
	}
	if len(closed) > 0 && !closed[0].Season.Before(last) {
		return nil
	}

	result := user.PetSeasonResult{Season: last}
	if len(closed) > 0 {
		best, err := s.seasons.Best(ctx, now, petBest)
		if err != nil {
			return errs.Wrap("s.seasons.Best", err)
		}
		result.Places = user.PetSeasonPlaces(best)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		first, err := s.seasons.CloseSeason(ctx, result)
		if err != nil || !first {
			return errs.Wrap("s.seasons.CloseSeason", err)
		}

		for _, p := range result.Places {
			if err = s.reward(ctx, p.Owner, p.Prize, "season"); err != nil {
				return errs.Wrap("s.reward", err)
			}
			if err = s.award(ctx, p.Owner, user.PetSeasonBest, now); err != nil {
				return errs.Wrap("s.award", err)
			}
		}

		return nil
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// reward pays votes to userID from the treasury; one gone gets nothing.
func (s *PetService) reward(ctx context.Context, userID uuid.UUID, votes int64, note string) error {
	if votes <= 0 {
		return nil
	}

	_, err := s.wallets.Credit(ctx, userID, votes, 0, user.Entry(user.VotePet, uuid.Nil, note))
	if errors.Is(err, user.ErrNotFound) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.wallets.Credit", err)
	}

	return errs.Wrap("s.pets.Count", s.pets.Count(ctx, 0, votes))
}

// award gives userID the badge a at now; one gone gets nothing.
func (s *PetService) award(ctx context.Context, userID uuid.UUID, a user.PetAchievement, now time.Time) error {
	err := s.quests.Achieve(ctx, userID, []user.PetAchievement{a}, now)
	if errors.Is(err, user.ErrNotFound) {
		return nil
	}

	return errs.Wrap("s.quests.Achieve", err)
}

// holdTournament fights the tournament of week if it is due at now, pays
// its places and settles its bets.
func (s *PetService) holdTournament(ctx context.Context, week, now time.Time) error {
	t, err := s.events.Tournament(ctx, week)
	if missing(err) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.events.Tournament", err)
	}
	if !t.Due(now) {
		return nil
	}

	// A match rolls dice of its own: one fewer than the pets.
	dice := make([]user.PetDice, user.PetTournamentSize)
	for i := range dice {
		if dice[i], err = s.dice(); err != nil {
			return errs.Wrap("s.dice", err)
		}
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := s.events.LockTournament(ctx, user.NewPetTournament(week))
		if err != nil {
			return errs.Wrap("s.events.LockTournament", err)
		}
		if !t.Due(now) {
			return nil
		}

		pets := map[uuid.UUID]*user.Pet{}
		for _, e := range t.Entrants() {
			p, err := s.pets.Pet(ctx, e.Owner, e.PetID)
			if errors.Is(err, user.ErrPetNotFound) {
				continue
			}
			if err != nil {
				return errs.Wrap("s.pets.Pet", err)
			}
			pets[e.PetID] = p
		}

		next := 0
		t = t.Played(pets, now, func() user.PetDice { next++; return dice[next-1] })

		return errs.Wrap("s.payTournament", s.payTournament(ctx, t, now))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// payTournament saves the tournament held, pays its places and its bets.
func (s *PetService) payTournament(ctx context.Context, t *user.PetTournament, now time.Time) error {
	if err := s.events.SaveTournament(ctx, t); err != nil {
		return errs.Wrap("s.events.SaveTournament", err)
	}

	for _, p := range t.Prizes() {
		if err := s.reward(ctx, p.UserID, p.Votes, "tournament"); err != nil {
			return errs.Wrap("s.reward", err)
		}
	}
	if w, ok := t.Winner(); ok {
		if err := s.award(ctx, w.Owner, user.PetChampion, now); err != nil {
			return errs.Wrap("s.award", err)
		}
	}

	paid, cut := t.Winnings()
	for _, p := range paid {
		_, err := s.wallets.Release(ctx, p.UserID, p.Votes, user.Entry(user.VotePet, uuid.Nil, "bet"))
		if errors.Is(err, user.ErrNotFound) {
			cut += p.Votes // the winnings of one gone go to the treasury
			continue
		}
		if err != nil {
			return errs.Wrap("s.wallets.Release", err)
		}
	}
	if cut == 0 {
		return nil
	}

	return errs.Wrap("s.wallets.Deposit", s.wallets.Deposit(ctx, cut))
}

// RunArena does the action of a.UserID on the arena: user.ErrPetNotFound
// and what the boss, the tournament and the market refuse.
func (s *PetService) RunArena(ctx context.Context, a PetArenaAct) (PetArenaDone, error) {
	a.Now = a.Now.Truncate(time.Microsecond)

	var done PetArenaDone
	var err error
	switch a.Action {
	case PetBossHit:
		done, err = s.hitBoss(ctx, a)
	case PetJoin, PetLeave, PetBetOn:
		done.Balance, err = s.enter(ctx, a)
	case PetSell, PetUnsell:
		err = s.list(ctx, a)
	case PetBuy:
		done.Balance, err = s.buy(ctx, a)
	default:
		err = ErrPetArenaAction
	}

	return done, err
}

// hitBoss hits the boss of the week with the pet of a; the hit that
// beats it pays all who hit it.
func (s *PetService) hitBoss(ctx context.Context, a PetArenaAct) (PetArenaDone, error) {
	dice, err := s.dice()
	if err != nil {
		return PetArenaDone{}, errs.Wrap("s.dice", err)
	}

	var done PetArenaDone

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		shop, err := s.pets.Shop(ctx)
		if err != nil {
			return errs.Wrap("s.pets.Shop", err)
		}

		boss, err := s.events.LockBoss(ctx, user.NewPetBoss(a.Now))
		if err != nil {
			return errs.Wrap("s.events.LockBoss", err)
		}

		p, err := s.pets.LockPet(ctx, a.UserID, a.PetID)
		if err != nil {
			return errs.Wrap("s.pets.LockPet", err)
		}

		hit, pet, bout, err := boss.Hit(a.UserID, p, shop, a.Now, dice)
		if err != nil {
			return errs.Wrap("boss.Hit", err)
		}
		done.Bout = &bout

		if err = s.pets.SavePet(ctx, pet); err != nil {
			return errs.Wrap("s.pets.SavePet", err)
		}
		if err = s.events.SaveBoss(ctx, hit); err != nil {
			return errs.Wrap("s.events.SaveBoss", err)
		}
		if err = s.earnShards(ctx, a.UserID, user.PetBossShards); err != nil {
			return errs.Wrap("s.earnShards", err)
		}
		if hit.Down() {
			if err = s.payBoss(ctx, hit, a.Now); err != nil {
				return errs.Wrap("s.payBoss", err)
			}
		}

		done.Balance, err = s.did(ctx, a.UserID, a.Now, user.PetQuestBoss)

		return errs.Wrap("s.did", err)
	})
	if err != nil {
		return PetArenaDone{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return done, nil
}

// payBoss pays the boss just beaten to those who hit it.
func (s *PetService) payBoss(ctx context.Context, b *user.PetBoss, now time.Time) error {
	for _, p := range b.Prizes() {
		if err := s.reward(ctx, p.UserID, p.Votes, "boss"); err != nil {
			return errs.Wrap("s.reward", err)
		}
	}

	return errs.Wrap("s.award", s.award(ctx, b.KilledBy(), user.PetBossSlayer, now))
}

// earnShards adds shards to the keeper of userID.
func (s *PetService) earnShards(ctx context.Context, userID uuid.UUID, shards int) error {
	if shards == 0 {
		return nil
	}

	k, err := s.keepers.LockKeeper(ctx, userID)
	if err != nil {
		return errs.Wrap("s.keepers.LockKeeper", err)
	}

	return errs.Wrap("s.keepers.SaveKeeper", s.keepers.SaveKeeper(ctx, userID, k.Earned(shards)))
}

// enter joins the tournament of the week with the pet of a, leaves it or
// bets on the pet, holding the votes of the bet until it is over.
func (s *PetService) enter(ctx context.Context, a PetArenaAct) (int64, error) {
	var balance int64

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := s.events.LockTournament(ctx, user.NewPetTournament(a.Now))
		if err != nil {
			return errs.Wrap("s.events.LockTournament", err)
		}

		switch a.Action {
		case PetJoin:
			p, err := s.pets.Pet(ctx, a.UserID, a.PetID)
			if err != nil {
				return errs.Wrap("s.pets.Pet", err)
			}
			if t, err = t.Joined(a.UserID, p, a.Now); err != nil {
				return errs.Wrap("t.Joined", err)
			}
		case PetLeave:
			if t, err = t.Left(a.UserID, a.Now); err != nil {
				return errs.Wrap("t.Left", err)
			}
		default:
			if t, err = t.Bet(a.UserID, a.PetID, a.Votes, a.Now); err != nil {
				return errs.Wrap("t.Bet", err)
			}
			if balance, err = s.wallets.Hold(ctx, a.UserID, a.Votes, user.Entry(user.VotePet, uuid.Nil, "bet")); err != nil {
				return errs.Wrap("s.wallets.Hold", err)
			}
		}

		return errs.Wrap("s.events.SaveTournament", s.events.SaveTournament(ctx, t))
	})

	return balance, errs.Wrap("s.tx.WithinTx", err)
}

// list puts the pet of a up for sale for a.Votes or takes it off: a pet
// in a fight led by hand is not sold.
func (s *PetService) list(ctx context.Context, a PetArenaAct) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.pets.LockPet(ctx, a.UserID, a.PetID)
		if err != nil {
			return errs.Wrap("s.pets.LockPet", err)
		}

		if a.Action == PetUnsell {
			return errs.Wrap("s.pets.SavePet", s.pets.SavePet(ctx, p.Unlisted()))
		}

		if _, err = s.battles.LockBattle(ctx, a.UserID, a.PetID); !errors.Is(err, user.ErrNoPetBattle) {
			if err == nil {
				err = user.ErrPetInBattle
			}

			return errs.Wrap("s.battles.LockBattle", err)
		}

		pets, err := s.pets.Pets(ctx, a.UserID)
		if err != nil {
			return errs.Wrap("s.pets.Pets", err)
		}
		var listed int
		for _, o := range pets {
			if o.SalePrice() > 0 && o.ID() != a.PetID {
				listed++
			}
		}
		if listed >= user.MaxPetSales {
			return user.ErrTooManyPetSales
		}

		if p, err = p.ListedFor(a.Votes, a.Now); err != nil {
			return errs.Wrap("p.ListedFor", err)
		}

		return errs.Wrap("s.pets.SavePet", s.pets.SavePet(ctx, p))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// buy buys the pet a.PetID up for sale for its price: the treasury takes
// user.PetMarketFee percent, its owner gets the rest.
func (s *PetService) buy(ctx context.Context, a PetArenaAct) (int64, error) {
	var balance int64

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		listed, err := s.market.LockListed(ctx, a.PetID)
		if err != nil {
			return errs.Wrap("s.market.LockListed", err)
		}
		if listed.Owner == a.UserID {
			return user.ErrPetOwnSale
		}

		price := listed.Pet.SalePrice()
		sold, err := listed.Pet.SoldAt(a.Now)
		if err != nil {
			return errs.Wrap("listed.Pet.SoldAt", err)
		}

		fee := user.PetSaleFee(price)
		if _, err = s.wallets.Spend(ctx, a.UserID, fee, user.Entry(user.VotePet, listed.Owner, "market")); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}
		if balance, err = s.wallets.Move(ctx, a.UserID, listed.Owner, price-fee, user.Entry(user.VotePet, uuid.Nil, "market")); err != nil {
			return errs.Wrap("s.wallets.Move", err)
		}

		if err = s.pets.SavePet(ctx, sold); err != nil {
			return errs.Wrap("s.pets.SavePet", err)
		}

		return errs.Wrap("s.market.MovePet", s.market.MovePet(ctx, a.PetID, a.UserID))
	})

	return balance, errs.Wrap("s.tx.WithinTx", err)
}

// did counts kinds done once each by userID at now towards the tasks of
// the day and pays those finished; the votes after, 0 when none were
// paid.
func (s *PetService) did(ctx context.Context, userID uuid.UUID, now time.Time, kinds ...user.PetQuestKind) (int64, error) {
	q, err := s.quests.LockQuests(ctx, userID)
	if err != nil {
		return 0, errs.Wrap("s.quests.LockQuests", err)
	}

	var votes int64
	var shards int
	for _, k := range kinds {
		var prize user.PetQuestPrize
		q, prize = q.Did(k, 1, now)
		votes += prize.Votes
		shards += prize.Shards
	}

	if err = s.quests.SaveQuests(ctx, userID, q); err != nil {
		return 0, errs.Wrap("s.quests.SaveQuests", err)
	}
	if err = s.earnShards(ctx, userID, shards); err != nil {
		return 0, errs.Wrap("s.earnShards", err)
	}
	if votes == 0 {
		return 0, nil
	}

	wallet, err := s.wallets.Credit(ctx, userID, votes, 0, user.Entry(user.VotePet, uuid.Nil, "quest"))
	if err != nil {
		return 0, errs.Wrap("s.wallets.Credit", err)
	}

	return wallet.Votes, errs.Wrap("s.pets.Count", s.pets.Count(ctx, 0, votes))
}
