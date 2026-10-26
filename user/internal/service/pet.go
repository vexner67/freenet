package service

import (
	"context"
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
	SavePet(ctx context.Context, p *user.Pet) error
	DeletePet(ctx context.Context, userID, petID uuid.UUID) error
	// Opponent is a pet near rating and its owner, one of the nearest at
	// random, of someone not userID and neither deleted nor banned at
	// now; user.ErrNoPetOpponent.
	Opponent(ctx context.Context, userID uuid.UUID, rating int, now time.Time) (owner, pet uuid.UUID, err error)
	AddFight(ctx context.Context, f user.PetFight) error
	// Fights are those userID fought on either side, newest first.
	Fights(ctx context.Context, userID uuid.UUID, limit int) ([]user.PetFight, error)
	// Count adds votes spent on pets and won by them.
	Count(ctx context.Context, spent, won int64) error
	Stats(ctx context.Context) (user.PetShopStats, error)
}

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
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	// roll is a random number in [0, n).
	roll func(n int64) (int64, error)
}

func NewPetService(users UserRepository, pets PetRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *PetService {
	return &PetService{userRepo: users, pets: pets, wallets: wallets, tx: tx, admins: admins, roll: cryptoRoll}
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

		if care.Balance, err = s.spend(ctx, userID, shop.Price(kind), "adopt"); err != nil {
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

		return PetCare{Pet: fed, Balance: balance}, errs.Wrap("s.spend", err)
	})

	return care, errs.Wrap("s.change", err)
}

// Play plays with the pet petID of userID at now; user.ErrPetTired.
func (s *PetService) Play(ctx context.Context, userID, petID uuid.UUID, now time.Time) (*user.Pet, error) {
	care, err := s.change(ctx, userID, petID, func(_ context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		played, err := p.Played(now.Truncate(time.Microsecond))

		return PetCare{Pet: played}, errs.Wrap("p.Played", err)
	})

	return care.Pet, errs.Wrap("s.change", err)
}

// Rename calls the pet petID of userID name.
func (s *PetService) Rename(ctx context.Context, userID, petID uuid.UUID, name user.PetName) (*user.Pet, error) {
	care, err := s.change(ctx, userID, petID, func(_ context.Context, p *user.Pet, _ user.PetShop) (PetCare, error) {
		return PetCare{Pet: p.Renamed(name)}, nil
	})

	return care.Pet, errs.Wrap("s.change", err)
}

// Release lets the pet petID of userID go; its fights stay in the lists.
func (s *PetService) Release(ctx context.Context, userID, petID uuid.UUID) error {
	return errs.Wrap("s.pets.DeletePet", s.pets.DeletePet(ctx, userID, petID))
}

// defender is the owner and the pet the pet petID of userID fights: the
// best pet of nickname, or a pet near in rating with nickname empty.
func (s *PetService) defender(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, now time.Time) (owner, pet uuid.UUID, err error) {
	if nickname == "" {
		own, err := s.pets.Pet(ctx, userID, petID)
		if err != nil {
			return uuid.Nil, uuid.Nil, errs.Wrap("s.pets.Pet", err)
		}

		owner, pet, err = s.pets.Opponent(ctx, userID, own.Rating(), now)

		return owner, pet, errs.Wrap("s.pets.Opponent", err)
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
// fights of the same pets never wait for each other in a circle.
func (s *PetService) lockPair(ctx context.Context, a, b petRef) (*user.Pet, *user.Pet, error) {
	first, second := a, b
	if second.pet.String() < first.pet.String() {
		first, second = second, first
	}

	pf, err := s.pets.LockPet(ctx, first.owner, first.pet)
	if err != nil {
		return nil, nil, errs.Wrap("s.pets.LockPet", err)
	}

	ps, err := s.pets.LockPet(ctx, second.owner, second.pet)
	if err != nil {
		return nil, nil, errs.Wrap("s.pets.LockPet", err)
	}

	if first == a {
		return pf, ps, nil
	}

	return ps, pf, nil
}

// Fight sets the pet petID of userID on the best pet of nickname, or on
// a pet near in rating with nickname empty, at now; a won fight brings
// votes. user.ErrPetNotFound, user.ErrPetFightSelf, user.ErrNoPetOpponent
// and what user.Pet.Fought refuses.
func (s *PetService) Fight(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, now time.Time) (PetFought, error) {
	now = now.Truncate(time.Microsecond)

	defenderID, defenderPet, err := s.defender(ctx, userID, petID, nickname, now)
	if err != nil {
		return PetFought{}, errs.Wrap("s.defender", err)
	}

	dice, err := s.dice()
	if err != nil {
		return PetFought{}, errs.Wrap("s.dice", err)
	}

	var fought PetFought

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		shop, err := s.pets.Shop(ctx)
		if err != nil {
			return errs.Wrap("s.pets.Shop", err)
		}

		attacker, defender, err := s.lockPair(ctx, petRef{userID, petID}, petRef{defenderID, defenderPet})
		if err != nil {
			return errs.Wrap("s.lockPair", err)
		}

		a, d, bout, err := attacker.Fought(defender, shop, now, dice)
		if err != nil {
			return errs.Wrap("attacker.Fought", err)
		}

		if err = s.pets.SavePet(ctx, a); err != nil {
			return errs.Wrap("s.pets.SavePet", err)
		}
		if err = s.pets.SavePet(ctx, d); err != nil {
			return errs.Wrap("s.pets.SavePet", err)
		}

		fought.Pet = a
		fought.Fight = user.PetFight{ID: uuid.Must(uuid.NewV7()), AttackerID: userID, DefenderID: defenderID, Bout: bout, At: now}
		if err = s.pets.AddFight(ctx, fought.Fight); err != nil {
			return errs.Wrap("s.pets.AddFight", err)
		}

		if !bout.Won || shop.Win() == 0 {
			return nil
		}

		wallet, err := s.wallets.Credit(ctx, userID, shop.Win(), 0, user.Entry(user.VotePet, defenderID, "fight"))
		if err != nil {
			return errs.Wrap("s.wallets.Credit", err)
		}
		fought.Balance = wallet.Votes

		return errs.Wrap("s.pets.Count", s.pets.Count(ctx, 0, shop.Win()))
	})
	if err != nil {
		return PetFought{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return fought, nil
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
