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
	// Cases are missing from a shop set before there were cases: the
	// default prices then.
	Cases  map[string]int64 `json:"cases,omitempty"`
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
	if err != nil || stored.Cases == nil {
		return shop, errs.Wrap("user.NewPetShop", err)
	}

	cases := make(map[user.PetCase]int64, len(stored.Cases))
	for c, price := range stored.Cases {
		cases[user.PetCase(c)] = price
	}

	shop, err = shop.WithCases(cases)

	return shop, errs.Wrap("shop.WithCases", err)
}

const setPetShopQuery = `
	UPDATE pet_shop
	SET settings = $1
`

func (r *PetRepository) SetShop(ctx context.Context, shop user.PetShop) error {
	stored := storedPetShop{Prices: map[string]int64{}, Cases: map[string]int64{}, Meal: shop.Meal(), Win: shop.Win(), Fights: shop.Fights()}
	for kind, price := range shop.Prices() {
		stored.Prices[kind.String()] = price
	}
	for c, price := range shop.Cases() {
		stored.Cases[c.String()] = price
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
		fights,
		wardrobe,
		outfit,
		stance,
		streak,
		streak_at,
		owned_since,
		sale_price,
		boss_at
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
		kind, name, stance string
		playedAt, foughtOn *time.Time
		streakAt           *time.Time
		ownedSince, bossAt *time.Time
		wardrobe, outfit   []string
	)

	err := row.Scan(
		&s.ID, &ownerID, &kind, &name, &s.BornAt, &s.Satiety, &s.Joy, &s.CountedAt, &playedAt,
		&s.Experience, &s.Rating, &s.Wins, &s.Losses, &foughtOn, &s.Fights, &wardrobe, &outfit,
		&stance, &s.Streak, &streakAt, &ownedSince, &s.SalePrice, &bossAt,
	)
	if err != nil {
		return nil, err
	}

	s.Kind, s.Name, s.Stance = user.PetKind(kind), user.PetName(name), user.PetStance(stance)
	if streakAt != nil {
		s.StreakAt = streakAt.UTC()
	}
	if ownedSince != nil {
		s.OwnedSince = ownedSince.UTC()
	}
	if bossAt != nil {
		s.BossAt = bossAt.UTC()
	}
	s.Wardrobe, s.Outfit = petItems(wardrobe), petItems(outfit)
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

	var playedAt, foughtOn, streakAt, ownedSince, bossAt *time.Time
	if !s.OwnedSince.IsZero() {
		ownedSince = &s.OwnedSince
	}
	if !s.BossAt.IsZero() {
		bossAt = &s.BossAt
	}
	if !s.PlayedAt.IsZero() {
		playedAt = &s.PlayedAt
	}
	if !s.StreakAt.IsZero() {
		streakAt = &s.StreakAt
	}
	if !s.FoughtOn.IsZero() {
		foughtOn = &s.FoughtOn
	}

	return []any{
		s.ID, userID, s.Kind.String(), s.Name.String(), s.BornAt, s.Satiety, s.Joy, s.CountedAt, playedAt,
		s.Experience, s.Rating, s.Wins, s.Losses, foughtOn, s.Fights, itemNames(s.Wardrobe), itemNames(s.Outfit),
		s.Stance.String(), s.Streak, streakAt, ownedSince, s.SalePrice, bossAt,
	}
}

// petItems are the clothes of names, none for none.
func petItems(names []string) []user.PetItem {
	if len(names) == 0 {
		return nil
	}

	items := make([]user.PetItem, len(names))
	for i, n := range names {
		items[i] = user.PetItem(n)
	}

	return items
}

func itemNames(items []user.PetItem) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.String()
	}

	return names
}

// insertPetQuery adds the pet unless its owner keeps $24 already.
const insertPetQuery = `
	INSERT INTO pets (` + petColumns + `)
	SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
	WHERE (
		SELECT count(*)
		FROM pets
		WHERE user_id = $2
	) < $24
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
		fights = $14,
		wardrobe = $15,
		outfit = $16,
		stance = $17,
		streak = $18,
		streak_at = $19,
		owned_since = $20,
		sale_price = $21,
		boss_at = $22
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

// petContendersQuery takes the pets nearest to the rating, by the rating
// index from above and below, $4 of them.
const petContendersQuery = `
	WITH near AS (
		(SELECT p.id, p.rating
		FROM pets AS p
		JOIN users AS u ON u.id = p.user_id
		WHERE p.rating >= $2
			AND p.user_id <> $1
			AND ` + petShownUser + `
		ORDER BY p.rating, p.id
		LIMIT $4)
		UNION ALL
		(SELECT p.id, p.rating
		FROM pets AS p
		JOIN users AS u ON u.id = p.user_id
		WHERE p.rating < $2
			AND p.user_id <> $1
			AND ` + petShownUser + `
		ORDER BY p.rating DESC, p.id
		LIMIT $4)
	)
	SELECT user_id,` + petColumns + `
	FROM pets
	WHERE id IN (
		SELECT id
		FROM near
		ORDER BY abs(rating - $2), id
		LIMIT $4
	)
	ORDER BY abs(rating - $2), id
`

// petShownUser is the condition of an owner u neither deleted nor banned
// at $3.
const petShownUser = `u.deactivated_at IS NULL AND (u.banned_at IS NULL OR u.banned_until <= $3)`

func (r *PetRepository) Contenders(ctx context.Context, userID uuid.UUID, rating int, now time.Time, limit int) ([]user.PetContender, error) {
	rows, err := r.conn(ctx).Query(ctx, petContendersQuery, userID, rating, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	contenders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.PetContender, error) {
		var owner uuid.UUID
		p, err := scanPet(ownedRow{row: row, owner: &owner})

		return user.PetContender{Owner: owner, Pet: p}, err
	})

	return contenders, errs.Wrap("pgx.CollectRows", err)
}

// ownedRow is a row of the owner of a pet and its petColumns.
type ownedRow struct {
	row   pgx.Row
	owner *uuid.UUID
}

func (r ownedRow) Scan(dest ...any) error {
	return r.row.Scan(append([]any{r.owner}, dest...)...)
}

// storedHit, storedFighter and storedBout are a fight as the column keeps
// it.
type storedHit struct {
	ByAttacker     bool            `json:"byAttacker"`
	Move           user.PetMove    `json:"move,omitempty"`
	Dodged         bool            `json:"dodged"`
	Crit           bool            `json:"crit"`
	Damage         int             `json:"damage"`
	Heal           int             `json:"heal,omitempty"`
	Special        user.PetSpecial `json:"special,omitempty"`
	Answer         user.PetSpecial `json:"answer,omitempty"`
	AttackerHealth int             `json:"attackerHealth"`
	DefenderHealth int             `json:"defenderHealth"`
}

type storedFighter struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Level   int      `json:"level"`
	Health  int      `json:"health"`
	Outfit  []string `json:"outfit,omitempty"`
	Stance  string   `json:"stance,omitempty"`
	Power   int      `json:"power,omitempty"`
	Fatigue int      `json:"fatigue,omitempty"`
}

type storedBout struct {
	Attacker storedFighter `json:"attacker"`
	Defender storedFighter `json:"defender"`
	Hits     []storedHit   `json:"hits"`
	Won      bool          `json:"won"`
	Rating   int           `json:"rating"`
	Ratio    int           `json:"ratio,omitempty"`
	Reward   int64         `json:"reward,omitempty"`
}

func storeFighter(f user.PetFighter) storedFighter {
	return storedFighter{
		Kind: f.Kind.String(), Name: f.Name.String(), Level: f.Level, Health: f.Health, Outfit: itemNames(f.Outfit),
		Stance: f.Stance.String(), Power: f.Power, Fatigue: f.Fatigue,
	}
}

func restoreFighter(f storedFighter) user.PetFighter {
	return user.PetFighter{
		Kind: user.PetKind(f.Kind), Name: user.PetName(f.Name), Level: f.Level, Health: f.Health, Outfit: petItems(f.Outfit),
		Stance: user.PetStance(f.Stance), Power: f.Power, Fatigue: f.Fatigue,
	}
}

func storeHits(hits []user.PetHit) []storedHit {
	stored := make([]storedHit, len(hits))
	for i, h := range hits {
		stored[i] = storedHit(h)
	}

	return stored
}

func restoreHits(stored []storedHit) []user.PetHit {
	hits := make([]user.PetHit, len(stored))
	for i, h := range stored {
		hits[i] = user.PetHit(h)
	}

	return hits
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
		Hits: storeHits(f.Bout.Hits), Won: f.Bout.Won, Rating: f.Bout.Rating, Ratio: f.Bout.Ratio, Reward: f.Bout.Reward,
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
			Hits: restoreHits(stored.Hits), Won: stored.Won, Rating: stored.Rating, Ratio: stored.Ratio, Reward: stored.Reward,
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

const selectPetKeeperQuery = `
	SELECT
		shards,
		pity,
		cases_on,
		cases,
		drop_kind,
		drop_case
	FROM pet_keepers
	WHERE user_id = $1
`

// ensurePetKeeperQuery adds the row of a keeper, so that it can be locked.
const ensurePetKeeperQuery = `
	INSERT INTO pet_keepers (user_id)
	VALUES ($1)
	ON CONFLICT (user_id) DO NOTHING
`

func (r *PetRepository) Keeper(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error) {
	k, err := r.keeper(ctx, selectPetKeeperQuery, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.NewPetKeeper(), nil
	}

	return k, errs.Wrap("r.keeper", err)
}

func (r *PetRepository) LockKeeper(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error) {
	_, err := r.conn(ctx).Exec(ctx, ensurePetKeeperQuery, userID)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.Exec", err)
	}

	k, err := r.keeper(ctx, selectPetKeeperQuery+" FOR UPDATE", userID)

	return k, errs.Wrap("r.keeper", err)
}

func (r *PetRepository) keeper(ctx context.Context, query string, userID uuid.UUID) (*user.PetKeeper, error) {
	var (
		s                  user.PetKeeperState
		casesOn            *time.Time
		dropKind, dropCase *string
	)

	err := r.conn(ctx).QueryRow(ctx, query, userID).Scan(&s.Shards, &s.Pity, &casesOn, &s.Cases, &dropKind, &dropCase)
	if err != nil {
		return nil, err
	}

	if casesOn != nil {
		s.CasesOn = *casesOn
	}
	if dropKind != nil && dropCase != nil {
		s.Drop, s.DropCase = user.PetKind(*dropKind), user.PetCase(*dropCase)
	}

	return user.RestorePetKeeper(s), nil
}

const upsertPetKeeperQuery = `
	INSERT INTO pet_keepers (
		user_id,
		shards,
		pity,
		cases_on,
		cases,
		drop_kind,
		drop_case
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (user_id) DO UPDATE
	SET shards = EXCLUDED.shards,
		pity = EXCLUDED.pity,
		cases_on = EXCLUDED.cases_on,
		cases = EXCLUDED.cases,
		drop_kind = EXCLUDED.drop_kind,
		drop_case = EXCLUDED.drop_case
`

func (r *PetRepository) SaveKeeper(ctx context.Context, userID uuid.UUID, k *user.PetKeeper) error {
	s := k.State()

	var (
		casesOn            *time.Time
		dropKind, dropCase *string
	)
	if !s.CasesOn.IsZero() {
		casesOn = &s.CasesOn
	}
	if s.Drop != "" {
		kind, c := s.Drop.String(), s.DropCase.String()
		dropKind, dropCase = &kind, &c
	}

	_, err := r.conn(ctx).Exec(ctx, upsertPetKeeperQuery, userID, s.Shards, s.Pity, casesOn, s.Cases, dropKind, dropCase)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

// The state of a battle is the domain state as JSON: it is only ever read
// back by this code, whole.
const listPetBattlesQuery = `
	SELECT state
	FROM pet_battles
	WHERE user_id = $1
	ORDER BY pet_id
`

func (r *PetRepository) Battles(ctx context.Context, userID uuid.UUID) ([]*user.PetBattle, error) {
	rows, err := r.conn(ctx).Query(ctx, listPetBattlesQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	battles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*user.PetBattle, error) {
		return scanBattle(row)
	})

	return battles, errs.Wrap("pgx.CollectRows", err)
}

const lockPetBattleQuery = `
	SELECT state
	FROM pet_battles
	WHERE pet_id = $2
		AND user_id = $1
	FOR UPDATE
`

func (r *PetRepository) LockBattle(ctx context.Context, userID, petID uuid.UUID) (*user.PetBattle, error) {
	b, err := scanBattle(r.conn(ctx).QueryRow(ctx, lockPetBattleQuery, userID, petID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrNoPetBattle
	}

	return b, errs.Wrap("scanBattle", err)
}

func scanBattle(row pgx.Row) (*user.PetBattle, error) {
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, err
	}

	var s user.PetBattleState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}
	s.StartedAt = s.StartedAt.UTC()

	return user.RestorePetBattle(s), nil
}

const upsertPetBattleQuery = `
	INSERT INTO pet_battles (
		pet_id,
		user_id,
		state,
		started_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (pet_id) DO UPDATE
	SET state = EXCLUDED.state
`

func (r *PetRepository) SaveBattle(ctx context.Context, userID uuid.UUID, b *user.PetBattle) error {
	data, err := json.Marshal(b.State())
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, upsertPetBattleQuery, b.PetID(), userID, data, b.StartedAt())

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrPetNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

const deletePetBattleQuery = `
	DELETE FROM pet_battles
	WHERE pet_id = $2
		AND user_id = $1
`

func (r *PetRepository) DeleteBattle(ctx context.Context, userID, petID uuid.UUID) error {
	_, err := r.conn(ctx).Exec(ctx, deletePetBattleQuery, userID, petID)

	return errs.Wrap("r.conn.Exec", err)
}
