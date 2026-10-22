package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// MineRepository keeps the mine of the site and its miners.
type MineRepository interface {
	// Mine is user.DefaultMine until one is set.
	Mine(ctx context.Context) (user.Mine, error)
	SetMine(ctx context.Context, mine user.Mine) error
	// Miner is the miner userID, fresh if it never dug; in a transaction
	// it stays so until the end — digs and sales of one miner wait for
	// each other.
	Miner(ctx context.Context, userID uuid.UUID, fresh user.Miner) (user.Miner, error)
	SaveMiner(ctx context.Context, userID uuid.UUID, m user.Miner) error
	// CountDig adds a dig that found chest votes, CountSale — votes paid
	// for ore.
	CountDig(ctx context.Context, chest int64) error
	CountSale(ctx context.Context, votes int64) error
	Stats(ctx context.Context) (user.MineStats, error)
}

// MineView is the mine as a miner sees it at a time: its settings, the
// miner and how long its next point of energy takes.
type MineView struct {
	Mine       user.Mine
	Miner      user.Miner
	NextEnergy time.Duration
}

// Dug is what a dig gave and the mine after it; Balance is the votes of
// the miner after a chest, 0 without one.
type Dug struct {
	Haul    user.Haul
	View    MineView
	Balance int64
}

// Sold is what a sale gave: the units of ore sold, the votes, the balance
// and the mine after it.
type Sold struct {
	Units   int64
	Votes   int64
	Balance int64
	View    MineView
}

// MineSettings are the mine and how it does: for the administrators.
type MineSettings struct {
	Mine  user.Mine
	Stats user.MineStats
}

// MineService is the mine: people spend energy digging ore, which opens
// with experience, and sell the ore for votes out of the treasury.
type MineService struct {
	userRepo UserRepository
	mines    MineRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	// roll is a random number in [0, n).
	roll func(n int64) (int64, error)
}

func NewMineService(users UserRepository, mines MineRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *MineService {
	return &MineService{userRepo: users, mines: mines, wallets: wallets, tx: tx, admins: admins, roll: cryptoRoll}
}

// WithRoll makes the digs roll with roll: for tests.
func (s *MineService) WithRoll(roll func(n int64) (int64, error)) *MineService {
	s.roll = roll

	return s
}

func view(mine user.Mine, m user.Miner, now time.Time) MineView {
	return MineView{Mine: mine, Miner: m.At(mine, now), NextEnergy: m.NextEnergy(mine, now)}
}

// Mine is the mine of userID at now.
func (s *MineService) Mine(ctx context.Context, userID uuid.UUID, now time.Time) (MineView, error) {
	var got MineView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		mine, err := s.mines.Mine(ctx)
		if err != nil {
			return errs.Wrap("s.mines.Mine", err)
		}

		m, err := s.mines.Miner(ctx, userID, user.NewMiner(mine, now))
		if err != nil {
			return errs.Wrap("s.mines.Miner", err)
		}

		got = view(mine, m, now)

		return nil
	})
	if err != nil {
		return MineView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return got, nil
}

// rolls are the random numbers of a dig of vein.
func (s *MineService) rolls(vein user.OreVein, events user.MineEvents) (user.MineRolls, error) {
	var (
		rolls user.MineRolls
		err   error
	)

	if rolls.Event, err = s.roll(user.MineOdds); err != nil {
		return rolls, errs.Wrap("s.roll", err)
	}
	if rolls.Yield, err = s.roll(vein.Most - vein.Least + 1); err != nil {
		return rolls, errs.Wrap("s.roll", err)
	}
	if rolls.Chest, err = s.roll(events.ChestMost - events.ChestLeast + 1); err != nil {
		return rolls, errs.Wrap("s.roll", err)
	}

	return rolls, nil
}

// Dig digs ore for userID at now; a chest is paid at once.
// user.ErrUnknownOre, user.ErrOreLocked, user.ErrNoEnergy.
func (s *MineService) Dig(ctx context.Context, userID uuid.UUID, ore user.Ore, now time.Time) (Dug, error) {
	var dug Dug

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		mine, err := s.mines.Mine(ctx)
		if err != nil {
			return errs.Wrap("s.mines.Mine", err)
		}

		vein, err := mine.Vein(ore)
		if err != nil {
			return errs.Wrap("mine.Vein", err)
		}

		m, err := s.mines.Miner(ctx, userID, user.NewMiner(mine, now))
		if err != nil {
			return errs.Wrap("s.mines.Miner", err)
		}

		rolls, err := s.rolls(vein, mine.Events())
		if err != nil {
			return errs.Wrap("s.rolls", err)
		}

		if m, dug.Haul, err = m.Dig(mine, ore, now, rolls); err != nil {
			return errs.Wrap("m.Dig", err)
		}

		if err = s.mines.SaveMiner(ctx, userID, m); err != nil {
			return errs.Wrap("s.mines.SaveMiner", err)
		}

		if dug.Haul.Votes > 0 {
			wallet, err := s.wallets.Credit(ctx, userID, dug.Haul.Votes, 0, user.Entry(user.VoteMineChest, uuid.Nil, ore.String()))
			if err != nil {
				return errs.Wrap("s.wallets.Credit", err)
			}

			dug.Balance = wallet.Votes
		}

		dug.View = view(mine, m, now)

		return errs.Wrap("s.mines.CountDig", s.mines.CountDig(ctx, dug.Haul.Votes))
	})
	if err != nil {
		return Dug{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return dug, nil
}

// Sell sells the ore of userID at now, all of it or, with ore empty, of
// every ore that makes a whole vote; user.ErrMineNotYet for a page that
// may not sell yet, user.ErrNothingToSell.
func (s *MineService) Sell(ctx context.Context, userID uuid.UUID, ore user.Ore, now time.Time) (Sold, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return Sold{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = u.CheckSellAt(now); err != nil {
		return Sold{}, errs.Wrap("u.CheckSellAt", err)
	}

	var sold Sold

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		mine, err := s.mines.Mine(ctx)
		if err != nil {
			return errs.Wrap("s.mines.Mine", err)
		}

		m, err := s.mines.Miner(ctx, userID, user.NewMiner(mine, now))
		if err != nil {
			return errs.Wrap("s.mines.Miner", err)
		}

		if m, err = s.sell(ctx, userID, mine, m, ore, &sold); err != nil {
			return errs.Wrap("s.sell", err)
		}

		if err = s.mines.SaveMiner(ctx, userID, m); err != nil {
			return errs.Wrap("s.mines.SaveMiner", err)
		}

		sold.View = view(mine, m, now)

		return errs.Wrap("s.mines.CountSale", s.mines.CountSale(ctx, sold.Votes))
	})
	if err != nil {
		return Sold{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return sold, nil
}

// sell sells ore, or every ore with ore empty, and pays for each.
func (s *MineService) sell(ctx context.Context, userID uuid.UUID, mine user.Mine, m user.Miner, ore user.Ore, sold *Sold) (user.Miner, error) {
	ores := []user.Ore{ore}
	if ore == "" {
		ores = user.Ores
	}

	for _, o := range ores {
		next, units, votes, err := m.Sell(mine, o)
		if ore == "" && err != nil {
			continue
		}
		if err != nil {
			return user.Miner{}, errs.Wrap("m.Sell", err)
		}

		wallet, err := s.wallets.Credit(ctx, userID, votes, 0, user.Entry(user.VoteMine, uuid.Nil, o.String()))
		if err != nil {
			return user.Miner{}, errs.Wrap("s.wallets.Credit", err)
		}

		m = next
		sold.Units += units
		sold.Votes += votes
		sold.Balance = wallet.Votes
	}

	if sold.Votes == 0 {
		return user.Miner{}, user.ErrNothingToSell
	}

	return m, nil
}

// Settings are the mine and its stats; user.ErrNotAdmin.
func (s *MineService) Settings(ctx context.Context, adminID uuid.UUID) (MineSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return MineSettings{}, errs.Wrap("checkAdmin", err)
	}

	mine, err := s.mines.Mine(ctx)
	if err != nil {
		return MineSettings{}, errs.Wrap("s.mines.Mine", err)
	}

	stats, err := s.mines.Stats(ctx)
	if err != nil {
		return MineSettings{}, errs.Wrap("s.mines.Stats", err)
	}

	return MineSettings{Mine: mine, Stats: stats}, nil
}

// SetMine changes the ores, the energy and the events; user.ErrNotAdmin.
func (s *MineService) SetMine(ctx context.Context, adminID uuid.UUID, mine user.Mine) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.mines.SetMine", s.mines.SetMine(ctx, mine))
}
