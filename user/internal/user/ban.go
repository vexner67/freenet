package user

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxBanReasonLength bounds why a page is blocked.
const maxBanReasonLength = 255

var (
	ErrBanReasonEmpty   = errors.New("a ban needs a reason")
	ErrBanReasonTooLong = errors.New("the reason of a ban is too long")
	// ErrBanUntilPast: a ban that ends at once blocks nothing.
	ErrBanUntilPast = errors.New("a ban must end in the future")
	// ErrBanSelf: an administrator does not lock themselves out.
	ErrBanSelf = errors.New("you cannot ban yourself")
	// ErrBanAdmin: the administrators of the site are not blocked; they
	// would lose the panel that lifts the block.
	ErrBanAdmin = errors.New("an administrator cannot be banned")
)

// BanReason is why the administrators blocked a page, as the blocked
// person and others read it: openvk's "Комментарий модератора".
type BanReason string

func NewBanReason(s string) (BanReason, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	if s == "" {
		return "", ErrBanReasonEmpty
	}

	if utf8.RuneCountInString(s) > maxBanReasonLength {
		return "", ErrBanReasonTooLong
	}

	return BanReason(s), nil
}

func (r BanReason) String() string {
	return string(r)
}

// Ban is a block of a page by the administrators of the site, openvk's
// "Заблокировать пользователя": for a while or, with a zero Until, for
// good. The zero value is a page that is not blocked.
type Ban struct {
	At     time.Time
	Until  time.Time
	Reason BanReason
}

// NewBan blocks a page at now until a time in the future, or for good
// with the zero time.
func NewBan(reason BanReason, now, until time.Time) (Ban, error) {
	if reason == "" {
		return Ban{}, ErrBanReasonEmpty
	}

	if !until.IsZero() && !until.After(now) {
		return Ban{}, ErrBanUntilPast
	}

	ban := Ban{At: now.UTC().Truncate(time.Microsecond), Reason: reason}
	if !until.IsZero() {
		ban.Until = until.UTC().Truncate(time.Microsecond)
	}

	return ban, nil
}

// Forever tells whether the ban never ends.
func (b Ban) Forever() bool {
	return b.Until.IsZero()
}

// ActiveAt tells whether the ban blocks the page at now: it was given
// and has not ended.
func (b Ban) ActiveAt(now time.Time) bool {
	return !b.At.IsZero() && (b.Forever() || now.Before(b.Until))
}

// BanAt is the ban that blocks u at now, the zero Ban if none does: an
// ended one blocks nothing.
func (u *User) BanAt(now time.Time) Ban {
	if !u.ban.ActiveAt(now) {
		return Ban{}
	}

	return u.ban
}

// Ban is the block u was given, ended or not, for storage.
func (u *User) Ban() Ban {
	return u.ban
}

// IsBannedAt tells whether u is blocked at now.
func (u *User) IsBannedAt(now time.Time) bool {
	return u.ban.ActiveAt(now)
}

// CheckBanBy: adminID may block u unless it is themselves or another
// administrator (isAdmin).
func (u *User) CheckBanBy(adminID uuid.UUID, isAdmin bool) error {
	switch {
	case u.id == adminID:
		return ErrBanSelf
	case isAdmin:
		return ErrBanAdmin
	default:
		return nil
	}
}

// WithBan is u blocked by ban, or let be with the zero Ban, for storage
// that keeps users whole.
func (u *User) WithBan(ban Ban) *User {
	changed := *u
	changed.ban = ban

	return &changed
}
