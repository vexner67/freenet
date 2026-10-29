package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// The boss, the tournament, the tasks of a person and the places of a
// season are their domain states as JSON, as the battles are: only ever
// read back by this code, whole.

const selectPetBossQuery = `
	SELECT state
	FROM pet_bosses
	WHERE week = $1
`

func (r *PetRepository) Boss(ctx context.Context, week time.Time) (*user.PetBoss, error) {
	var s user.PetBossState
	if err := r.state(ctx, selectPetBossQuery, week, &s); err != nil {
		return nil, err
	}

	return user.RestorePetBoss(s), nil
}

// state reads the JSON state of the row of key by query into s;
// user.ErrNotFound without it.
func (r *PetRepository) state(ctx context.Context, query string, key, s any) error {
	var data []byte
	err := r.conn(ctx).QueryRow(ctx, query, key).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	return errs.Wrap("json.Unmarshal", json.Unmarshal(data, s))
}

const ensurePetBossQuery = `
	INSERT INTO pet_bosses (
		week,
		state
	)
	VALUES ($1, $2)
	ON CONFLICT (week) DO NOTHING
`

func (r *PetRepository) LockBoss(ctx context.Context, fresh *user.PetBoss) (*user.PetBoss, error) {
	if err := r.ensure(ctx, ensurePetBossQuery, fresh.Week(), fresh.State()); err != nil {
		return nil, err
	}

	var s user.PetBossState
	if err := r.state(ctx, selectPetBossQuery+" FOR UPDATE", fresh.Week(), &s); err != nil {
		return nil, err
	}

	return user.RestorePetBoss(s), nil
}

// ensure writes the state s of the week by query.
func (r *PetRepository) ensure(ctx context.Context, query string, week time.Time, s any) error {
	data, err := json.Marshal(s)
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, query, week, data)

	return errs.Wrap("r.conn.Exec", err)
}

const savePetBossQuery = `
	INSERT INTO pet_bosses (
		week,
		state
	)
	VALUES ($1, $2)
	ON CONFLICT (week) DO UPDATE
	SET state = EXCLUDED.state
`

func (r *PetRepository) SaveBoss(ctx context.Context, b *user.PetBoss) error {
	return r.ensure(ctx, savePetBossQuery, b.Week(), b.State())
}

const selectPetTournamentQuery = `
	SELECT state
	FROM pet_tournaments
	WHERE week = $1
`

func (r *PetRepository) Tournament(ctx context.Context, week time.Time) (*user.PetTournament, error) {
	var s user.PetTournamentState
	if err := r.state(ctx, selectPetTournamentQuery, week, &s); err != nil {
		return nil, err
	}

	return user.RestorePetTournament(s), nil
}

const ensurePetTournamentQuery = `
	INSERT INTO pet_tournaments (
		week,
		state
	)
	VALUES ($1, $2)
	ON CONFLICT (week) DO NOTHING
`

func (r *PetRepository) LockTournament(ctx context.Context, fresh *user.PetTournament) (*user.PetTournament, error) {
	if err := r.ensure(ctx, ensurePetTournamentQuery, fresh.Week(), fresh.State()); err != nil {
		return nil, err
	}

	var s user.PetTournamentState
	if err := r.state(ctx, selectPetTournamentQuery+" FOR UPDATE", fresh.Week(), &s); err != nil {
		return nil, err
	}

	return user.RestorePetTournament(s), nil
}

const savePetTournamentQuery = `
	INSERT INTO pet_tournaments (
		week,
		state
	)
	VALUES ($1, $2)
	ON CONFLICT (week) DO UPDATE
	SET state = EXCLUDED.state
`

func (r *PetRepository) SaveTournament(ctx context.Context, t *user.PetTournament) error {
	return r.ensure(ctx, savePetTournamentQuery, t.Week(), t.State())
}

const bestPetsQuery = `
	SELECT p.user_id,` + petColumns + `
	FROM pets AS p
	JOIN users AS u ON u.id = p.user_id
	WHERE ` + petShownAt + `
	ORDER BY p.rating DESC, p.id
	LIMIT $2
`

// petShownAt is petShownUser with now as $1.
const petShownAt = `u.deactivated_at IS NULL AND (u.banned_at IS NULL OR u.banned_until <= $1)`

func (r *PetRepository) Best(ctx context.Context, now time.Time, limit int) ([]user.PetContender, error) {
	return r.ownedPets(ctx, bestPetsQuery, now, limit)
}

// ownedPets are the pets and their owners query reads with args.
func (r *PetRepository) ownedPets(ctx context.Context, query string, args ...any) ([]user.PetContender, error) {
	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	pets, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.PetContender, error) {
		var owner uuid.UUID
		p, err := scanPet(ownedRow{row: row, owner: &owner})

		return user.PetContender{Owner: owner, Pet: p}, err
	})

	return pets, errs.Wrap("pgx.CollectRows", err)
}

const listPetSeasonsQuery = `
	SELECT season,
		places
	FROM pet_seasons
	ORDER BY season DESC
	LIMIT $1
`

func (r *PetRepository) Seasons(ctx context.Context, limit int) ([]user.PetSeasonResult, error) {
	rows, err := r.conn(ctx).Query(ctx, listPetSeasonsQuery, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	seasons, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.PetSeasonResult, error) {
		var (
			s    user.PetSeasonResult
			data []byte
		)
		if err := row.Scan(&s.Season, &data); err != nil {
			return s, err
		}

		return s, json.Unmarshal(data, &s.Places)
	})

	return seasons, errs.Wrap("pgx.CollectRows", err)
}

const closePetSeasonQuery = `
	INSERT INTO pet_seasons (
		season,
		places,
		closed_at
	)
	VALUES ($1, $2, now())
	ON CONFLICT (season) DO NOTHING
`

// carryPetRatingsQuery is user.SeasonRating of every rating: $1 the
// start, $2 the percent kept.
const carryPetRatingsQuery = `
	UPDATE pets
	SET rating = $1 + (rating - $1) * $2 / 100
`

func (r *PetRepository) CloseSeason(ctx context.Context, result user.PetSeasonResult) (bool, error) {
	places := result.Places
	if places == nil {
		places = []user.PetSeasonPlace{}
	}

	data, err := json.Marshal(places)
	if err != nil {
		return false, errs.Wrap("json.Marshal", err)
	}

	tag, err := r.conn(ctx).Exec(ctx, closePetSeasonQuery, result.Season, data)
	if err != nil {
		return false, errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	_, err = r.conn(ctx).Exec(ctx, carryPetRatingsQuery, user.PetStartRating, user.PetSeasonKeeps)

	return true, errs.Wrap("r.conn.Exec", err)
}

const ensurePetQuestsQuery = `
	INSERT INTO pet_quests (
		user_id,
		state
	)
	VALUES ($1, '{}')
	ON CONFLICT (user_id) DO NOTHING
`

const lockPetQuestsQuery = `
	SELECT state
	FROM pet_quests
	WHERE user_id = $1
	FOR UPDATE
`

func (r *PetRepository) LockQuests(ctx context.Context, userID uuid.UUID) (*user.PetQuests, error) {
	_, err := r.conn(ctx).Exec(ctx, ensurePetQuestsQuery, userID)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.Exec", err)
	}

	var s user.PetQuestsState
	if err = r.state(ctx, lockPetQuestsQuery, userID, &s); err != nil {
		return nil, err
	}

	return user.RestorePetQuests(userID, s), nil
}

const savePetQuestsQuery = `
	UPDATE pet_quests
	SET state = $2
	WHERE user_id = $1
`

func (r *PetRepository) SaveQuests(ctx context.Context, userID uuid.UUID, q *user.PetQuests) error {
	data, err := json.Marshal(q.State())
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	_, err = r.conn(ctx).Exec(ctx, savePetQuestsQuery, userID, data)

	return errs.Wrap("r.conn.Exec", err)
}

const listPetAchievementsQuery = `
	SELECT achievement,
		got_at
	FROM pet_achievements
	WHERE user_id = $1
	ORDER BY got_at, achievement
`

func (r *PetRepository) Achievements(ctx context.Context, userID uuid.UUID) ([]user.PetAchievementGot, error) {
	rows, err := r.conn(ctx).Query(ctx, listPetAchievementsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (user.PetAchievementGot, error) {
		var (
			g    user.PetAchievementGot
			name string
		)
		err := row.Scan(&name, &g.At)
		g.Achievement, g.At = user.PetAchievement(name), g.At.UTC()

		return g, err
	})

	return got, errs.Wrap("pgx.CollectRows", err)
}

const addPetAchievementsQuery = `
	INSERT INTO pet_achievements (
		user_id,
		achievement,
		got_at
	)
	SELECT $1, unnest($2::text[]), $3
	ON CONFLICT (user_id, achievement) DO NOTHING
`

func (r *PetRepository) Achieve(ctx context.Context, userID uuid.UUID, got []user.PetAchievement, now time.Time) error {
	names := make([]string, len(got))
	for i, a := range got {
		names[i] = string(a)
	}

	_, err := r.conn(ctx).Exec(ctx, addPetAchievementsQuery, userID, names, now)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}

	return errs.Wrap("r.conn.Exec", err)
}

const listPetSalesQuery = `
	SELECT p.user_id,` + petColumns + `
	FROM pets AS p
	JOIN users AS u ON u.id = p.user_id
	WHERE p.sale_price > 0
		AND ` + petShownAt + `
	ORDER BY p.sale_price, p.id
	LIMIT $2
`

func (r *PetRepository) Listings(ctx context.Context, now time.Time, limit int) ([]user.PetContender, error) {
	return r.ownedPets(ctx, listPetSalesQuery, now, limit)
}

const lockListedPetQuery = `
	SELECT user_id,` + petColumns + `
	FROM pets
	WHERE id = $1
	FOR UPDATE
`

func (r *PetRepository) LockListed(ctx context.Context, petID uuid.UUID) (user.PetContender, error) {
	var owner uuid.UUID
	p, err := scanPet(ownedRow{row: r.conn(ctx).QueryRow(ctx, lockListedPetQuery, petID), owner: &owner})
	if errors.Is(err, pgx.ErrNoRows) {
		return user.PetContender{}, user.ErrPetNotFound
	}
	if err != nil {
		return user.PetContender{}, errs.Wrap("scanPet", err)
	}

	return user.PetContender{Owner: owner, Pet: p}, nil
}

// movePetQuery hands the pet $1 to $2 unless they keep $3 already.
const movePetQuery = `
	UPDATE pets
	SET user_id = $2
	WHERE id = $1
		AND (
			SELECT count(*)
			FROM pets
			WHERE user_id = $2
		) < $3
`

func (r *PetRepository) MovePet(ctx context.Context, petID, toID uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, movePetQuery, petID, toID, user.MaxPets)

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
