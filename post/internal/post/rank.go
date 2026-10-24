package post

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/google/uuid"
)

const (
	// RankWindow is how far back the ranked news look ("Сначала
	// интересные"): the posts of the last week stand by how interesting
	// they are, the older ones follow newest first.
	RankWindow = 7 * 24 * time.Hour
	// AffinityPeriod is how far back the likes and comments of a reader
	// tell whom they care about.
	AffinityPeriod = 30 * 24 * time.Hour
	// MaxAffinity bounds what a reader's likes and comments of one source
	// add to its posts: a fan of one page still sees the others.
	MaxAffinity = 10
)

// Interest is what makes a post worth reading to one reader: what others
// did with it, what it holds, whose it is and how old it is.
type Interest struct {
	Counters Counters
	// Photos says whether the post shows any: people stop at pictures.
	Photos bool
	// Affinity is how many times in AffinityPeriod the reader liked or
	// commented on posts of the same source (the author, or the community
	// a post speaks for); their own posts count as MaxAffinity.
	Affinity int
	// Age is how long ago the post was written.
	Age time.Duration
}

// Score ranks posts as the news of Hacker News and Twitter do: points for
// likes, comments and reposts (a repost is worth the most, a like the
// least) and for the reader's affinity to the source, divided by the age
// raised to 1.5, so a fresh post with a few likes beats a popular one of
// three days ago. The same formula is in the SQL of the repository.
func (i Interest) Score() float64 {
	points := 1 + float64(i.Counters.Likes) + 2*float64(i.Counters.Comments) + 3*float64(i.Counters.Reposts) +
		2*float64(min(i.Affinity, MaxAffinity))
	if i.Photos {
		points++
	}

	hours := max(i.Age.Hours(), 0)

	return points / math.Pow(hours+2, 1.5)
}

// WindowStart is the smallest ID a post written at from or later can have:
// IDs are UUIDv7, whose first 48 bits are the milliseconds of their time.
func WindowStart(from time.Time) uuid.UUID {
	var id uuid.UUID
	ms := uint64(max(from.UnixMilli(), 0)) //nolint:gosec // checked not to be negative

	var b [8]byte
	binary.BigEndian.PutUint64(b[:], ms)
	copy(id[:6], b[2:])

	return id
}
