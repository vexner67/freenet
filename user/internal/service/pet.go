package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PetRepository keeps the pets, their fights and the pet shop.
type PetRepository interface {
	// Shop is user.DefaultPetShop until one is set.
	Shop(ctx context.Context) (user.PetShop, error)
	SetShop(ctx context.Context, shop user.PetShop) error
	// Pets are the pets of userID, the oldest first.
	Pets(ctx context.Context, userID uuid.UUID) ([]*user.Pet, error)
	// Pet and LockPet are the pet petID of userID, user.ErrPetNotFound
	// without it; LockPet keeps the pet so until the end of the
	// transaction.
	Pet(ctx context.Context, userID, petID uuid.UUID) (*user.Pet, error)
	LockPet(ctx context.Context, userID, petID uuid.UUID) (*user.Pet, error)
	// AddPet is user.ErrTooManyPets when userID keeps user.MaxPets,
	// user.ErrNotFound without the user.
	AddPet(ctx context.Context, userID uuid.UUID, p *user.Pet) error
	// GrantPet adds p however many pets userID keeps, once a kind: a
	// second time is user.ErrPetClaimed, even after p is gone.
	GrantPet(ctx context.Context, userID uuid.UUID, p *user.Pet, now time.Time) error
	// Granted says whether userID was given a pet of kind.
	Granted(ctx context.Context, userID uuid.UUID, kind user.PetKind) (bool, error)
	SavePet(ctx context.Context, p *user.Pet) error
	DeletePet(ctx context.Context, userID, petID uuid.UUID) error
	// Contenders are the pets nearest to rating, up to limit, of people
	// not userID and neither deleted nor banned at now.
	Contenders(ctx context.Context, userID uuid.UUID, rating int, now time.Time, limit int) ([]user.PetContender, error)
	AddFight(ctx context.Context, f user.PetFight) error
	// Fights are those userID fought on either side, newest first.
	Fights(ctx context.Context, userID uuid.UUID, limit int) ([]user.PetFight, error)
	// Count adds votes spent on pets and won by them.
	Count(ctx context.Context, spent, won int64) error
	Stats(ctx context.Context) (user.PetShopStats, error)
}

// PetKeeperRepository keeps what people keep of the cases.
type PetKeeperRepository interface {
	// Keeper is user.NewPetKeeper for one who never opened a case.
	Keeper(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error)
	// LockKeeper is Keeper kept so until the end of the transaction.
	LockKeeper(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error)
	SaveKeeper(ctx context.Context, userID uuid.UUID, k *user.PetKeeper) error
}

// PetBattleRepository keeps the fights led a round after another, one a
// pet; a pet let go takes its fight.
type PetBattleRepository interface {
	// Battles are those of the pets of userID.
	Battles(ctx context.Context, userID uuid.UUID) ([]*user.PetBattle, error)
	// LockBattle is the fight of the pet petID of userID kept so until the
	// end of the transaction; user.ErrNoPetBattle.
	LockBattle(ctx context.Context, userID, petID uuid.UUID) (*user.PetBattle, error)
	SaveBattle(ctx context.Context, userID uuid.UUID, b *user.PetBattle) error
	DeleteBattle(ctx context.Context, userID, petID uuid.UUID) error
}

// petContenders are the contenders an opponent is picked of.
const petContenders = 200

// PetCare is the pet after something was done to it and the votes of its
// owner after, 0 when nothing was paid.
type PetCare struct {
	Pet     *user.Pet
	Balance int64
}

// PetFought is a fight, the pet after it and the votes of its owner, 0
// when it brought none.
type PetFought struct {
	Fight   user.PetFight
	Pet     *user.Pet
	Balance int64
}

// PetDrop is the keeper after a case and the votes after it, 0 when
// nothing was paid.
type PetDrop struct {
	Keeper    *user.PetKeeper
	Balance   int64
	CasesADay int
}

// PetBattleTurn is a fight after a move and the pet that leads it; Fought
// when it is over.
type PetBattleTurn struct {
	Battle *user.PetBattle
	Pet    *user.Pet
	Fought *PetFought
}

// PetSettings are the shop and how the pets do: for the administrators.
type PetSettings struct {
	Shop  user.PetShop
	Stats user.PetShopStats
}

// PetService is "Питомец": a tamagotchi people adopt for votes, feed,
// play with and set on the pets of others.
type PetService struct {
	userRepo UserRepository
	pets     PetRepository
	keepers  PetKeeperRepository
	battles  PetBattleRepository
	events   PetEventRepository
	seasons  PetSeasonRepository
	quests   PetQuestRepository
	market   PetMarketRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	// roll is a random number in [0, n).
	roll func(n int64) (int64, error)
}

// PetStores are where the pets are kept.
type PetStores struct {
	Pets    PetRepository
	Keepers PetKeeperRepository
	Battles PetBattleRepository
	Events  PetEventRepository
	Seasons PetSeasonRepository
	Quests  PetQuestRepository
	Market  PetMarketRepository
}

func NewPetService(users UserRepository, stores PetStores, wallets WalletRepository, tx TxManager, admins user.Admins) *PetService {
	return &PetService{
		userRepo: users, pets: stores.Pets, keepers: stores.Keepers, battles: stores.Battles,
		events: stores.Events, seasons: stores.Seasons, quests: stores.Quests, market: stores.Market,
		wallets: wallets, tx: tx, admins: admins, roll: cryptoRoll,
	}
}

// WithRoll makes the fights roll with roll: for tests.
func (s *PetService) WithRoll(roll func(n int64) (int64, error)) *PetService {
	s.roll = roll

	return s
}

// Shop is the pet shop.
func (s *PetService) Shop(ctx context.Context) (user.PetShop, error) {
	shop, err := s.pets.Shop(ctx)

	return shop, errs.Wrap("s.pets.Shop", err)
}

// Pets are the pets of userID, the oldest first.
func (s *PetService) Pets(ctx context.Context, userID uuid.UUID) ([]*user.Pet, error) {
	pets, err := s.pets.Pets(ctx, userID)

	return pets, errs.Wrap("s.pets.Pets", err)
}

// UserPets are the pets of nickname and their owner; user.ErrPetNotFound
// for a page deleted or banned at now.
func (s *PetService) UserPets(ctx context.Context, nickname user.Nickname, now time.Time) ([]*user.Pet, uuid.UUID, error) {
	owner, err := s.shownOwner(ctx, nickname, now)
	if err != nil {
		return nil, uuid.Nil, errs.Wrap("s.shownOwner", err)
	}

	pets, err := s.pets.Pets(ctx, owner.ID())
	if err != nil {
		return nil, uuid.Nil, errs.Wrap("s.pets.Pets", err)
	}

	return pets, owner.ID(), nil
}

// shownOwner is the user of nickname whose pet is shown at now.
func (s *PetService) shownOwner(ctx context.Context, nickname user.Nickname, now time.Time) (*user.User, error) {
	owner, err := s.userRepo.GetByNickname(ctx, nickname)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByNickname", err)
	}

	if owner.IsDeactivated() || owner.IsBannedAt(now) {
		return nil, user.ErrPetNotFound
	}

	return owner, nil
}

// spend takes votes of userID for pets, none when votes is 0.
func (s *PetService) spend(ctx context.Context, userID uuid.UUID, votes int64, note string) (int64, error) {
	if votes == 0 {
		return 0, nil
	}

	balance, err := s.wallets.Spend(ctx, userID, votes, user.Entry(user.VotePet, uuid.Nil, note))
	if err != nil {
		return 0, errs.Wrap("s.wallets.Spend", err)
	}

	return balance, errs.Wrap("s.pets.Count", s.pets.Count(ctx, votes, 0))
}

// Adopt gives userID a pet of kind called name at now for its price;
// user.ErrTooManyPets, user.ErrNotEnoughVotes.
func (s *PetService) Adopt(ctx context.Context, userID uuid.UUID, kind user.PetKind, name user.PetName, now time.Time) (PetCare, error) {
	var care PetCare

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		shop, err := s.pets.Shop(ctx)
		if err != nil {
			return errs.Wrap("s.pets.Shop", err)
		}

		// The pets are counted before the votes are taken, as a store
		// without transactions keeps what was done; AddPet counts them
		// again.
		pets, err := s.pets.Pets(ctx, userID)
		if err != nil {
			return errs.Wrap("s.pets.Pets", err)
		}
		if len(pets) >= user.MaxPets {
			return user.ErrTooManyPets
		}

		price, err := shop.Price(kind)
		if err != nil {
			return errs.Wrap("shop.Price", err)
		}

		if care.Balance, err = s.spend(ctx, userID, price, "adopt"); err != nil {
			return errs.Wrap("s.spend", err)
		}

		care.Pet = user.NewPet(kind, name, now.Truncate(time.Microsecond))

		return errs.Wrap("s.pets.AddPet", s.pets.AddPet(ctx, userID, care.Pet))
	})
	if err != nil {
		return PetCare{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return care, nil
}

// NamedPet says when the pet everyone is given opens and whether userID
// took it.
type NamedPet struct {
	Kind    user.PetKind
	OpensAt time.Time
	Claimed bool
}

// Big2Z is the state of the pet for userID.
func (s *PetService) Big2Z(ctx context.Context, userID uuid.UUID) (NamedPet, error) {
	claimed, err := s.pets.Granted(ctx, userID, user.PetBig2Z)
	if err != nil {
		return NamedPet{}, errs.Wrap("s.pets.Granted", err)
	}

	return NamedPet{Kind: user.PetBig2Z, OpensAt: user.PetBig2ZOpensAt, Claimed: claimed}, nil
}

// ClaimBig2Z gives userID the pet for free; user.ErrPetLocked before it
// opens, user.ErrPetClaimed the second time. It does not count against
// user.MaxPets, as everyone is to have it.
func (s *PetService) ClaimBig2Z(ctx context.Context, userID uuid.UUID, now time.Time) (*user.Pet, error) {
	if now.Before(user.PetBig2ZOpensAt) {
		return nil, user.ErrPetLocked
	}

	now = now.Truncate(time.Microsecond)
	pet := user.NewPet(user.PetBig2Z, user.PetBig2ZName, now)
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		return errs.Wrap("s.pets.GrantPet", s.pets.GrantPet(ctx, userID, pet, now))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return pet, nil
}

// Keeper is what userID keeps of the cases.
func (s *PetService) Keeper(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error) {
	k, err := s.keepers.Keeper(ctx, userID)

	return k, errs.Wrap("s.keepers.Keeper", err)
}

// OpenCase opens the case c for userID at now, for its price or for
// shards with withShards: the pet it gives waits to be taken or left.
// What user.PetKeeper.Opened refuses, user.ErrNotEnoughVotes.
func (s *PetService) OpenCase(ctx context.Context, userID uuid.UUID, c user.PetCase, withShards bool, now time.Time) (PetDrop, error) {
	rarity, err := s.roll(user.PetCaseOdds)
	if err != nil {
		return PetDrop{}, errs.Wrap("s.roll", err)
	}

	kind, err := s.roll(user.PetDiceRange)
	if err != nil {
		return PetDrop{}, errs.Wrap("s.roll", err)
	}

	var drop PetDrop

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		shop, err := s.pets.Shop(ctx)
		if err != nil {
			return errs.Wrap("s.pets.Shop", err)
		}

		k, err := s.keepers.LockKeeper(ctx, userID)
		if err != nil {
			return errs.Wrap("s.keepers.LockKeeper", err)
		}

		drop.CasesADay = shop.CasesADay()

		if drop.Keeper, err = k.Opened(c, withShards, shop.CasesADay(), rarity, kind, now); err != nil {
			return errs.Wrap("k.Opened", err)
		}

		if !withShards {
			if drop.Balance, err = s.spend(ctx, userID, shop.CasePrice(c), "case"); err != nil {
				return errs.Wrap("s.spend", err)
			}
		}

		if err = s.keepers.SaveKeeper(ctx, userID, drop.Keeper); err != nil {
			return errs.Wrap("s.keepers.SaveKeeper", err)
		}

		paid, err := s.did(ctx, userID, now, user.PetQuestCase)
		if paid > 0 {
			drop.Balance = paid
			// The task of the case may have brought shards.
			drop.Keeper, err = s.keepers.Keeper(ctx, userID)
		}

		return errs.Wrap("s.did", err)
	})
	if err != nil {
		return PetDrop{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return drop, nil
}

// TakeDrop takes the pet of the case of userID at now, called name;
// user.ErrNoPetDrop, user.ErrTooManyPets.
func (s *PetService) TakeDrop(ctx context.Context, userID uuid.UUID, name user.PetName, now time.Time) (*user.Pet, error) {
	var pet *user.Pet

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		k, err := s.keepers.LockKeeper(ctx, userID)
		if err != nil {
			return errs.Wrap("s.keepers.LockKeeper", err)
		}

		taken, kind, err := k.Took()
		if err != nil {
			return errs.Wrap("k.Took", err)
		}

		// As in Adopt: the pets are counted before anything is saved.
		pets, err := s.pets.Pets(ctx, userID)
		if err != nil {
			return errs.Wrap("s.pets.Pets", err)
		}
		if len(pets) >= user.MaxPets {
			return user.ErrTooManyPets
		}

		pet = user.NewPet(kind, name, now.Truncate(time.Microsecond))
		if err = s.pets.AddPet(ctx, userID, pet); err != nil {
			return errs.Wrap("s.pets.AddPet", err)
		}

		return errs.Wrap("s.keepers.SaveKeeper", s.keepers.SaveKeeper(ctx, userID, taken))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return pet, nil
}

// LeaveDrop leaves the pet of the case of userID for its shards;
// user.ErrNoPetDrop.
func (s *PetService) LeaveDrop(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error) {
	var left *user.PetKeeper

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		k, err := s.keepers.LockKeeper(ctx, userID)
		if err != nil {
			return errs.Wrap("s.keepers.LockKeeper", err)
		}

		if left, err = k.Left(); err != nil {
			return errs.Wrap("k.Left", err)
		}

		return errs.Wrap("s.keepers.SaveKeeper", s.keepers.SaveKeeper(ctx, userID, left))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return left, nil
}

// BuyItem buys item for the pet petID of userID and puts it on, for its
// price; user.ErrPetItemOwned, user.ErrNotEnoughVotes.
func (s *PetService) BuyItem(ctx context.Context, userID, petID uuid.UUID, item user.PetItem) (PetCare, error) {
	care, err := s.change(ctx, userID, petID, func(ctx context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		bought, err := p.Bought(item)
		if err != nil {
			return PetCare{}, errs.Wrap("p.Bought", err)
		}

		balance, err := s.spend(ctx, userID, item.Spec().Price, "clothes")

		return PetCare{Pet: bought, Balance: balance}, errs.Wrap("s.spend", err)
	})

	return care, errs.Wrap("s.change", err)
}

// Dress puts item of its wardrobe on the pet petID of userID, or takes
// it off; user.ErrPetItemMissing.
func (s *PetService) Dress(ctx context.Context, userID, petID uuid.UUID, item user.PetItem, wear bool) (*user.Pet, error) {
	care, err := s.change(ctx, userID, petID, func(_ context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		dressed, err := p.Dressed(item, wear)

		return PetCare{Pet: dressed}, errs.Wrap("p.Dressed", err)
	})

	return care.Pet, errs.Wrap("s.change", err)
}

// change does do to the pet petID of userID in a transaction and saves
// it.
func (s *PetService) change(ctx context.Context, userID, petID uuid.UUID, do func(ctx context.Context, p *user.Pet, shop user.PetShop) (PetCare, error)) (PetCare, error) {
	var care PetCare

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		shop, err := s.pets.Shop(ctx)
		if err != nil {
			return errs.Wrap("s.pets.Shop", err)
		}

		p, err := s.pets.LockPet(ctx, userID, petID)
		if err != nil {
			return errs.Wrap("s.pets.LockPet", err)
		}

		if care, err = do(ctx, p, shop); err != nil {
			return err
		}

		return errs.Wrap("s.pets.SavePet", s.pets.SavePet(ctx, care.Pet))
	})
	if err != nil {
		return PetCare{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return care, nil
}

// Feed feeds the pet petID of userID at now for the price of a meal;
// user.ErrPetFull, user.ErrNotEnoughVotes.
func (s *PetService) Feed(ctx context.Context, userID, petID uuid.UUID, now time.Time) (PetCare, error) {
	care, err := s.change(ctx, userID, petID, func(ctx context.Context, p *user.Pet, shop user.PetShop) (PetCare, error) {
		fed, err := p.Fed(now.Truncate(time.Microsecond))
		if err != nil {
			return PetCare{}, errs.Wrap("p.Fed", err)
		}

		balance, err := s.spend(ctx, userID, shop.Meal(), "feed")
		if err != nil {
			return PetCare{}, errs.Wrap("s.spend", err)
		}

		paid, err := s.did(ctx, userID, now, user.PetQuestFeed)

		return PetCare{Pet: fed, Balance: max(paid, balance)}, errs.Wrap("s.did", err)
	})

	return care, errs.Wrap("s.change", err)
}

// Play plays with the pet petID of userID at now; the balance is 0 unless
// a quest of the day paid. user.ErrPetTired.
func (s *PetService) Play(ctx context.Context, userID, petID uuid.UUID, now time.Time) (PetCare, error) {
	care, err := s.change(ctx, userID, petID, func(_ context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		played, err := p.Played(now.Truncate(time.Microsecond))
		if err != nil {
			return PetCare{}, errs.Wrap("p.Played", err)
		}

		paid, err := s.did(ctx, userID, now, user.PetQuestPlay)

		return PetCare{Pet: played, Balance: paid}, errs.Wrap("s.did", err)
	})

	return care, errs.Wrap("s.change", err)
}

// Rename calls the pet petID of userID name.
func (s *PetService) Rename(ctx context.Context, userID, petID uuid.UUID, name user.PetName) (*user.Pet, error) {
	care, err := s.change(ctx, userID, petID, func(_ context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		return PetCare{Pet: p.Renamed(name)}, nil
	})

	return care.Pet, errs.Wrap("s.change", err)
}

// Release lets the pet petID of userID go for its shards; its fights stay
// in the lists, the fight it leads is lost.
func (s *PetService) Release(ctx context.Context, userID, petID uuid.UUID) (*user.PetKeeper, error) {
	var keeper *user.PetKeeper

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.pets.LockPet(ctx, userID, petID)
		if err != nil {
			return errs.Wrap("s.pets.LockPet", err)
		}

		if err = s.pets.DeletePet(ctx, userID, petID); err != nil {
			return errs.Wrap("s.pets.DeletePet", err)
		}

		k, err := s.keepers.LockKeeper(ctx, userID)
		if err != nil {
			return errs.Wrap("s.keepers.LockKeeper", err)
		}
		keeper = k.Released(p.Kind())

		return errs.Wrap("s.keepers.SaveKeeper", s.keepers.SaveKeeper(ctx, userID, keeper))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return keeper, nil
}

// SetStance makes the pet petID of userID defend in stance.
func (s *PetService) SetStance(ctx context.Context, userID, petID uuid.UUID, stance user.PetStance) (*user.Pet, error) {
	care, err := s.change(ctx, userID, petID, func(_ context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		return PetCare{Pet: p.Stood(stance)}, nil
	})

	return care.Pet, errs.Wrap("s.change", err)
}

// defender is the owner and the pet the pet petID of userID fights: the
// best pet of nickname, or a pet near in rating with nickname empty.
func (s *PetService) defender(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, now time.Time) (owner, pet uuid.UUID, err error) {
	if nickname == "" {
		own, err := s.pets.Pet(ctx, userID, petID)
		if err != nil {
			return uuid.Nil, uuid.Nil, errs.Wrap("s.pets.Pet", err)
		}

		contenders, err := s.pets.Contenders(ctx, userID, own.Rating(), now, petContenders)
		if err != nil {
			return uuid.Nil, uuid.Nil, errs.Wrap("s.pets.Contenders", err)
		}

		pick, err := s.roll(user.PetDiceRange)
		if err != nil {
			return uuid.Nil, uuid.Nil, errs.Wrap("s.roll", err)
		}

		c, err := user.PickPetOpponent(own, contenders, pick)

		return c.Owner, c.Pet.ID(), errs.Wrap("user.PickPetOpponent", err)
	}

	u, err := s.shownOwner(ctx, nickname, now)
	if err != nil {
		return uuid.Nil, uuid.Nil, errs.Wrap("s.shownOwner", err)
	}
	if u.ID() == userID {
		return uuid.Nil, uuid.Nil, user.ErrPetFightSelf
	}

	pets, err := s.pets.Pets(ctx, u.ID())
	if err != nil {
		return uuid.Nil, uuid.Nil, errs.Wrap("s.pets.Pets", err)
	}

	var best *user.Pet
	for _, p := range pets {
		if best == nil || p.Rating() > best.Rating() {
			best = p
		}
	}
	if best == nil {
		return uuid.Nil, uuid.Nil, user.ErrPetNotFound
	}

	return u.ID(), best.ID(), nil
}

// dice are the random numbers of a fight.
func (s *PetService) dice() (user.PetDice, error) {
	dice := make(user.PetDice, user.PetDiceCount)
	for i := range dice {
		v, err := s.roll(user.PetDiceRange)
		if err != nil {
			return nil, errs.Wrap("s.roll", err)
		}
		dice[i] = v
	}

	return dice, nil
}

// petRef names a pet: its owner and its ID.
type petRef struct {
	owner, pet uuid.UUID
}

// lockPair locks the pets a and b in the order of their IDs, so that two
// fights of the same pets never wait for each other in a circle; b is nil
// when it is gone.
func (s *PetService) lockPair(ctx context.Context, a, b petRef) (*user.Pet, *user.Pet, error) {
	lock := func(r petRef) (*user.Pet, error) {
		p, err := s.pets.LockPet(ctx, r.owner, r.pet)
		if r == b && errors.Is(err, user.ErrPetNotFound) {
			return nil, nil
		}

		return p, errs.Wrap("s.pets.LockPet", err)
	}

	first, second := a, b
	if second.pet.String() < first.pet.String() {
		first, second = second, first
	}

	pf, err := lock(first)
	if err != nil {
		return nil, nil, err
	}

	ps, err := lock(second)
	if err != nil {
		return nil, nil, err
	}

	if first == a {
		return pf, ps, nil
	}

	return ps, pf, nil
}

// fightOf is what a fight needs: the opponent of the pet petID of userID
// and the dice.
func (s *PetService) fightOf(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, now time.Time) (petRef, user.PetDice, error) {
	owner, pet, err := s.defender(ctx, userID, petID, nickname, now)
	if err != nil {
		return petRef{}, nil, errs.Wrap("s.defender", err)
	}

	dice, err := s.dice()
	if err != nil {
		return petRef{}, nil, errs.Wrap("s.dice", err)
	}

	return petRef{owner: owner, pet: pet}, dice, nil
}

// challenge locks the pet petID of userID and the defender, refuses a pet
// in a fight and starts one in stance at now.
func (s *PetService) challenge(ctx context.Context, userID, petID uuid.UUID, defender petRef, stance user.PetStance, now time.Time) (*user.Pet, *user.Pet, *user.PetBattle, user.PetShop, error) {
	shop, err := s.pets.Shop(ctx)
	if err != nil {
		return nil, nil, nil, shop, errs.Wrap("s.pets.Shop", err)
	}

	attacker, d, err := s.lockPair(ctx, petRef{userID, petID}, defender)
	if err != nil {
		return nil, nil, nil, shop, errs.Wrap("s.lockPair", err)
	}
	if d == nil {
		return nil, nil, nil, shop, user.ErrNoPetOpponent
	}

	if _, err = s.battles.LockBattle(ctx, userID, petID); !errors.Is(err, user.ErrNoPetBattle) {
		if err == nil {
			err = user.ErrPetInBattle
		}

		return nil, nil, nil, shop, errs.Wrap("s.battles.LockBattle", err)
	}

	a, d, battle, err := attacker.Challenged(d, stance, shop, now)
	if err != nil {
		return nil, nil, nil, shop, errs.Wrap("attacker.Challenged", err)
	}

	return a, d, battle.Against(defender.owner), shop, nil
}

// settle saves the pets after the battle of userID is over, keeps the
// fight and pays a win.
func (s *PetService) settle(ctx context.Context, userID uuid.UUID, battle *user.PetBattle, shop user.PetShop, now time.Time) (PetFought, error) {
	attacker, defender, err := s.lockPair(ctx, petRef{userID, battle.PetID()}, petRef{battle.DefenderOwner(), battle.DefenderPet()})
	if err != nil {
		return PetFought{}, errs.Wrap("s.lockPair", err)
	}

	return s.saveFight(ctx, userID, battle, shop, attacker, defender, now)
}

// saveFight saves attacker and defender, as they were at the start of the
// battle, after it.
func (s *PetService) saveFight(ctx context.Context, userID uuid.UUID, battle *user.PetBattle, shop user.PetShop, attacker, defender *user.Pet, now time.Time) (PetFought, error) {
	a, d, err := battle.Settled(attacker, defender)
	if err != nil {
		return PetFought{}, errs.Wrap("battle.Settled", err)
	}

	if err = s.pets.SavePet(ctx, a); err != nil {
		return PetFought{}, errs.Wrap("s.pets.SavePet", err)
	}
	if d != nil {
		if err = s.pets.SavePet(ctx, d); err != nil {
			return PetFought{}, errs.Wrap("s.pets.SavePet", err)
		}
	}

	bout := battle.Bout(shop.Win())
	fought := PetFought{Pet: a, Fight: user.PetFight{ID: uuid.Must(uuid.NewV7()), AttackerID: userID, DefenderID: battle.DefenderOwner(), Bout: bout, At: now}}
	// The owner of the defender may have gone during a long fight.
	if err = s.pets.AddFight(ctx, fought.Fight); err != nil && !errors.Is(err, user.ErrNotFound) {
		return PetFought{}, errs.Wrap("s.pets.AddFight", err)
	}

	if bout.Reward == 0 {
		return fought, nil
	}

	wallet, err := s.wallets.Credit(ctx, userID, bout.Reward, 0, user.Entry(user.VotePet, battle.DefenderOwner(), "fight"))
	if err != nil {
		return PetFought{}, errs.Wrap("s.wallets.Credit", err)
	}
	fought.Balance = wallet.Votes

	return fought, errs.Wrap("s.pets.Count", s.pets.Count(ctx, 0, bout.Reward))
}

// Fight sets the pet petID of userID in stance on the best pet of
// nickname, or on a pet near in power with nickname empty, at now, and
// fights it to the end; a won fight brings votes. user.ErrPetNotFound,
// user.ErrPetFightSelf, user.ErrNoPetOpponent, user.ErrPetInBattle and
// what user.Pet.Challenged refuses.
func (s *PetService) Fight(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, stance user.PetStance, now time.Time) (PetFought, error) {
	now = now.Truncate(time.Microsecond)

	defender, dice, err := s.fightOf(ctx, userID, petID, nickname, now)
	if err != nil {
		return PetFought{}, errs.Wrap("s.fightOf", err)
	}

	var fought PetFought

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, d, battle, shop, err := s.challenge(ctx, userID, petID, defender, stance, now)
		if err != nil {
			return errs.Wrap("s.challenge", err)
		}

		if battle, err = battle.Moved(user.PetAuto, dice); err != nil {
			return errs.Wrap("battle.Moved", err)
		}

		if fought, err = s.saveFight(ctx, userID, battle, shop, a, d, now); err != nil {
			return errs.Wrap("s.saveFight", err)
		}

		return errs.Wrap("s.didFight", s.didFight(ctx, userID, &fought, now))
	})
	if err != nil {
		return PetFought{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return fought, nil
}

// didFight counts the fight, and more, towards the tasks of the day of
// userID, the votes they paid in the fight.
func (s *PetService) didFight(ctx context.Context, userID uuid.UUID, fought *PetFought, now time.Time, more ...user.PetQuestKind) error {
	kinds := append([]user.PetQuestKind{user.PetQuestFight}, more...)
	if fought.Fight.Bout.Won {
		kinds = append(kinds, user.PetQuestWin)
	}

	paid, err := s.did(ctx, userID, now, kinds...)
	if paid > 0 {
		fought.Balance = paid
	}

	return errs.Wrap("s.did", err)
}

// Battles are the fights the pets of userID lead a round after another.
func (s *PetService) Battles(ctx context.Context, userID uuid.UUID) ([]*user.PetBattle, error) {
	battles, err := s.battles.Battles(ctx, userID)

	return battles, errs.Wrap("s.battles.Battles", err)
}

// StartBattle sets the pet petID of userID in stance on an opponent as
// Fight does, but the owner leads the fight a round after another; the
// fight counts as lost until it is won. What Fight refuses.
func (s *PetService) StartBattle(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, stance user.PetStance, now time.Time) (PetBattleTurn, error) {
	now = now.Truncate(time.Microsecond)

	defender, _, err := s.fightOf(ctx, userID, petID, nickname, now)
	if err != nil {
		return PetBattleTurn{}, errs.Wrap("s.fightOf", err)
	}

	var turn PetBattleTurn

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		a, d, battle, _, err := s.challenge(ctx, userID, petID, defender, stance, now)
		if err != nil {
			return errs.Wrap("s.challenge", err)
		}

		if err = s.pets.SavePet(ctx, a); err != nil {
			return errs.Wrap("s.pets.SavePet", err)
		}
		if err = s.pets.SavePet(ctx, d); err != nil {
			return errs.Wrap("s.pets.SavePet", err)
		}

		turn = PetBattleTurn{Battle: battle, Pet: a}

		return errs.Wrap("s.battles.SaveBattle", s.battles.SaveBattle(ctx, userID, battle))
	})
	if err != nil {
		return PetBattleTurn{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return turn, nil
}

// MoveBattle makes move in the fight of the pet petID of userID at now;
// the last round settles it. user.ErrNoPetBattle.
func (s *PetService) MoveBattle(ctx context.Context, userID, petID uuid.UUID, move user.PetMove, now time.Time) (PetBattleTurn, error) {
	now = now.Truncate(time.Microsecond)

	dice, err := s.dice()
	if err != nil {
		return PetBattleTurn{}, errs.Wrap("s.dice", err)
	}

	var turn PetBattleTurn

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		battle, err := s.battles.LockBattle(ctx, userID, petID)
		if err != nil {
			return errs.Wrap("s.battles.LockBattle", err)
		}

		if turn.Battle, err = battle.Moved(move, dice); err != nil {
			return errs.Wrap("battle.Moved", err)
		}

		if !turn.Battle.Done() {
			if turn.Pet, err = s.pets.Pet(ctx, userID, petID); err != nil {
				return errs.Wrap("s.pets.Pet", err)
			}

			return errs.Wrap("s.battles.SaveBattle", s.battles.SaveBattle(ctx, userID, turn.Battle))
		}

		shop, err := s.pets.Shop(ctx)
		if err != nil {
			return errs.Wrap("s.pets.Shop", err)
		}

		fought, err := s.settle(ctx, userID, turn.Battle, shop, now)
		if err != nil {
			return errs.Wrap("s.settle", err)
		}
		if err = s.didFight(ctx, userID, &fought, now, user.PetQuestManual); err != nil {
			return errs.Wrap("s.didFight", err)
		}
		turn.Pet, turn.Fought = fought.Pet, &fought

		return errs.Wrap("s.battles.DeleteBattle", s.battles.DeleteBattle(ctx, userID, petID))
	})
	if err != nil {
		return PetBattleTurn{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return turn, nil
}

// Fights are the last fights of the pets of userID, on either side.
func (s *PetService) Fights(ctx context.Context, userID uuid.UUID) ([]user.PetFight, error) {
	fights, err := s.pets.Fights(ctx, userID, user.MaxPetFights)

	return fights, errs.Wrap("s.pets.Fights", err)
}

// Settings are the shop and its stats; user.ErrNotAdmin.
func (s *PetService) Settings(ctx context.Context, adminID uuid.UUID) (PetSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return PetSettings{}, errs.Wrap("checkAdmin", err)
	}

	shop, err := s.pets.Shop(ctx)
	if err != nil {
		return PetSettings{}, errs.Wrap("s.pets.Shop", err)
	}

	stats, err := s.pets.Stats(ctx)
	if err != nil {
		return PetSettings{}, errs.Wrap("s.pets.Stats", err)
	}

	return PetSettings{Shop: shop, Stats: stats}, nil
}

// SetShop changes the prices and the fights; user.ErrNotAdmin.
func (s *PetService) SetShop(ctx context.Context, adminID uuid.UUID, shop user.PetShop) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.pets.SetShop", s.pets.SetShop(ctx, shop))
}
