package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// PetRepository keeps the pets, their fights and the shop, the one row of
// pet_shop.
type PetRepository struct {
	pool *pgxpool.Pool
}

func NewPetRepository(pool *pgxpool.Pool) *PetRepository {
	return &PetRepository{
		pool: pool,
	}
}

func (r *PetRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// storedPetShop is the shop as the column keeps it.
type storedPetShop struct {
	Prices map[string]int64 `json:"prices"`
	Meal   int64            `json:"meal"`
	Win    int64            `json:"win"`
	Fights int              `json:"fights"`
}

const petShopQuery = `
	SELECT settings
	FROM pet_shop
`

func (r *PetRepository) Shop(ctx context.Context) (user.PetShop, error) {
	var data []byte
	if err := r.conn(ctx).QueryRow(ctx, petShopQuery).Scan(&data); err != nil {
		return user.PetShop{}, errs.Wrap("r.conn.QueryRow", err)
	}

	if data == nil {
		return user.DefaultPetShop(), nil
	}

	var stored storedPetShop
	if err := json.Unmarshal(data, &stored); err != nil {
		return user.PetShop{}, errs.Wrap("json.Unmarshal", err)
	}

	prices := make(map[user.PetKind]int64, len(stored.Prices))
	for kind, price := range stored.Prices {
		prices[user.PetKind(kind)] = price
	}

	shop, err := user.NewPetShop(prices, stored.Meal, stored.Win, stored.Fights)

	return shop, errs.Wrap("user.NewPetShop", err)
}

const setPetShopQuery = `
	UPDATE pet_shop
	SET settings = $1
`

func (r *PetRepository) SetShop(ctx context.Context, shop user.PetShop) error {
	stored := storedPetShop{Prices: map[string]int64{}, Meal: shop.Meal(), Win: shop.Win(), Fights: shop.Fights()}
	for kind, price := range shop.Prices() {
		stored.Prices[kind.String()] = price
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, setPetShopQuery, data)

	return errs.Wrap("r.conn.Exec", err)
}

// petColumns are the columns of a pet in the order petArgs and scanPet
// keep them.
const petColumns = `
		id,
		user_id,
		kind,
		name,
		born_at,
		satiety,
		joy,
		counted_at,
		played_at,
		experience,
		rating,
		wins,
		losses,
		fought_on,
		fights
`

const listPetsQuery = `
	SELECT` + petColumns + `
	FROM pets
	WHERE user_id = $1
	ORDER BY id
`

func (r *PetRepository) Pets(ctx context.Context, userID uuid.UUID) ([]*user.Pet, error) {
	rows, err := r.conn(ctx).Query(ctx, listPetsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	pets, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*user.Pet, error) {
		return scanPet(row)
	})

	return pets, errs.Wrap("pgx.CollectRows", err)
}

const selectPetQuery = `
	SELECT` + petColumns + `
	FROM pets
	WHERE id = $2
		AND user_id = $1
`

func (r *PetRepository) Pet(ctx context.Context, userID, petID uuid.UUID) (*user.Pet, error) {
	p, err := r.pet(ctx, selectPetQuery, userID, petID)

	return p, errs.Wrap("r.pet", err)
}

func (r *PetRepository) LockPet(ctx context.Context, userID, petID uuid.UUID) (*user.Pet, error) {
	p, err := r.pet(ctx, selectPetQuery+" FOR UPDATE", userID, petID)

	return p, errs.Wrap("r.pet", err)
}

func (r *PetRepository) pet(ctx context.Context, query string, userID, petID uuid.UUID) (*user.Pet, error) {
	p, err := scanPet(r.conn(ctx).QueryRow(ctx, query, userID, petID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrPetNotFound
	}

	return p, errs.Wrap("scanPet", err)
}

// scanPet reads the petColumns of row.
func scanPet(row pgx.Row) (*user.Pet, error) {
	var (
		s                  user.PetState
		ownerID            uuid.UUID
		kind, name         string
		playedAt, foughtOn *time.Time
	)

	err := row.Scan(
		&s.ID, &ownerID, &kind, &name, &s.BornAt, &s.Satiety, &s.Joy, &s.CountedAt, &playedAt,
		&s.Experience, &s.Rating, &s.Wins, &s.Losses, &foughtOn, &s.Fights,
	)
	if err != nil {
		return nil, err
	}

	s.Kind, s.Name = user.PetKind(kind), user.PetName(name)
	if playedAt != nil {
		s.PlayedAt = playedAt.UTC()
	}
	if foughtOn != nil {
		s.FoughtOn = *foughtOn
	}
	s.BornAt, s.CountedAt = s.BornAt.UTC(), s.CountedAt.UTC()

	return user.RestorePet(s), nil
}

// petArgs are the petColumns of p kept by userID.
func petArgs(userID uuid.UUID, p *user.Pet) []any {
	s := p.State()

	var playedAt, foughtOn *time.Time
	if !s.PlayedAt.IsZero() {
		playedAt = &s.PlayedAt
	}
	if !s.FoughtOn.IsZero() {
		foughtOn = &s.FoughtOn
	}

	return []any{
		s.ID, userID, s.Kind.String(), s.Name.String(), s.BornAt, s.Satiety, s.Joy, s.CountedAt, playedAt,
		s.Experience, s.Rating, s.Wins, s.Losses, foughtOn, s.Fights,
	}
}

// insertPetQuery adds the pet unless its owner keeps $16 already.
const insertPetQuery = `
	INSERT INTO pets (` + petColumns + `)
	SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
	WHERE (
		SELECT count(*)
		FROM pets
		WHERE user_id = $2
	) < $16
`

func (r *PetRepository) AddPet(ctx context.Context, userID uuid.UUID, p *user.Pet) error {
	tag, err := r.conn(ctx).Exec(ctx, insertPetQuery, append(petArgs(userID, p), user.MaxPets)...)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrTooManyPets
	}

	return nil
}

const updatePetQuery = `
	UPDATE pets
	SET kind = $2,
		name = $3,
		born_at = $4,
		satiety = $5,
		joy = $6,
		counted_at = $7,
		played_at = $8,
		experience = $9,
		rating = $10,
		wins = $11,
		losses = $12,
		fought_on = $13,
		fights = $14
	WHERE id = $1
`

func (r *PetRepository) SavePet(ctx context.Context, p *user.Pet) error {
	args := petArgs(uuid.Nil, p)
	args = append(args[:1], args[2:]...)

	tag, err := r.conn(ctx).Exec(ctx, updatePetQuery, args...)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrPetNotFound
	}

	return nil
}

const deletePetQuery = `
	DELETE FROM pets
	WHERE id = $2
		AND user_id = $1
`

func (r *PetRepository) DeletePet(ctx context.Context, userID, petID uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deletePetQuery, userID, petID)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrPetNotFound
	}

	return nil
}

// petOpponentQuery takes the ten pets nearest to the rating, by the rating
// index from above and below, and one of them at random.
const petOpponentQuery = `
	WITH near AS (
		(SELECT p.user_id, p.id, p.rating
		FROM pets AS p
		JOIN users AS u ON u.id = p.user_id
		WHERE p.rating >= $2
			AND p.user_id <> $1
			AND ` + petShownUser + `
		ORDER BY p.rating, p.id
		LIMIT 10)
		UNION ALL
		(SELECT p.user_id, p.id, p.rating
		FROM pets AS p
		JOIN users AS u ON u.id = p.user_id
		WHERE p.rating < $2
			AND p.user_id <> $1
			AND ` + petShownUser + `
		ORDER BY p.rating DESC, p.id
		LIMIT 10)
	)
	SELECT user_id, id
	FROM (
		SELECT user_id, id
		FROM near
		ORDER BY abs(rating - $2), id
		LIMIT 10
	) AS nearest
	ORDER BY random()
	LIMIT 1
`

// petShownUser is the condition of an owner u neither deleted nor banned
// at $3.
const petShownUser = `u.deactivated_at IS NULL AND (u.banned_at IS NULL OR u.banned_until <= $3)`

func (r *PetRepository) Opponent(ctx context.Context, userID uuid.UUID, rating int, now time.Time) (owner, pet uuid.UUID, err error) {
	err = r.conn(ctx).QueryRow(ctx, petOpponentQuery, userID, rating, now).Scan(&owner, &pet)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, user.ErrNoPetOpponent
	}

	return owner, pet, errs.Wrap("r.conn.QueryRow", err)
}

// storedHit, storedFighter and storedBout are a fight as the column keeps
// it.
type storedHit struct {
	ByAttacker     bool `json:"byAttacker"`
	Dodged         bool `json:"dodged"`
	Crit           bool `json:"crit"`
	Damage         int  `json:"damage"`
	AttackerHealth int  `json:"attackerHealth"`
	DefenderHealth int  `json:"defenderHealth"`
}

type storedFighter struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Health int    `json:"health"`
}

type storedBout struct {
	Attacker storedFighter `json:"attacker"`
	Defender storedFighter `json:"defender"`
	Hits     []storedHit   `json:"hits"`
	Won      bool          `json:"won"`
	Rating   int           `json:"rating"`
}

func storeFighter(f user.PetFighter) storedFighter {
	return storedFighter{Kind: f.Kind.String(), Name: f.Name.String(), Level: f.Level, Health: f.Health}
}

func restoreFighter(f storedFighter) user.PetFighter {
	return user.PetFighter{Kind: user.PetKind(f.Kind), Name: user.PetName(f.Name), Level: f.Level, Health: f.Health}
}

const insertPetFightQuery = `
	INSERT INTO pet_fights (
		id,
		attacker_id,
		defender_id,
		bout,
		fought_at
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *PetRepository) AddFight(ctx context.Context, f user.PetFight) error {
	stored := storedBout{
		Attacker: storeFighter(f.Bout.Attacker), Defender: storeFighter(f.Bout.Defender),
		Hits: make([]storedHit, len(f.Bout.Hits)), Won: f.Bout.Won, Rating: f.Bout.Rating,
	}
	for i, h := range f.Bout.Hits {
		stored.Hits[i] = storedHit(h)
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, insertPetFightQuery, f.ID, f.AttackerID, f.DefenderID, data, f.At)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

const listPetFightsQuery = `
	SELECT
		id,
		attacker_id,
		defender_id,
		bout,
		fought_at
	FROM (
		(SELECT id, attacker_id, defender_id, bout, fought_at
		FROM pet_fights
		WHERE attacker_id = $1
		ORDER BY id DESC
		LIMIT $2)
		UNION
		(SELECT id, attacker_id, defender_id, bout, fought_at
		FROM pet_fights
		WHERE defender_id = $1
		ORDER BY id DESC
		LIMIT $2)
	) AS fights
	ORDER BY id DESC
	LIMIT $2
`

func (r *PetRepository) Fights(ctx context.Context, userID uuid.UUID, limit int) ([]user.PetFight, error) {
	rows, err := r.conn(ctx).Query(ctx, listPetFightsQuery, userID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	fights, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.PetFight, error) {
		var (
			f    user.PetFight
			data []byte
		)
		if err := row.Scan(&f.ID, &f.AttackerID, &f.DefenderID, &data, &f.At); err != nil {
			return f, err
		}

		var stored storedBout
		if err := json.Unmarshal(data, &stored); err != nil {
			return f, err
		}

		f.At = f.At.UTC()
		f.Bout = user.PetBout{
			Attacker: restoreFighter(stored.Attacker), Defender: restoreFighter(stored.Defender),
			Hits: make([]user.PetHit, len(stored.Hits)), Won: stored.Won, Rating: stored.Rating,
		}
		for i, h := range stored.Hits {
			f.Bout.Hits[i] = user.PetHit(h)
		}

		return f, nil
	})

	return fights, errs.Wrap("pgx.CollectRows", err)
}

const countPetShopQuery = `
	UPDATE pet_shop
	SET spent = spent + $1,
		won = won + $2
`

func (r *PetRepository) Count(ctx context.Context, spent, won int64) error {
	_, err := r.conn(ctx).Exec(ctx, countPetShopQuery, spent, won)

	return errs.Wrap("r.conn.Exec", err)
}

const petShopStatsQuery = `
	SELECT
		(SELECT count(*) FROM pets),
		(SELECT count(*) FROM pet_fights),
		spent,
		won
	FROM pet_shop
`

func (r *PetRepository) Stats(ctx context.Context) (user.PetShopStats, error) {
	var s user.PetShopStats
	err := r.conn(ctx).QueryRow(ctx, petShopStatsQuery).Scan(&s.Pets, &s.Fights, &s.Spent, &s.Won)

	return s, errs.Wrap("r.conn.QueryRow", err)
}
