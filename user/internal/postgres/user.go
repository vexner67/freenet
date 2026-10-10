package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

const (
	uniqueViolationCode = "23505"
	partnerForeignKey   = "users_partner_fkey"

	// Unique indexes from migrations; the name tells which field clashed.
	emailIndex    = "users_email_uidx"
	nicknameIndex = "users_nickname_uidx"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		pool: pool,
	}
}

const saveUserQuery = `
	INSERT INTO users (
		id,
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		avatar_id,
		clubs_read
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
`

func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	p := u.Profile()
	pr := u.Privacy()

	if _, err := r.pool.Exec(
		ctx,
		saveUserQuery,
		u.ID(),
		u.Email().String(),
		p.Name.String(),
		p.Surname.String(),
		p.Nickname.String(),
		p.Status.String(),
		p.Gender.String(),
		p.Relationship.String(),
		p.Worldview.String(),
		p.PoliticalViews.String(),
		birthdayToDB(p.Birthday),
		pr.ShowBirthday,
		pr.ShowRelationship,
		pr.ShowWorldview,
		pr.ShowPoliticalViews,
		pr.WallWrite.String(),
		pr.MessagesWrite.String(),
		idToDB(u.AvatarID()),
		pr.ClubsRead.String(),
	); err != nil {
		return errs.Wrap("r.pool.Exec", translateError(err))
	}

	return nil
}

const updateUserQuery = `
	UPDATE users
	SET
		name = $2,
		surname = $3,
		nickname = $4,
		status = $5,
		gender = $6,
		relationship = $7,
		worldview = $8,
		political_views = $9,
		birthday = $10,
		show_birthday = $11,
		show_relationship = $12,
		show_worldview = $13,
		show_political_views = $14,
		wall_write = $15,
		messages_write = $16,
		avatar_id = $17,
		deactivated_at = $18,
		purge_at = $19,
		deactivation_reason = $20,
		clubs_read = $21,
		hometown = $22,
		relationship_partner_id = $23,
		hide_birth_year = $24,
		contact_email = $25,
		telegram = $26,
		website = $27,
		city = $28,
		address = $29,
		interests = $30,
		fav_music = $31,
		fav_films = $32,
		fav_shows = $33,
		fav_books = $34,
		fav_quotes = $35,
		fav_games = $36,
		about = $37,
		extra_fields = $38,
		page_read = $39,
		info_read = $40,
		photos_read = $41,
		friends_read = $42,
		online_read = $43,
		avatar_style = $44,
		main_page = $45,
		hidden_menu = $46,
		updated_at = now()
	WHERE id = $1
`

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	p := u.Profile()
	pr := u.Privacy()
	d := u.Deactivation()
	c := u.Contacts()
	in := u.Interests()
	ui := u.Interface()

	fields, err := extraFieldsToDB(u.ExtraFields())
	if err != nil {
		return errs.Wrap("extraFieldsToDB", err)
	}

	tag, err := r.pool.Exec(
		ctx,
		updateUserQuery,
		u.ID(),
		p.Name.String(),
		p.Surname.String(),
		p.Nickname.String(),
		p.Status.String(),
		p.Gender.String(),
		p.Relationship.String(),
		p.Worldview.String(),
		p.PoliticalViews.String(),
		birthdayToDB(p.Birthday),
		pr.ShowBirthday,
		pr.ShowRelationship,
		pr.ShowWorldview,
		pr.ShowPoliticalViews,
		pr.WallWrite.String(),
		pr.MessagesWrite.String(),
		idToDB(u.AvatarID()),
		timeToDB(d.At),
		timeToDB(d.PurgeAt),
		d.Reason.String(),
		pr.ClubsRead.String(),
		p.Hometown.String(),
		idToDB(p.Partner),
		p.HideBirthYear,
		c.Email.String(),
		c.Telegram.String(),
		c.Website.String(),
		c.City.String(),
		c.Address.String(),
		in.Interests.String(),
		in.Music.String(),
		in.Films.String(),
		in.Shows.String(),
		in.Books.String(),
		in.Quotes.String(),
		in.Games.String(),
		in.About.String(),
		fields,
		pr.PageRead.String(),
		pr.InfoRead.String(),
		pr.PhotosRead.String(),
		pr.FriendsRead.String(),
		pr.OnlineRead.String(),
		ui.AvatarStyle.String(),
		ui.MainPage.String(),
		hiddenMenuToDB(ui.HiddenMenu),
	)
	if err != nil {
		return errs.Wrap("r.pool.Exec", translateError(err))
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}

// markSeenQuery keeps the latest visit: requests may be told out of order.
const markSeenQuery = `
	UPDATE users
	SET last_seen_at = $2
	WHERE id = $1
		AND (last_seen_at IS NULL OR last_seen_at < $2)
`

func (r *UserRepository) MarkSeen(ctx context.Context, id uuid.UUID, at time.Time) error {
	if _, err := r.pool.Exec(ctx, markSeenQuery, id, at); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

// countUsersQuery reads the whole table: the page "о сайте" is opened
// seldom and the table is small.
const countUsersQuery = `
	SELECT
		count(*),
		count(*) FILTER (WHERE last_seen_at > $1)
	FROM users
	WHERE deactivated_at IS NULL
`

func (r *UserRepository) Count(ctx context.Context, since time.Time) (service.Statistics, error) {
	var stats service.Statistics

	if err := r.pool.QueryRow(ctx, countUsersQuery, since).Scan(&stats.Users, &stats.Online); err != nil {
		return service.Statistics{}, errs.Wrap("r.pool.QueryRow", err)
	}

	return stats, nil
}

// setBackdropQueries change one side of a page each, so that two changes
// of the two sides do not undo each other, and return what the side
// showed before: the row is locked by the subquery, so a second change of
// the side waits and reads the first one's photo.
var setBackdropQueries = map[user.BackdropSide]string{
	user.BackdropLeft: `
	UPDATE users u
	SET backdrop_left = $2
	FROM (SELECT id, backdrop_left FROM users WHERE id = $1 FOR UPDATE) old
	WHERE u.id = old.id
	RETURNING old.backdrop_left
`,
	user.BackdropRight: `
	UPDATE users u
	SET backdrop_right = $2
	FROM (SELECT id, backdrop_right FROM users WHERE id = $1 FOR UPDATE) old
	WHERE u.id = old.id
	RETURNING old.backdrop_right
`,
}

func (r *UserRepository) SetBackdrop(ctx context.Context, id uuid.UUID, side user.BackdropSide, photoID uuid.UUID) (uuid.UUID, error) {
	var previous *uuid.UUID

	err := r.pool.QueryRow(ctx, setBackdropQueries[side], id, idToDB(photoID)).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, user.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, errs.Wrap("r.pool.QueryRow", err)
	}

	return idFromDB(previous), nil
}

// userColumns are what scanUser reads, in its order.
const userColumns = `
		id,
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		clubs_read,
		avatar_id,
		deactivated_at,
		purge_at,
		deactivation_reason,
		hometown,
		relationship_partner_id,
		hide_birth_year,
		contact_email,
		telegram,
		website,
		city,
		address,
		interests,
		fav_music,
		fav_films,
		fav_shows,
		fav_books,
		fav_quotes,
		fav_games,
		about,
		extra_fields,
		page_read,
		info_read,
		photos_read,
		friends_read,
		online_read,
		last_seen_at,
		backdrop_left,
		backdrop_right,
		avatar_style,
		main_page,
		hidden_menu`

// qualifiedUserColumns are userColumns of the users table joined as u.
const qualifiedUserColumns = `
		u.id,
		u.email,
		u.name,
		u.surname,
		u.nickname,
		u.status,
		u.gender,
		u.relationship,
		u.worldview,
		u.political_views,
		u.birthday,
		u.show_birthday,
		u.show_relationship,
		u.show_worldview,
		u.show_political_views,
		u.wall_write,
		u.messages_write,
		u.clubs_read,
		u.avatar_id,
		u.deactivated_at,
		u.purge_at,
		u.deactivation_reason,
		u.hometown,
		u.relationship_partner_id,
		u.hide_birth_year,
		u.contact_email,
		u.telegram,
		u.website,
		u.city,
		u.address,
		u.interests,
		u.fav_music,
		u.fav_films,
		u.fav_shows,
		u.fav_books,
		u.fav_quotes,
		u.fav_games,
		u.about,
		u.extra_fields,
		u.page_read,
		u.info_read,
		u.photos_read,
		u.friends_read,
		u.online_read,
		u.last_seen_at,
		u.backdrop_left,
		u.backdrop_right,
		u.avatar_style,
		u.main_page,
		u.hidden_menu`

const getUserByIDQuery = `
	SELECT` + userColumns + `
	FROM users
	WHERE id = $1
`

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, getUserByIDQuery, id))
	if err != nil {
		return nil, errs.Wrap("scanUser", err)
	}

	return u, nil
}

const getUserByNicknameQuery = `
	SELECT` + userColumns + `
	FROM users
	WHERE lower(nickname) = lower($1)
		AND nickname <> ''
`

func (r *UserRepository) GetByNickname(ctx context.Context, nickname user.Nickname) (*user.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, getUserByNicknameQuery, nickname.String()))
	if err != nil {
		return nil, errs.Wrap("scanUser", err)
	}

	return u, nil
}

const getUsersByIDsQuery = `
	SELECT` + userColumns + `
	FROM users
	WHERE id = ANY($1)
`

func (r *UserRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*user.User, error) {
	rows, err := r.pool.Query(ctx, getUsersByIDsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	users := make([]*user.User, 0, len(ids))
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, errs.Wrap("scanUser", err)
		}
		users = append(users, u)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return users, nil
}

const getUserIDByEmailQuery = `
	SELECT id
	FROM users
	WHERE lower(email) = lower($1)
`

func (r *UserRepository) GetIDByEmail(ctx context.Context, email user.Email) (uuid.UUID, error) {
	var id uuid.UUID

	err := r.pool.QueryRow(ctx, getUserIDByEmailQuery, email.String()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, user.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, errs.Wrap("r.pool.QueryRow", err)
	}

	return id, nil
}

func scanUser(row pgx.Row) (*user.User, error) {
	var (
		id                                         uuid.UUID
		email, name, surname, nickname, status     string
		gender, relationship, worldview, political string
		birthday                                   *time.Time
		privacy                                    user.Privacy
		wallWrite, messagesWrite, clubsRead        string
		avatarID                                   *uuid.UUID
		deactivatedAt, purgeAt                     *time.Time
		reason                                     string
		partner                                    *uuid.UUID
		hometown                                   string
		contacts                                   [5]string
		interests                                  [8]string
		extraFields                                []byte
		hideBirthYear                              bool
		reads                                      [5]string
		lastSeen                                   *time.Time
		backdrop                                   [2]*uuid.UUID
		avatarStyle, mainPage                      string
		hiddenMenu                                 []string
	)

	err := row.Scan(
		&id,
		&email,
		&name,
		&surname,
		&nickname,
		&status,
		&gender,
		&relationship,
		&worldview,
		&political,
		&birthday,
		&privacy.ShowBirthday,
		&privacy.ShowRelationship,
		&privacy.ShowWorldview,
		&privacy.ShowPoliticalViews,
		&wallWrite,
		&messagesWrite,
		&clubsRead,
		&avatarID,
		&deactivatedAt,
		&purgeAt,
		&reason,
		&hometown,
		&partner,
		&hideBirthYear,
		&contacts[0], &contacts[1], &contacts[2], &contacts[3], &contacts[4],
		&interests[0], &interests[1], &interests[2], &interests[3],
		&interests[4], &interests[5], &interests[6], &interests[7],
		&extraFields,
		&reads[0], &reads[1], &reads[2], &reads[3], &reads[4],
		&lastSeen,
		&backdrop[0], &backdrop[1],
		&avatarStyle, &mainPage, &hiddenMenu,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}

	// Data comes from our own table and was validated on write.
	privacy.WallWrite = user.WallWrite(wallWrite)
	privacy.MessagesWrite = user.MessagesWrite(messagesWrite)
	privacy.ClubsRead = user.ClubsRead(clubsRead)
	privacy.PageRead, privacy.InfoRead = user.Audience(reads[0]), user.Audience(reads[1])
	privacy.PhotosRead, privacy.FriendsRead = user.Audience(reads[2]), user.Audience(reads[3])
	privacy.OnlineRead = user.Audience(reads[4])

	fields, err := extraFieldsFromDB(extraFields)
	if err != nil {
		return nil, errs.Wrap("extraFieldsFromDB", err)
	}

	return user.Restore(user.Stored{
		ID:    id,
		Email: user.Email(email),
		Profile: user.Profile{
			Name:           user.Name(name),
			Surname:        user.Surname(surname),
			Nickname:       user.Nickname(nickname),
			Status:         user.Status(status),
			Gender:         user.Gender(gender),
			Relationship:   user.Relationship(relationship),
			Worldview:      user.Worldview(worldview),
			PoliticalViews: user.PoliticalViews(political),
			Birthday:       birthdayFromDB(birthday),
			HideBirthYear:  hideBirthYear,
			Hometown:       user.Place(hometown),
			Partner:        idFromDB(partner),
		},
		Contacts: user.Contacts{
			Email:    user.Email(contacts[0]),
			Telegram: user.Telegram(contacts[1]),
			Website:  user.Website(contacts[2]),
			City:     user.Place(contacts[3]),
			Address:  user.Address(contacts[4]),
		},
		Interests: user.Interests{
			Interests: user.About(interests[0]),
			Music:     user.About(interests[1]),
			Films:     user.About(interests[2]),
			Shows:     user.About(interests[3]),
			Books:     user.About(interests[4]),
			Quotes:    user.About(interests[5]),
			Games:     user.About(interests[6]),
			About:     user.About(interests[7]),
		},
		ExtraFields: fields,
		Privacy:     privacy,
		AvatarID:    idFromDB(avatarID),
		Deactivation: user.Deactivation{
			At:      timeFromDB(deactivatedAt),
			PurgeAt: timeFromDB(purgeAt),
			Reason:  user.DeactivationReason(reason),
		},
		LastSeen: timeFromDB(lastSeen),
		Backdrop: user.Backdrop{Left: idFromDB(backdrop[0]), Right: idFromDB(backdrop[1])},
		Interface: user.Interface{
			AvatarStyle: user.AvatarStyle(avatarStyle),
			MainPage:    user.MainPage(mainPage),
			HiddenMenu:  hiddenMenuFromDB(hiddenMenu),
		},
	}), nil
}

// hiddenMenuToDB is never NULL: an empty array for a whole menu.
func hiddenMenuToDB(items []user.MenuItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.String())
	}

	return out
}

// hiddenMenuFromDB is nil for a whole menu, as a new user has it.
func hiddenMenuFromDB(items []string) []user.MenuItem {
	if len(items) == 0 {
		return nil
	}

	out := make([]user.MenuItem, 0, len(items))
	for _, item := range items {
		out = append(out, user.MenuItem(item))
	}

	return out
}

// An unset birthday is stored as NULL rather than year 1.
func birthdayToDB(b user.Birthday) *time.Time {
	if b.Time().IsZero() {
		return nil
	}

	t := b.Time()

	return &t
}

func birthdayFromDB(t *time.Time) user.Birthday {
	if t == nil {
		return user.Birthday{}
	}

	return user.Birthday(*t)
}

// extraFieldRow is an additional field as the extra_fields array keeps it.
type extraFieldRow struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	Place string `json:"place"`
}

func extraFieldsToDB(fields []user.ExtraField) ([]byte, error) {
	rows := make([]extraFieldRow, 0, len(fields))
	for _, f := range fields {
		rows = append(rows, extraFieldRow{Name: f.Name, Text: f.Text.String(), Place: f.Place.String()})
	}

	b, err := json.Marshal(rows)
	if err != nil {
		return nil, errs.Wrap("json.Marshal", err)
	}

	return b, nil
}

func extraFieldsFromDB(b []byte) ([]user.ExtraField, error) {
	var rows []extraFieldRow
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Data comes from our own table and was validated on write.
	fields := make([]user.ExtraField, 0, len(rows))
	for _, r := range rows {
		fields = append(fields, user.ExtraField{Name: r.Name, Text: user.About(r.Text), Place: user.FieldPlace(r.Place)})
	}

	return fields, nil
}

// No photo, or no partner, is stored as NULL.
func idToDB(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

func idFromDB(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}

	return *id
}

// timeToDB stores the zero time as NULL.
func timeToDB(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}

func timeFromDB(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.UTC()
}

// searchUsersSelect is completed by searchUsersQuery with one LIKE per word.
const searchUsersSelect = `
	SELECT` + userColumns + `
	FROM users
	WHERE nickname <> ''
		AND deactivated_at IS NULL
		AND id < $1`

// searchHaystack must match the expression of users_search_idx exactly,
// or the trigram index is not used.
const searchHaystack = `lower(nickname || ' ' || name || ' ' || surname)`

// searchCondition is one condition of a search: an SQL expression with %d
// for the placeholder of its parameter.
type searchCondition struct {
	sql   string
	param string
}

// searchConditions are the words of q, one "LIKE $n" each: unlike LIKE ALL
// (array), such conditions are served by the trigram index; then its
// filters: a hometown or a city as typed, whatever the case, a favourite
// anywhere in its field.
func searchConditions(q user.SearchQuery) []searchCondition {
	var conds []searchCondition
	for _, w := range q.Words() {
		conds = append(conds, searchCondition{searchHaystack + " LIKE $%d", "%" + escapeLike(w) + "%"})
	}

	// The filters look into the information, which only the pages open
	// to everyone tell.
	f := q.Filters()
	if f != (user.SearchFilters{}) {
		conds = append(conds, searchCondition{"info_read = $%d", user.AudienceEveryone.String()})
	}

	for _, c := range []searchCondition{
		{"lower(hometown) = $%d", f.Hometown},
		{"lower(city) = $%d", f.City},
		{"lower(fav_films) LIKE $%d", f.Film},
		{"lower(fav_shows) LIKE $%d", f.Show},
		{"lower(fav_books) LIKE $%d", f.Book},
	} {
		if c.param == "" {
			continue
		}
		if strings.Contains(c.sql, "LIKE") {
			c.param = "%" + escapeLike(c.param) + "%"
		}
		conds = append(conds, c)
	}

	return conds
}

// searchUsersQuery ANDs the conditions. Only placeholders are added to the
// text; the values are passed as parameters.
func searchUsersQuery(conds []searchCondition) string {
	var b strings.Builder

	b.WriteString(searchUsersSelect)
	for i, c := range conds {
		b.WriteString("\n\t\tAND ")
		fmt.Fprintf(&b, c.sql, i+3)
	}
	b.WriteString("\n\tORDER BY id DESC\n\tLIMIT $2")

	return b.String()
}

func (r *UserRepository) Search(ctx context.Context, q user.SearchQuery, before uuid.UUID, limit int) ([]*user.User, error) {
	if before == uuid.Nil {
		before = uuid.Max
	}

	conds := searchConditions(q)
	args := []any{before, limit}
	for _, c := range conds {
		args = append(args, c.param)
	}

	rows, err := r.pool.Query(ctx, searchUsersQuery(conds), args...)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var users []*user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, errs.Wrap("scanUser", err)
		}
		users = append(users, u)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return users, nil
}

// escapeLike makes % and _ in a search word match themselves.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}

func translateError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	// The partner was erased since the nickname was looked up.
	if pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == partnerForeignKey {
		return user.ErrPartnerNotFound
	}

	if pgErr.Code != uniqueViolationCode {
		return err
	}

	switch pgErr.ConstraintName {
	case emailIndex:
		return user.ErrAlreadyExists
	case nicknameIndex:
		return user.ErrNicknameTaken
	default:
		return err
	}
}

// deleteDueUsersQuery deletes up to $2 pages due to be erased by $1; SKIP
// LOCKED lets replicas purge side by side without taking the same ones.
const deleteDueUsersQuery = `
	DELETE FROM users
	WHERE id IN (
		SELECT id
		FROM users
		WHERE purge_at <= $1
		ORDER BY purge_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	RETURNING id
`

func (r *UserRepository) DeleteDue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, deleteDueUsersQuery, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *UserRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}
