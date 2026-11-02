package fedi

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNoKeys = errors.New("an account needs its keys")

// KeyPair is the RSA key pair a page signs what it sends with, in PEM.
// The private one never leaves the service.
type KeyPair struct {
	Public  string
	Private string
}

// Account is a page that was opened to the Fediverse. Closed again, it
// keeps its keys: what it sends on closing is signed with them, and the
// servers that know it know them when it opens again.
type Account struct {
	userID    uuid.UUID
	keys      KeyPair
	open      bool
	createdAt time.Time
}

func NewAccount(userID uuid.UUID, keys KeyPair, now time.Time) (*Account, error) {
	if userID == uuid.Nil || keys.Public == "" || keys.Private == "" {
		return nil, ErrNoKeys
	}

	return &Account{
		userID:    userID,
		keys:      keys,
		open:      true,
		createdAt: now.UTC(),
	}, nil
}

func RestoreAccount(userID uuid.UUID, keys KeyPair, open bool, createdAt time.Time) *Account {
	return &Account{
		userID:    userID,
		keys:      keys,
		open:      open,
		createdAt: createdAt,
	}
}

// Open: the page is open to the Fediverse now.
func (a *Account) Open() bool {
	return a.open
}

// Opened is the account open again, with its keys.
func (a *Account) Opened() *Account {
	c := *a
	c.open = true

	return &c
}

// Closed is the account closed, keeping its keys.
func (a *Account) Closed() *Account {
	c := *a
	c.open = false

	return &c
}

func (a *Account) UserID() uuid.UUID {
	return a.userID
}

func (a *Account) Keys() KeyPair {
	return a.keys
}

func (a *Account) CreatedAt() time.Time {
	return a.createdAt
}
