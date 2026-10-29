package user

import (
	"errors"

	"github.com/google/uuid"
)

// MaxVotes is the most votes one transfer or one voucher carries.
const MaxVotes = MaxMoved

var (
	ErrVotesInvalid = errors.New("the number of votes must be at least 1")
	// ErrNotEnoughVotes: the balance does not cover the gift or the
	// transfer.
	ErrNotEnoughVotes = errors.New("not enough votes")
	ErrTransferSelf   = errors.New("you cannot transfer votes to yourself")
	// ErrTransferRefused: either of the two has blacklisted the other.
	ErrTransferRefused = errors.New("you cannot transfer votes to this user")
)

// Votes is a number of openvk's "голоса", the currency gifts are priced
// in: what is spent or handed over at once, never zero.
type Votes int64

func NewVotes(n int64) (Votes, error) {
	if n < 1 || n > MaxVotes {
		return 0, ErrVotesInvalid
	}

	return Votes(n), nil
}

func (v Votes) Int64() int64 {
	return int64(v)
}

// Wallet is what vouchers gave a page and what is left of it: the votes
// ("Баланс") and the percent they added to its rating.
type Wallet struct {
	Votes  int64
	Rating int
}

func (u *User) Wallet() Wallet {
	return u.wallet
}

// WithWallet is u with another wallet, for storage that keeps users
// whole.
func (u *User) WithWallet(w Wallet) *User {
	changed := *u
	changed.wallet = w

	return &changed
}

// CheckTransferFrom tells whether senderID, tied to u by t, may hand u
// votes ("Перевод голосов"): not oneself, not a deleted page and not
// across a blacklist.
func (u *User) CheckTransferFrom(senderID uuid.UUID, t Ties) error {
	switch {
	case senderID == u.id:
		return ErrTransferSelf
	case u.IsDeactivated():
		return ErrDeactivated
	case t.Blocked:
		return ErrTransferRefused
	default:
		return nil
	}
}
