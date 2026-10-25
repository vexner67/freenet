package service

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// MaxDueTables are the tables a sweep moves on at most.
const MaxDueTables = 100

// MaxListedTables are the tables of a section of the lobby.
const MaxListedTables = 50

// TableSection is a part of the lobby of the tables: the public ones with
// players, those the viewer sits at and those they are invited to.
type TableSection string

const (
	TablesPublic  TableSection = "public"
	TablesMine    TableSection = "mine"
	TablesInvited TableSection = "invited"
)

var ErrTableSection = errors.New("unknown section of poker tables")

// HoldemRepository keeps the tables of "Покер".
type HoldemRepository interface {
	AddTable(ctx context.Context, t *user.HoldemTable) error
	// Table and LockTable (FOR UPDATE, in a transaction) return
	// user.ErrTableNotFound.
	Table(ctx context.Context, id uuid.UUID) (*user.HoldemTable, error)
	LockTable(ctx context.Context, id uuid.UUID) (*user.HoldemTable, error)
	SaveTable(ctx context.Context, t *user.HoldemTable) error
	DeleteTable(ctx context.Context, id uuid.UUID) error
	// LockFree is a public table of bigBlind with a free seat userID does
	// not sit at, the fullest first, locked and skipping locked ones;
	// user.ErrTableNotFound.
	LockFree(ctx context.Context, bigBlind int64, userID uuid.UUID) (*user.HoldemTable, error)
	// Tables are the tables of a section for userID, at most
	// MaxListedTables.
	Tables(ctx context.Context, section TableSection, userID uuid.UUID) ([]*user.HoldemTable, error)
	// Due are the tables due by now, at most limit.
	Due(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
}

// NewTableInput is a table a player makes and sits at with BuyIn chips.
type NewTableInput struct {
	Name     string
	Private  bool
	Seats    int
	BigBlind int64
	BuyIn    int64
}

// HoldemView is a table after a move and, when the votes of the player
// changed, their balance (-1 when not).
type HoldemView struct {
	Table   *user.HoldemTable
	Balance int64
}

// HoldemService is the tables of "Покер": Texas Hold'em of people.
type HoldemService struct {
	userRepo UserRepository
	tables   HoldemRepository
	wallets  WalletRepository
	tx       TxManager
	roll     func(n int64) (int64, error)
	now      func() time.Time
}

func NewHoldemService(users UserRepository, tables HoldemRepository, wallets WalletRepository, tx TxManager) *HoldemService {
	return &HoldemService{userRepo: users, tables: tables, wallets: wallets, tx: tx, roll: cryptoRoll, now: time.Now}
}

// WithRoll makes the decks shuffle with roll and the clock tell now: for
// tests.
func (s *HoldemService) WithRoll(roll func(n int64) (int64, error), now func() time.Time) *HoldemService {
	s.roll, s.now = roll, now

	return s
}

// shuffle is a full deck shuffled; a failure of the randomness leaves it
// in order, which the table does not deal in practice (crypto/rand does
// not fail).
func (s *HoldemService) shuffle() []user.Card {
	deck := make([]user.Card, user.DeckSize)
	for i := range deck {
		deck[i] = user.Card(i)
	}

	for i := range user.DeckSize - 1 {
		j, err := s.roll(int64(user.DeckSize - i))
		if err != nil {
			return deck
		}
		deck[i], deck[i+int(j)] = deck[i+int(j)], deck[i]
	}

	return deck
}

// Tables are the tables of a section of the lobby; ErrTableSection.
func (s *HoldemService) Tables(ctx context.Context, userID uuid.UUID, section TableSection) ([]*user.HoldemTable, error) {
	if section != TablesPublic && section != TablesMine && section != TablesInvited {
		return nil, ErrTableSection
	}
	if userID == uuid.Nil && section != TablesPublic {
		return nil, nil
	}

	tables, err := s.tables.Tables(ctx, section, userID)

	return tables, errs.Wrap("s.tables.Tables", err)
}

// Create makes a table of userID and sits them at it, the buy-in paid;
// user.ErrTableName, user.ErrTableSeats, user.ErrTableBlind,
// user.ErrTableBuyIn, user.ErrNotEnoughVotes.
func (s *HoldemService) Create(ctx context.Context, userID uuid.UUID, in NewTableInput) (HoldemView, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return HoldemView{}, errs.Wrap("uuid.NewV7", err)
	}

	now := s.now()

	table, err := user.NewHoldemTable(id, userID, in.Name, in.Private, in.Seats, in.BigBlind, now)
	if err != nil {
		return HoldemView{}, errs.Wrap("user.NewHoldemTable", err)
	}

	view := HoldemView{Table: table}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := table.Sit(userID, -1, in.BuyIn, now); err != nil {
			return errs.Wrap("table.Sit", err)
		}

		if view.Balance, err = s.buyIn(ctx, userID, in.BuyIn); err != nil {
			return errs.Wrap("s.buyIn", err)
		}

		return errs.Wrap("s.tables.AddTable", s.tables.AddTable(ctx, table))
	})
	if err != nil {
		return HoldemView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// Quick sits userID at a public table of bigBlind with a free seat, a new
// one when there is none; user.ErrTableStake, user.ErrTableBuyIn,
// user.ErrNotEnoughVotes.
func (s *HoldemService) Quick(ctx context.Context, userID uuid.UUID, bigBlind, buyIn int64) (HoldemView, error) {
	if !slices.Contains(user.TableStakes, bigBlind) {
		return HoldemView{}, user.ErrTableStake
	}

	var view HoldemView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		now := s.now()

		table, err := s.tables.LockFree(ctx, bigBlind, userID)
		fresh := errors.Is(err, user.ErrTableNotFound)

		switch {
		case fresh:
			id, err := uuid.NewV7()
			if err != nil {
				return errs.Wrap("uuid.NewV7", err)
			}
			if table, err = user.NewHoldemTable(id, userID, "", false, user.DefaultTableSeats, bigBlind, now); err != nil {
				return errs.Wrap("user.NewHoldemTable", err)
			}
		case err != nil:
			return errs.Wrap("s.tables.LockFree", err)
		default:
			if err = s.moveOn(ctx, table, now); err != nil {
				return errs.Wrap("s.moveOn", err)
			}
		}

		if err = table.Sit(userID, -1, buyIn, now); err != nil {
			return errs.Wrap("table.Sit", err)
		}

		if view.Balance, err = s.buyIn(ctx, userID, buyIn); err != nil {
			return errs.Wrap("s.buyIn", err)
		}

		view.Table = table
		if fresh {
			return errs.Wrap("s.tables.AddTable", s.tables.AddTable(ctx, table))
		}

		return errs.Wrap("s.tables.SaveTable", s.tables.SaveTable(ctx, table))
	})
	if err != nil {
		return HoldemView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// buyIn takes the chips of a player from their votes into the Treasury
// and is their balance after it.
func (s *HoldemService) buyIn(ctx context.Context, userID uuid.UUID, chips int64) (int64, error) {
	balance, err := s.wallets.Spend(ctx, userID, chips, user.Entry(user.VotePoker, uuid.Nil, "table"))

	return balance, errs.Wrap("s.wallets.Spend", err)
}

// moveOn brings a locked table to now and pays those it stood up.
func (s *HoldemService) moveOn(ctx context.Context, table *user.HoldemTable, now time.Time) error {
	payouts, _ := table.At(now, s.shuffle)

	_, err := s.payOut(ctx, uuid.Nil, payouts)

	return errs.Wrap("s.payOut", err)
}

// payOut pays back the chips of the players who left, from the Treasury;
// the balance is of userID if among them, -1 if not. A page erased since
// loses its chips.
func (s *HoldemService) payOut(ctx context.Context, userID uuid.UUID, payouts []user.TablePayout) (int64, error) {
	balance := int64(-1)

	for _, p := range payouts {
		if p.Chips <= 0 {
			continue
		}

		wallet, err := s.wallets.Credit(ctx, p.UserID, p.Chips, 0, user.Entry(user.VotePoker, uuid.Nil, "table"))
		if errors.Is(err, user.ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, errs.Wrap("s.wallets.Credit", err)
		}
		if p.UserID == userID {
			balance = wallet.Votes
		}
	}

	return balance, nil
}

// visible tells whether viewerID may see the table: a private one only
// those who sit at it, own it or are invited.
func visible(table *user.HoldemTable, viewerID uuid.UUID) bool {
	return table.IsInvited(viewerID) || table.SeatOf(viewerID) >= 0
}

// Table is the table id at now for viewerID; it is moved on and kept only
// when it changed, so that watching it costs a read; user.ErrTableNotFound.
func (s *HoldemService) Table(ctx context.Context, viewerID, id uuid.UUID) (*user.HoldemTable, error) {
	table, err := s.tables.Table(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.tables.Table", err)
	}
	if !visible(table, viewerID) {
		return nil, user.ErrTableNotFound
	}

	if due := table.Due(); due.IsZero() || s.now().Before(due) {
		return table, nil
	}

	view, err := s.change(ctx, uuid.Nil, id, nothing)
	if err != nil {
		return nil, errs.Wrap("s.change", err)
	}

	return view.Table, nil
}

// nothing is no move: the table is only brought to now.
func nothing(*user.HoldemTable, time.Time) ([]user.TablePayout, error) {
	return nil, nil
}

// change moves the locked table id on to now, does play on it, pays out
// who left and keeps it — or deletes it once empty.
func (s *HoldemService) change(ctx context.Context, userID, id uuid.UUID, play func(*user.HoldemTable, time.Time) ([]user.TablePayout, error)) (HoldemView, error) {
	view := HoldemView{Balance: -1}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		table, err := s.tables.LockTable(ctx, id)
		if err != nil {
			return errs.Wrap("s.tables.LockTable", err)
		}

		now := s.now()
		payouts, _ := table.At(now, s.shuffle)

		left, err := play(table, now)
		if err != nil {
			return errs.Wrap("play", err)
		}

		view.Table = table
		payouts = append(payouts, left...)

		if view.Balance, err = s.payOut(ctx, userID, payouts); err != nil {
			return errs.Wrap("s.payOut", err)
		}

		if table.Empty() {
			return errs.Wrap("s.tables.DeleteTable", s.tables.DeleteTable(ctx, id))
		}

		return errs.Wrap("s.tables.SaveTable", s.tables.SaveTable(ctx, table))
	})
	if err != nil {
		return HoldemView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// Sit puts userID at the table id on seat (-1 — any), the buy-in paid;
// user.ErrTableNotFound, user.ErrTableSeated, user.ErrTableNotInvited,
// user.ErrTableBuyIn, user.ErrTableFull, user.ErrTableSeatTaken,
// user.ErrNotEnoughVotes.
func (s *HoldemService) Sit(ctx context.Context, userID, id uuid.UUID, seat int, buyIn int64) (HoldemView, error) {
	var balance int64

	view, err := s.change(ctx, userID, id, func(table *user.HoldemTable, now time.Time) ([]user.TablePayout, error) {
		if !visible(table, userID) {
			return nil, user.ErrTableNotFound
		}
		if err := table.Sit(userID, seat, buyIn, now); err != nil {
			return nil, errs.Wrap("table.Sit", err)
		}

		var err error
		balance, err = s.buyIn(ctx, userID, buyIn)

		return nil, errs.Wrap("s.buyIn", err)
	})
	if err != nil {
		return HoldemView{}, errs.Wrap("s.change", err)
	}

	view.Balance = balance

	return view, nil
}

// Leave stands userID up with their chips paid back; the table without
// players closes and is nil then; user.ErrTableNotFound,
// user.ErrTableNotSeated.
func (s *HoldemService) Leave(ctx context.Context, userID, id uuid.UUID) (HoldemView, error) {
	view, err := s.change(ctx, userID, id, func(table *user.HoldemTable, now time.Time) ([]user.TablePayout, error) {
		payout, err := table.Leave(userID, now)

		return []user.TablePayout{payout}, errs.Wrap("table.Leave", err)
	})
	if err != nil {
		return HoldemView{}, errs.Wrap("s.change", err)
	}

	if view.Balance < 0 {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return HoldemView{}, errs.Wrap("s.userRepo.GetByID", err)
		}
		view.Balance = u.Wallet().Votes
	}
	if view.Table.Empty() {
		view.Table = nil
	}

	return view, nil
}

// Act is the move of userID at the table id; user.ErrTableNotFound,
// user.ErrTableNotSeated, user.ErrTableNotYourTurn, user.ErrTableAction,
// user.ErrTableRaise.
func (s *HoldemService) Act(ctx context.Context, userID, id uuid.UUID, action user.TableAction, amount int64) (*user.HoldemTable, error) {
	view, err := s.change(ctx, userID, id, func(table *user.HoldemTable, now time.Time) ([]user.TablePayout, error) {
		return nil, errs.Wrap("table.Act", table.Act(userID, action, amount, now))
	})
	if err != nil {
		return nil, errs.Wrap("s.change", err)
	}

	return view.Table, nil
}

// Back brings userID back from sitting out; user.ErrTableNotFound,
// user.ErrTableNotSeated.
func (s *HoldemService) Back(ctx context.Context, userID, id uuid.UUID) (*user.HoldemTable, error) {
	view, err := s.change(ctx, userID, id, func(table *user.HoldemTable, now time.Time) ([]user.TablePayout, error) {
		return nil, errs.Wrap("table.Back", table.Back(userID, now))
	})
	if err != nil {
		return nil, errs.Wrap("s.change", err)
	}

	return view.Table, nil
}

// Invite invites the people of nicknames to the table id; the unknown are
// skipped; user.ErrTableNotFound, user.ErrTableNotSeated,
// user.ErrTableInvites.
func (s *HoldemService) Invite(ctx context.Context, userID, id uuid.UUID, nicknames []user.Nickname) (*user.HoldemTable, error) {
	var ids []uuid.UUID
	for _, n := range nicknames {
		u, err := s.userRepo.GetByNickname(ctx, n)
		if errors.Is(err, user.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap("s.userRepo.GetByNickname", err)
		}
		if !u.IsDeactivated() {
			ids = append(ids, u.ID())
		}
	}

	view, err := s.change(ctx, userID, id, func(table *user.HoldemTable, _ time.Time) ([]user.TablePayout, error) {
		return nil, errs.Wrap("table.Invite", table.Invite(userID, ids))
	})
	if err != nil {
		return nil, errs.Wrap("s.change", err)
	}

	return view.Table, nil
}

// SweepDue moves on the tables due that nobody looks at: turns pass,
// hands are dealt, players away stand up; a failure of one table is
// skipped. The count is of the tables moved.
func (s *HoldemService) SweepDue(ctx context.Context) (int, error) {
	ids, err := s.tables.Due(ctx, s.now(), MaxDueTables)
	if err != nil {
		return 0, errs.Wrap("s.tables.Due", err)
	}

	moved := 0
	var first error
	for _, id := range ids {
		_, err := s.change(ctx, uuid.Nil, id, nothing)
		if err != nil && !errors.Is(err, user.ErrTableNotFound) {
			first = cmp.Or(first, err)
			continue
		}
		moved++
	}

	return moved, errs.Wrap("s.change", first)
}
