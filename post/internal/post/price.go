package post

import "errors"

// MaxPrice is the most votes an author asks for the photos and documents
// of a post.
const MaxPrice = 1_000_000

var (
	ErrInvalidPrice = errors.New("the price must be between 0 and 1000000 votes")
	// ErrPricedElsewhere: only a post on the author's own wall sells its
	// photos — the votes go to its author, who is the owner of the wall.
	ErrPricedElsewhere = errors.New("only a post on one's own wall may be paid")
	// ErrPriceWithoutMedia: a price is for photos or documents, which a
	// post without them does not have.
	ErrPriceWithoutMedia = errors.New("a paid post needs photos or documents")
)

// Price is how many votes the photos and documents of a post cost others
// ("Открыть за N голосов"); 0 shows them to everyone. The text is always
// free.
type Price int64

func NewPrice(votes int64) (Price, error) {
	if votes < 0 || votes > MaxPrice {
		return 0, ErrInvalidPrice
	}

	return Price(votes), nil
}

func (p Price) Int64() int64 {
	return int64(p)
}

// hasMedia tells whether c has what a price is asked for.
func (c Content) hasMedia() bool {
	return len(c.photos) > 0 || len(c.documents) > 0
}
