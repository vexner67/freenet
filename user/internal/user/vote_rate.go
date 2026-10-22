package user

import "errors"

// MaxVoteRate is the dearest a vote may be said to be, in kopecks: the
// bound of the arithmetic, not of the market.
const MaxVoteRate = MaxAmount

// DefaultVoteRate is what a vote is worth until the administrators say
// otherwise: 7 ₽, as VK sold its votes.
const DefaultVoteRate VoteRate = 700

var ErrVoteRateInvalid = errors.New("the rate of a vote must be at least 1 kopeck")

// VoteRate is what one vote is worth in rubles, in kopecks: what the site
// tells people their votes cost. Nothing is sold through the site; the
// administrators hand votes out with vouchers at this rate.
type VoteRate int64

func NewVoteRate(kopecks int64) (VoteRate, error) {
	if kopecks < 1 || kopecks > MaxVoteRate {
		return 0, ErrVoteRateInvalid
	}

	return VoteRate(kopecks), nil
}

func (r VoteRate) Kopecks() int64 {
	return int64(r)
}
