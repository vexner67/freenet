package user

import "math/big"

// MulDiv is a×b/d rounded toward zero, as a*b/d is in Go, without the
// overflow of a*b, and never further from zero than MaxPayout: the votes
// moved may be as many as an administrator has (MaxMoved), times a
// multiplier of up to MaxMultiplier.
func MulDiv(a, b, d int64) int64 {
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))

	return bounded(n.Quo(n, big.NewInt(d)))
}

// MulDivUp is MulDiv rounded up, for a, b ≥ 0 and d > 0.
func MulDivUp(a, b, d int64) int64 {
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	n.Add(n, big.NewInt(d-1))

	return bounded(n.Quo(n, big.NewInt(d)))
}

func bounded(n *big.Int) int64 {
	switch most := big.NewInt(MaxPayout); {
	case n.Cmp(most) > 0:
		return MaxPayout
	case n.Cmp(most.Neg(most)) < 0:
		return -MaxPayout
	default:
		return n.Int64()
	}
}
