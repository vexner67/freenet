package user

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

// RocketGrowth is how fast the multiplier of "Ракетка" grows: e to the
// power of RocketGrowth times the milliseconds of the flight, so ×2 comes
// in about 5.8 s and ×10 in 19 s. The site draws the flight by the same
// number (rocketMultiplier in labels.js).
const RocketGrowth = 0.00012

// RocketRolls is the range of the roll a crash point is drawn from.
const RocketRolls = 1 << 52

// The bounds of "Ракетка": the return of the casino in hundredths of a
// percent (10000 — 100%: the chance a rocket reaches ×1 cannot be more),
// the highest multiplier a rocket reaches in hundredths (100000 — ×1000)
// and the multiplier to cash at by itself.
const (
	MaxRocketReturn = 10_000
	MinRocketTop    = 200
	MaxRocketTop    = MaxMultiplier
	MinRocketAuto   = 101
)

var (
	ErrRocketLimits   = errors.New("rocket bets must be at least 1 vote, the least not above the most")
	ErrRocketReturn   = errors.New("the return of the rocket must be from 0.01 to 100 percent")
	ErrRocketTop      = errors.New("the top of the rocket must be from 2 to 10000")
	ErrRocketAmount   = errors.New("the bet is out of the limits of the rocket")
	ErrRocketAuto     = errors.New("the rocket cashes by itself from 1.01 to its top")
	ErrRocketNotFound = errors.New("rocket not found")
	ErrRocketOver     = errors.New("the rocket is over")
)

// CasinoRocket is "Ракетка" of the casino: a player bets from Least to Most
// votes, a rocket takes off with a multiplier growing from ×1, and the bet
// comes back multiplied by what it showed when the player cashed — if the
// rocket had not exploded yet. It explodes at a crash point drawn at the
// launch: at x or later with the chance Return / x, never past Top.
type CasinoRocket struct {
	least, most int64
	ret, top    int
}

// NewCasinoRocket checks the rocket; ErrRocketLimits, ErrRocketReturn,
// ErrRocketTop.
func NewCasinoRocket(least, most int64, ret, top int) (CasinoRocket, error) {
	switch {
	case least < 1 || most > MaxBet || least > most:
		return CasinoRocket{}, ErrRocketLimits
	case ret < 1 || ret > MaxRocketReturn:
		return CasinoRocket{}, ErrRocketReturn
	case top < MinRocketTop || top > MaxRocketTop:
		return CasinoRocket{}, ErrRocketTop
	}

	return CasinoRocket{least: least, most: most, ret: ret, top: top}, nil
}

// DefaultCasinoRocket is the rocket until the administrators change it:
// bets of 1 to 100 votes, 85% back on average, up to ×10.
func DefaultCasinoRocket() CasinoRocket {
	return CasinoRocket{least: 1, most: 100, ret: 8500, top: 1000}
}

func (r CasinoRocket) Least() int64 {
	return r.least
}

func (r CasinoRocket) Most() int64 {
	return r.most
}

// Return is what the rocket gives back on average, in hundredths of a
// percent.
func (r CasinoRocket) Return() int {
	return r.ret
}

// Top is the highest multiplier, in hundredths: there the rocket explodes
// whatever was drawn.
func (r CasinoRocket) Top() int {
	return r.top
}

// Crash is the crash point of a roll in [0, RocketRolls), in hundredths:
// Return / (1 - roll / RocketRolls), at least ×1 (the rocket explodes at
// the start) and at most Top.
func (r CasinoRocket) Crash(roll int64) int {
	left := 1 - float64(roll)/RocketRolls
	crash := math.Floor(float64(r.ret) / 100 / left)

	if crash < 100 || math.IsNaN(crash) {
		return 100
	}
	if crash > float64(r.top) {
		return r.top
	}

	return int(crash)
}

// RocketMultiplier is the multiplier of a rocket flying for elapsed, in
// hundredths, rounded down.
func RocketMultiplier(elapsed time.Duration) int {
	if elapsed <= 0 {
		return 100
	}

	m := math.Floor(100 * math.Exp(RocketGrowth*float64(elapsed.Milliseconds())))
	if m > MaxRocketTop {
		return MaxRocketTop
	}

	return int(m)
}

// RocketState is where a round is: still flying, cashed, or over without
// a win — the rocket exploded first.
type RocketState string

const (
	RocketFlying  RocketState = "flying"
	RocketCashed  RocketState = "cashed"
	RocketCrashed RocketState = "crashed"
)

// RocketRound is one flight of the rocket for one bet. With auto the bet
// cashes by itself at that multiplier, so the round is over at the launch;
// otherwise the player cashes it while it flies (Cash).
type RocketRound struct {
	id        uuid.UUID
	userID    uuid.UUID
	amount    int64
	crash     int
	auto      int
	startedAt time.Time
	cashed    int
	payout    int64
}

// Launch is a round of amount votes with the crash point drawn, cashing by
// itself at auto (0 — by hand); ErrRocketAmount, ErrRocketAuto.
func (r CasinoRocket) Launch(id, userID uuid.UUID, amount int64, auto, crash int, now time.Time) (*RocketRound, error) {
	if amount < r.least || amount > r.most {
		return nil, ErrRocketAmount
	}
	if auto != 0 && (auto < MinRocketAuto || auto > r.top) {
		return nil, ErrRocketAuto
	}

	round := &RocketRound{id: id, userID: userID, amount: amount, crash: crash, auto: auto, startedAt: now.UTC()}
	if auto != 0 && auto < crash {
		round.cashed, round.payout = auto, prize(amount, auto)
	}

	return round, nil
}

// MostWin is the most a bet of amount may win: at auto, or by hand at the
// top.
func (r CasinoRocket) MostWin(amount int64, auto int) int64 {
	if auto != 0 {
		return prize(amount, auto)
	}

	return prize(amount, r.top)
}

// RestoreRocketRound is a round as the storage keeps it.
func RestoreRocketRound(id, userID uuid.UUID, amount int64, crash, auto int, startedAt time.Time, cashed int, payout int64) *RocketRound {
	return &RocketRound{id: id, userID: userID, amount: amount, crash: crash, auto: auto, startedAt: startedAt.UTC(), cashed: cashed, payout: payout}
}

func prize(amount int64, multiplier int) int64 {
	return amount * int64(multiplier) / 100
}

func (r *RocketRound) ID() uuid.UUID {
	return r.id
}

func (r *RocketRound) UserID() uuid.UUID {
	return r.userID
}

func (r *RocketRound) Amount() int64 {
	return r.amount
}

// Crash is the crash point, in hundredths: not to be told while the
// rocket flies.
func (r *RocketRound) Crash() int {
	return r.crash
}

// Auto is the multiplier the round cashes at by itself, 0 — by hand.
func (r *RocketRound) Auto() int {
	return r.auto
}

func (r *RocketRound) StartedAt() time.Time {
	return r.startedAt
}

// Cashed is the multiplier the round was cashed at, 0 — it was not.
func (r *RocketRound) Cashed() int {
	return r.cashed
}

// Payout is what the round paid: the bet times Cashed, rounded down.
func (r *RocketRound) Payout() int64 {
	return r.payout
}

// Owned tells whether the round is userID's; one not theirs is not found.
func (r *RocketRound) Owned(userID uuid.UUID) bool {
	return r.userID == userID
}

// StateAt is where the round is at now: a round cashing by itself is over
// at the launch, the others fly until the multiplier reaches the crash.
func (r *RocketRound) StateAt(now time.Time) RocketState {
	switch {
	case r.cashed > 0:
		return RocketCashed
	case r.auto > 0 || RocketMultiplier(now.Sub(r.startedAt)) >= r.crash:
		return RocketCrashed
	default:
		return RocketFlying
	}
}

// Cash takes the bet back at the multiplier of now: the round cashed, or,
// when the rocket has exploded already, crashed without a win.
// ErrRocketOver for a round cashed or cashing by itself.
func (r *RocketRound) Cash(now time.Time) (*RocketRound, error) {
	if r.cashed > 0 || r.auto > 0 {
		return nil, ErrRocketOver
	}

	m := RocketMultiplier(now.Sub(r.startedAt))
	if m >= r.crash {
		return r, nil
	}

	cashed := *r
	cashed.cashed, cashed.payout = m, prize(r.amount, m)

	return &cashed, nil
}
