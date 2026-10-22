package user

import (
	"crypto/rand"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const (
	// VoucherTokenLength is the length of a serial number, as on openvk.
	VoucherTokenLength = 24
	// MaxVoucherRating is the most percent of rating one voucher adds.
	MaxVoucherRating = 1_000_000
	// MaxVoucherUsages is the most accounts a limited voucher serves.
	MaxVoucherUsages = 1_000_000_000
	// UnlimitedUsages is a voucher any number of accounts may use (-1 in
	// openvk's form).
	UnlimitedUsages = -1

	voucherAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var (
	ErrVoucherTokenInvalid = errors.New("voucher serial number must be 24 letters and digits")
	// ErrVoucherEmpty: a voucher gives votes, rating or both.
	ErrVoucherEmpty         = errors.New("voucher must give votes or rating")
	ErrVoucherAmountInvalid = errors.New("a voucher gives at least 0 votes and at least 0 of rating")
	ErrVoucherUsagesInvalid = errors.New("voucher usages must be -1 or between 1 and 1000000000")
	// ErrVoucherUnlimitedVotes: votes are for a number of usages only.
	ErrVoucherUnlimitedVotes = errors.New("a voucher with votes has a limited number of usages")
	ErrVoucherExists         = errors.New("voucher with this serial number already exists")
	// ErrVoucherInvalid is all a user is told of a voucher that did not
	// work: there is none, it has run out or they have used it already
	// ("Ваучер недействительный" of openvk).
	ErrVoucherInvalid = errors.New("voucher is not valid")
	// ErrNotAdmin: vouchers are made by the administrators of the site.
	ErrNotAdmin = errors.New("only administrators can do this")
)

// VoucherToken is the serial number of a voucher: 24 Latin letters and
// digits, kept in upper case.
type VoucherToken string

// NewVoucherToken takes a serial number as people type it: in any case,
// with the dashes and spaces it is shown with.
func NewVoucherToken(s string) (VoucherToken, error) {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))

	if len(s) != VoucherTokenLength {
		return "", ErrVoucherTokenInvalid
	}

	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", ErrVoucherTokenInvalid
		}
	}

	return VoucherToken(s), nil
}

// GenerateVoucherToken makes up a serial number nobody can guess, of
// characters that are not mistaken for one another.
func GenerateVoucherToken() (VoucherToken, error) {
	random := make([]byte, VoucherTokenLength)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	token := make([]byte, VoucherTokenLength)
	for i, b := range random {
		// 32 characters: every one is as likely as another.
		token[i] = voucherAlphabet[int(b)%len(voucherAlphabet)]
	}

	return VoucherToken(token), nil
}

func (t VoucherToken) String() string {
	return string(t)
}

// Voucher is openvk's "Ваучер": a serial number that gives every account
// that enters it votes, rating or both, once per account and to a limited
// number of accounts.
type Voucher struct {
	id     uuid.UUID
	token  VoucherToken
	votes  int64
	rating int
	// usagesLeft is how many more accounts may use it; UnlimitedUsages
	// for any number.
	usagesLeft int
	// activations is how many accounts have used it.
	activations int
}

// NewVoucher makes a voucher that usages accounts may use
// (UnlimitedUsages: any number).
func NewVoucher(token VoucherToken, votes int64, rating, usages int) (*Voucher, error) {
	if votes < 0 || votes > MaxVotes || rating < 0 || rating > MaxVoucherRating {
		return nil, ErrVoucherAmountInvalid
	}

	if votes == 0 && rating == 0 {
		return nil, ErrVoucherEmpty
	}

	if usages != UnlimitedUsages && (usages < 1 || usages > MaxVoucherUsages) {
		return nil, ErrVoucherUsagesInvalid
	}

	// The treasury has to cover every usage of the votes in advance.
	if votes > 0 && usages == UnlimitedUsages {
		return nil, ErrVoucherUnlimitedVotes
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Voucher{id: id, token: token, votes: votes, rating: rating, usagesLeft: usages}, nil
}

// StoredVoucher is a voucher as storage keeps it, for RestoreVoucher.
type StoredVoucher struct {
	ID          uuid.UUID
	Token       VoucherToken
	Votes       int64
	Rating      int
	UsagesLeft  int
	Activations int
}

// RestoreVoucher rebuilds a voucher from storage.
func RestoreVoucher(s StoredVoucher) *Voucher {
	return &Voucher{
		id:          s.ID,
		token:       s.Token,
		votes:       s.Votes,
		rating:      s.Rating,
		usagesLeft:  s.UsagesLeft,
		activations: s.Activations,
	}
}

func (v *Voucher) ID() uuid.UUID       { return v.id }
func (v *Voucher) Token() VoucherToken { return v.token }
func (v *Voucher) Votes() int64        { return v.votes }
func (v *Voucher) Rating() int         { return v.rating }
func (v *Voucher) UsagesLeft() int     { return v.usagesLeft }
func (v *Voucher) Activations() int    { return v.activations }

// Expired: nobody else may use the voucher ("закончился").
func (v *Voucher) Expired() bool {
	return v.usagesLeft == 0
}

// Admins are the people who run the site, known by the email they sign
// in with: they make the vouchers.
type Admins map[Email]struct{}

// NewAdmins takes the emails as configured; one of a wrong form is an
// error rather than an administrator nobody can be.
func NewAdmins(emails []string) (Admins, error) {
	admins := make(Admins, len(emails))

	for _, raw := range emails {
		if strings.TrimSpace(raw) == "" {
			continue
		}

		email, err := NewEmail(raw)
		if err != nil {
			return nil, err
		}

		admins[Email(strings.ToLower(email.String()))] = struct{}{}
	}

	return admins, nil
}

// Has tells whether u runs the site; a deleted page does not.
func (a Admins) Has(u *User) bool {
	_, ok := a[Email(strings.ToLower(u.Email().String()))]

	return ok && !u.IsDeactivated()
}
