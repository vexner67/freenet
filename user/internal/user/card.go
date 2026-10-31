package user

import (
	"crypto/rand"
	"errors"
	"math/big"
	"slices"
	"time"
)

// CardBIN begins the number of every card of the site. No payment system
// issues numbers from it, so a card of the site is never taken for a real
// one.
const CardBIN = "1919"

const cardNumberLen = 16

// CardNumberTries is how many numbers issuing a card tries before giving
// up: with 10^11 numbers a second collision is next to impossible.
const CardNumberTries = 5

var (
	ErrCardCover  = errors.New("unknown cover of the card")
	ErrCardNumber = errors.New("invalid number of the card")
	// ErrNoCard: the page has not been issued a card yet.
	ErrNoCard = errors.New("the page has no card")
	// ErrCardNumberTaken: another card has the number.
	ErrCardNumberTaken = errors.New("the number of the card is taken")
)

// CardCover is the picture of a card. The site draws each by its name.
type CardCover string

// DefaultCardCover is the cover a new card has.
const DefaultCardCover CardCover = "classic"

var cardCovers = []CardCover{
	DefaultCardCover, "gold", "platinum", "black", "flame", "ocean", "matrix", "glitter", "chrome", "sky",
}

// NewCardCover is ErrCardCover for a cover the site does not have.
func NewCardCover(s string) (CardCover, error) {
	c := CardCover(s)
	if !slices.Contains(cardCovers, c) {
		return "", ErrCardCover
	}

	return c, nil
}

func (c CardCover) String() string { return string(c) }

// CardNumber is the sixteen digits of a card: CardBIN, the random ones and
// the check digit of the Luhn algorithm, as on a real card.
type CardNumber string

// NewCardNumber is ErrCardNumber unless s is such a number.
func NewCardNumber(s string) (CardNumber, error) {
	if len(s) != cardNumberLen || s[:len(CardBIN)] != CardBIN {
		return "", ErrCardNumber
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "", ErrCardNumber
		}
	}
	if s[cardNumberLen-1] != checkDigit(s[:cardNumberLen-1]) {
		return "", ErrCardNumber
	}

	return CardNumber(s), nil
}

// RandomCardNumber draws a number.
func RandomCardNumber() (CardNumber, error) {
	body := []byte(CardBIN)
	for len(body) < cardNumberLen-1 {
		digit, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err //nolint:wrapcheck // a domain package has no errs
		}
		body = append(body, byte('0'+digit.Int64()))
	}

	return CardNumber(append(body, checkDigit(string(body)))), nil
}

// checkDigit is the digit that makes body plus it pass the Luhn check.
func checkDigit(body string) byte {
	sum := 0
	for i := range len(body) {
		d := int(body[len(body)-1-i] - '0')
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}

	return byte('0' + (10-sum%10)%10)
}

func (n CardNumber) String() string { return string(n) }

// BankCard is the virtual card of a page. It holds no money of its own: it is
// the face of the account.
type BankCard struct {
	number   CardNumber
	cover    CardCover
	issuedAt time.Time
}

// NewBankCard is a card with the default cover, issued at now.
func NewBankCard(number CardNumber, now time.Time) BankCard {
	return BankCard{number: number, cover: DefaultCardCover, issuedAt: now}
}

// RestoreBankCard is a card from the storage.
func RestoreBankCard(number CardNumber, cover CardCover, issuedAt time.Time) BankCard {
	return BankCard{number: number, cover: cover, issuedAt: issuedAt}
}

func (c BankCard) Number() CardNumber  { return c.number }
func (c BankCard) Cover() CardCover    { return c.cover }
func (c BankCard) IssuedAt() time.Time { return c.issuedAt }
