package post_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/post/internal/post"
)

func TestInterest_Score(t *testing.T) {
	t.Parallel()

	fresh := post.Interest{Counters: post.Counters{Likes: 2}, Age: time.Hour}
	old := post.Interest{Counters: post.Counters{Likes: 20}, Age: 72 * time.Hour}
	if fresh.Score() <= old.Score() {
		t.Fatalf("a fresh post %v loses to an old popular one %v", fresh.Score(), old.Score())
	}

	liked := post.Interest{Counters: post.Counters{Likes: 3}, Age: time.Hour}
	commented := post.Interest{Counters: post.Counters{Comments: 3}, Age: time.Hour}
	shared := post.Interest{Counters: post.Counters{Reposts: 3}, Age: time.Hour}
	if !(liked.Score() < commented.Score() && commented.Score() < shared.Score()) {
		t.Fatalf("likes %v, comments %v, reposts %v", liked.Score(), commented.Score(), shared.Score())
	}

	plain := post.Interest{Age: time.Hour}
	if (post.Interest{Age: time.Hour, Photos: true}).Score() <= plain.Score() {
		t.Fatal("photos add nothing")
	}
	if (post.Interest{Age: time.Hour, Affinity: 3}).Score() <= plain.Score() {
		t.Fatal("affinity adds nothing")
	}
	if (post.Interest{Age: time.Hour, Affinity: 1000}).Score() != (post.Interest{Age: time.Hour, Affinity: post.MaxAffinity}).Score() {
		t.Fatal("affinity is not bounded")
	}
	if (post.Interest{Age: -time.Hour}).Score() != (post.Interest{}).Score() {
		t.Fatal("a post from the future scores more than a new one")
	}
}

func TestWindowStart(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	start := post.WindowStart(at)

	earlier := v7At(t, at.Add(-time.Millisecond))
	later := v7At(t, at)
	if bytes.Compare(earlier[:], start[:]) >= 0 || bytes.Compare(later[:], start[:]) < 0 {
		t.Fatalf("start %s, earlier %s, later %s", start, earlier, later)
	}
}

// v7At is a UUIDv7 of the time at with the rest of its bits set, the
// largest one of that millisecond.
func v7At(t *testing.T, at time.Time) uuid.UUID {
	t.Helper()

	id := post.WindowStart(at)
	for i := 6; i < 16; i++ {
		id[i] = 0xff
	}

	return id
}
