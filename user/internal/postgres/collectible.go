package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

const listGiftStockQuery = `
	SELECT
		gift,
		supply,
		sold,
		minted
	FROM gift_stock
`

func (r *GiftRepository) Stocks(ctx context.Context) (map[user.GiftKind]user.GiftStock, error) {
	rows, err := r.conn(ctx).Query(ctx, listGiftStockQuery)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	stocks := make(map[user.GiftKind]user.GiftStock)

	for rows.Next() {
		var (
			kind                 string
			supply, sold, minted int
		)

		if err := rows.Scan(&kind, &supply, &sold, &minted); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		stocks[user.GiftKind(kind)] = user.GiftStock{Supply: user.GiftSupply(supply), Sold: sold, Minted: minted}
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return stocks, nil
}

const setGiftSupplyQuery = `
	INSERT INTO gift_stock (
		gift,
		supply
	)
	VALUES ($1, $2)
	ON CONFLICT (gift) DO UPDATE
	SET supply = EXCLUDED.supply
`

func (r *GiftRepository) SetSupply(ctx context.Context, kind user.GiftKind, supply user.GiftSupply) error {
	if _, err := r.conn(ctx).Exec(ctx, setGiftSupplyQuery, kind.String(), supply.Int()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// The edition is checked by the statement that counts the gift: two
// sending the last one at once do not both get it.
const sellGiftQuery = `
	INSERT INTO gift_stock (
		gift,
		sold
	)
	VALUES ($1, 1)
	ON CONFLICT (gift) DO UPDATE
	SET sold = gift_stock.sold + 1
	WHERE gift_stock.supply = 0
		OR gift_stock.sold < gift_stock.supply
	RETURNING sold
`

func (r *GiftRepository) Sell(ctx context.Context, kind user.GiftKind) error {
	var sold int

	err := r.conn(ctx).QueryRow(ctx, sellGiftQuery, kind.String()).Scan(&sold)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.ErrGiftSoldOut
	}
	if err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	return nil
}

const mintGiftQuery = `
	INSERT INTO gift_stock (
		gift,
		minted
	)
	VALUES ($1, 1)
	ON CONFLICT (gift) DO UPDATE
	SET minted = gift_stock.minted + 1
	RETURNING minted
`

func (r *GiftRepository) Mint(ctx context.Context, kind user.GiftKind) (int, error) {
	var number int

	if err := r.conn(ctx).QueryRow(ctx, mintGiftQuery, kind.String()).Scan(&number); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return number, nil
}

const lockGiftQuery = `
	SELECT` + giftColumns + `
	FROM gifts
	WHERE id = $1
	FOR UPDATE
`

func (r *GiftRepository) Lock(ctx context.Context, id uuid.UUID) (*user.Gift, error) {
	g, err := scanGift(r.conn(ctx).QueryRow(ctx, lockGiftQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrGiftNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanGift", err)
	}

	return g, nil
}

const updateGiftQuery = `
	UPDATE gifts
	SET recipient_id = $2,
		sender_id = $3,
		caption = $4,
		anonymous = $5,
		number = $6,
		model = $7,
		backdrop = $8,
		symbol = $9,
		sale_price = $10
	WHERE id = $1
`

func (r *GiftRepository) Update(ctx context.Context, g *user.Gift) error {
	number, model, backdrop, symbol, price := collectibleArgs(g)

	tag, err := r.conn(ctx).Exec(ctx, updateGiftQuery,
		g.ID(), nullID(g.RecipientID()), nullID(g.SenderID()), g.Caption().String(), g.Anonymous(),
		number, model, backdrop, symbol, price)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return user.ErrGiftNotFound
	}

	return nil
}

// The market: gifts on sale of pages neither deleted nor banned now. The
// order is one of three fixed clauses; every value is a parameter.
const marketQuery = `
	SELECT` + prefixedGiftColumns + `
	FROM gifts AS g
	JOIN users AS u ON u.id = g.recipient_id
	WHERE g.sale_price IS NOT NULL
		AND u.deactivated_at IS NULL
		AND (u.banned_at IS NULL OR u.banned_until <= now())
		AND ($1 = '' OR g.gift = $1)
		AND ($2 = '' OR g.model = $2)
		AND ($3 = '' OR g.backdrop = $3)
		AND ($4 = '' OR g.symbol = $4)
		AND (NOT $5 OR `

const prefixedGiftColumns = `
		g.id,
		g.recipient_id,
		g.sender_id,
		g.gift,
		g.caption,
		g.anonymous,
		g.number,
		g.model,
		g.backdrop,
		g.symbol,
		g.sale_price`

var marketOrders = map[user.MarketSort]string{
	user.MarketCheapest: `(g.sale_price, g.id) > ($6, $7))
	ORDER BY g.sale_price, g.id
	LIMIT $8`,
	user.MarketDearest: `(g.sale_price, g.id) < ($6, $7))
	ORDER BY g.sale_price DESC, g.id DESC
	LIMIT $8`,
	user.MarketByNumber: `(g.number, g.id) > ($6, $7))
	ORDER BY g.number, g.id
	LIMIT $8`,
}

func (r *GiftRepository) Market(ctx context.Context, f user.MarketFilter, after *user.MarketKey, limit int) ([]*user.Gift, error) {
	order, ok := marketOrders[f.Sort]
	if !ok {
		return nil, user.ErrMarketSort
	}

	var key user.MarketKey
	if after != nil {
		key = *after
	}

	rows, err := r.conn(ctx).Query(ctx, marketQuery+order,
		f.Kind.String(), f.Model, f.Backdrop, f.Symbol, after != nil, key.Value, key.ID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var gifts []*user.Gift

	for rows.Next() {
		g, err := scanGift(rows)
		if err != nil {
			return nil, errs.Wrap("scanGift", err)
		}

		gifts = append(gifts, g)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return gifts, nil
}

const saveGiftEventQuery = `
	INSERT INTO gift_history (
		id,
		gift_id,
		kind,
		from_id,
		to_id,
		price
	)
	VALUES ($1, $2, $3, $4, $5, $6)
`

func (r *GiftRepository) SaveEvent(ctx context.Context, e user.GiftEvent) error {
	_, err := r.conn(ctx).Exec(ctx, saveGiftEventQuery,
		e.ID, e.GiftID, string(e.Kind), nullID(e.FromID), nullID(e.ToID), e.Price)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const listGiftHistoryQuery = `
	SELECT
		id,
		gift_id,
		kind,
		from_id,
		to_id,
		price
	FROM gift_history
	WHERE gift_id = $1
	ORDER BY id DESC
	LIMIT $2
`

func (r *GiftRepository) History(ctx context.Context, giftID uuid.UUID, limit int) ([]user.GiftEvent, error) {
	rows, err := r.conn(ctx).Query(ctx, listGiftHistoryQuery, giftID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var events []user.GiftEvent

	for rows.Next() {
		var (
			e            user.GiftEvent
			kind         string
			fromID, toID *uuid.UUID
		)

		if err := rows.Scan(&e.ID, &e.GiftID, &kind, &fromID, &toID, &e.Price); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		e.Kind, e.FromID, e.ToID = user.GiftEventKind(kind), idOrNil(fromID), idOrNil(toID)
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return events, nil
}

const saveAuctionQuery = `
	INSERT INTO gift_auctions (
		gift_id,
		seller_id,
		start_price,
		ends_at,
		top_bid,
		bidder_id,
		bids
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (gift_id) DO UPDATE
	SET ends_at = EXCLUDED.ends_at,
		top_bid = EXCLUDED.top_bid,
		bidder_id = EXCLUDED.bidder_id,
		bids = EXCLUDED.bids
`

func (r *GiftRepository) SaveAuction(ctx context.Context, a *user.GiftAuction) error {
	_, err := r.conn(ctx).Exec(ctx, saveAuctionQuery,
		a.GiftID(), nullID(a.SellerID()), a.Start(), a.EndsAt(), a.TopBid(), nullID(a.BidderID()), a.Bids())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const auctionColumns = `
		gift_id,
		seller_id,
		start_price,
		ends_at,
		top_bid,
		bidder_id,
		bids`

const getAuctionQuery = `
	SELECT` + auctionColumns + `
	FROM gift_auctions
	WHERE gift_id = $1
`

func (r *GiftRepository) Auction(ctx context.Context, giftID uuid.UUID) (*user.GiftAuction, error) {
	return r.auction(ctx, getAuctionQuery, giftID)
}

const lockAuctionQuery = getAuctionQuery + `
	FOR UPDATE
`

func (r *GiftRepository) LockAuction(ctx context.Context, giftID uuid.UUID) (*user.GiftAuction, error) {
	return r.auction(ctx, lockAuctionQuery, giftID)
}

func (r *GiftRepository) auction(ctx context.Context, query string, giftID uuid.UUID) (*user.GiftAuction, error) {
	a, err := scanAuction(r.conn(ctx).QueryRow(ctx, query, giftID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrAuctionNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAuction", err)
	}

	return a, nil
}

const deleteAuctionQuery = `
	DELETE FROM gift_auctions
	WHERE gift_id = $1
`

func (r *GiftRepository) DeleteAuction(ctx context.Context, giftID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteAuctionQuery, giftID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const listAuctionsQuery = `
	SELECT` + auctionColumns + `
	FROM gift_auctions
	WHERE ends_at > $1
		AND (NOT $2 OR (ends_at, gift_id) > ($3, $4))
	ORDER BY ends_at, gift_id
	LIMIT $5
`

func (r *GiftRepository) Auctions(ctx context.Context, now time.Time, after *service.AuctionKey, limit int) ([]*user.GiftAuction, error) {
	var key service.AuctionKey
	if after != nil {
		key = *after
	}

	rows, err := r.conn(ctx).Query(ctx, listAuctionsQuery, now, after != nil, key.EndsAt, key.GiftID, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var auctions []*user.GiftAuction

	for rows.Next() {
		a, err := scanAuction(rows)
		if err != nil {
			return nil, errs.Wrap("scanAuction", err)
		}

		auctions = append(auctions, a)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return auctions, nil
}

const listDueAuctionsQuery = `
	SELECT gift_id
	FROM gift_auctions
	WHERE ends_at <= $1
	ORDER BY ends_at, gift_id
	LIMIT $2
`

func (r *GiftRepository) DueAuctions(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, listDueAuctionsQuery, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

const listAuctionedQuery = `
	SELECT gift_id
	FROM gift_auctions
	WHERE gift_id = ANY($1)
`

func (r *GiftRepository) AuctionedIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := r.conn(ctx).Query(ctx, listAuctionedQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	found, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	auctioned := make(map[uuid.UUID]bool, len(found))
	for _, id := range found {
		auctioned[id] = true
	}

	return auctioned, nil
}

func scanAuction(row pgx.Row) (*user.GiftAuction, error) {
	var (
		giftID             uuid.UUID
		sellerID, bidderID *uuid.UUID
		start, topBid      int64
		endsAt             time.Time
		bids               int
	)

	if err := row.Scan(&giftID, &sellerID, &start, &endsAt, &topBid, &bidderID, &bids); err != nil {
		return nil, err
	}

	return user.RestoreGiftAuction(giftID, idOrNil(sellerID), start, endsAt.UTC(), topBid, idOrNil(bidderID), bids), nil
}
