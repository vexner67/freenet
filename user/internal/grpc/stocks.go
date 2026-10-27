package grpc

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// dayCandles are the candles of a company not looked at: its day.
	dayCandles = 24
	// bookLevels are the prices of each side of the order book shown.
	bookLevels = 10
)

func toProtoCandles(candles []user.Candle) []*userv1.StockCandle {
	out := make([]*userv1.StockCandle, len(candles))
	for i, k := range candles {
		out[i] = &userv1.StockCandle{
			Hour: timestamppb.New(k.Hour), Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume,
		}
	}

	return out
}

func toProtoCompany(v service.FirmsView, f *user.Firm, looked bool) *userv1.StockCompany {
	q := v.City.Quote(v.Rules, f, v.At)
	candles := f.Candles()
	if !looked && len(candles) > dayCandles {
		candles = candles[len(candles)-dayCandles:]
	}

	holders := 0
	for _, h := range v.City.Holdings(f.ID()) {
		if h.UserID != user.MarketID {
			holders++
		}
	}

	return &userv1.StockCompany{
		FirmId: f.ID().String(), Name: f.Name().String(), Kind: f.Kind().String(), Level: int32(f.Level()), //nolint:gosec // at most user.MaxFirmLevels
		DirectorId: f.DirectorID().String(), Shares: q.Shares, Pool: q.Pool, Float: q.Float, Book: q.Book, Fair: q.Fair,
		Price: q.Price, Bid: q.Bid, Ask: q.Ask, BestBid: q.BestBid, BestAsk: q.BestAsk, DayAgo: q.DayAgo, Volume: q.Volume,
		Profit: f.Profit(), Holders: int32(holders), Candles: toProtoCandles(candles), //nolint:gosec // people of the site
	}
}

// toProtoBook is the order book of the company: the prices people wait
// at, the better first.
func toProtoBook(orders []user.StockOrder, firmID uuid.UUID) (bids, asks []*userv1.StockLevel) {
	side := func(buy bool) []*userv1.StockLevel {
		var levels []*userv1.StockLevel
		for _, o := range orders {
			if o.FirmID != firmID || o.Buy != buy {
				continue
			}

			i := slices.IndexFunc(levels, func(l *userv1.StockLevel) bool { return l.GetPrice() == o.Price })
			if i < 0 {
				levels = append(levels, &userv1.StockLevel{Price: o.Price})
				i = len(levels) - 1
			}
			levels[i].Shares += o.Shares
			levels[i].Orders++
		}

		slices.SortFunc(levels, func(a, b *userv1.StockLevel) int {
			if buy {
				return cmp.Compare(b.GetPrice(), a.GetPrice())
			}
			return cmp.Compare(a.GetPrice(), b.GetPrice())
		})

		return levels[:min(len(levels), bookLevels)]
	}

	return side(true), side(false)
}

func toProtoStocks(v service.FirmsView, userID, firmID uuid.UUID) *userv1.StocksState {
	c := v.City
	state := &userv1.StocksState{
		News: toProtoNews(c.News()), Fee: int32(v.Rules.Fee), Account: c.Account(userID), //nolint:gosec // at most user.MaxFirmFee
		NextHour: timestamppb.New(c.Hour().Add(time.Hour)), Tenders: toProtoTenders(c.Tenders()),
	}

	for _, f := range c.Firms() {
		state.Companies = append(state.Companies, toProtoCompany(v, f, f.ID() == firmID))
	}
	if firmID != uuid.Nil {
		state.FirmId = firmID.String()
		state.Bids, state.Asks = toProtoBook(c.StockOrders(), firmID)
	}
	for _, p := range c.Index() {
		state.Index = append(state.Index, &userv1.StockIndexPoint{Hour: timestamppb.New(p.Hour), Value: p.Value})
	}
	for _, h := range c.HoldingsOf(userID) {
		state.Holdings = append(state.Holdings, &userv1.StockHolding{FirmId: h.FirmID.String(), Shares: h.Shares, Free: c.Free(h.FirmID, userID)})
	}
	for _, o := range c.StockOrders() {
		if o.UserID == userID {
			state.Orders = append(state.Orders, &userv1.StockOrder{
				Id: o.ID.String(), FirmId: o.FirmID.String(), Buy: o.Buy, Price: o.Price, Shares: o.Shares,
				PlacedAt: timestamppb.New(o.PlacedAt),
			})
		}
	}

	return state
}

// lookedAt is the company of firm_id, if any; a company gone shows none.
func lookedAt(v service.FirmsView, firmID string) (uuid.UUID, error) {
	id, err := optionalID(firmID, "enterprise")
	if err != nil {
		return uuid.Nil, err
	}

	if _, err = v.City.Firm(id); err != nil {
		return uuid.Nil, nil //nolint:nilerr // a company closed is no longer looked at
	}

	return id, nil
}

func (h *UserHandler) GetStocks(ctx context.Context, req *userv1.GetStocksRequest) (*userv1.StocksState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	v, err := h.firms.View(ctx, time.Now())
	if err != nil {
		return nil, err
	}

	firmID, err := lookedAt(v, req.GetFirmId())
	if err != nil {
		return nil, err
	}

	return toProtoStocks(v, userID, firmID), nil
}

// stockActions are the actions RunStocks does.
var stockActions = []user.FirmAction{
	user.FirmFund, user.FirmWithdraw, user.FirmStockOrder, user.FirmStockCancel, user.FirmTender, user.FirmTenderSell,
	user.FirmTenderClose,
}

func fromProtoStocksAct(req *userv1.RunStocksRequest) (service.FirmAct, error) {
	action, err := user.NewFirmAction(req.GetAction())
	if err != nil || !slices.Contains(stockActions, action) {
		return service.FirmAct{}, status.Error(codes.InvalidArgument, user.ErrUnknownFirmAction.Error())
	}

	act := service.FirmAct{Action: action, Votes: req.GetVotes()}
	if act.OrderID, err = optionalID(req.GetOrderId(), "order"); err != nil {
		return service.FirmAct{}, err
	}

	firmID, err := optionalID(req.GetFirmId(), "enterprise")
	if err != nil {
		return service.FirmAct{}, err
	}
	act.Stock = user.StockOrder{FirmID: firmID, Buy: req.GetBuy(), Price: req.GetPrice(), Shares: req.GetShares()}

	return act, nil
}

func (h *UserHandler) RunStocks(ctx context.Context, req *userv1.RunStocksRequest) (*userv1.RunStocksResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	act, err := fromProtoStocksAct(req)
	if err != nil {
		return nil, err
	}

	d, err := h.firms.Act(ctx, userID, act, time.Now())
	if err != nil {
		return nil, err
	}

	firmID, err := lookedAt(d.View, req.GetFirmId())
	if err != nil {
		return nil, err
	}

	return &userv1.RunStocksResponse{
		Spent: d.Spent, Got: d.Got, Balance: d.Balance, Shares: d.Fill.Shares, Money: d.Fill.Money,
		Stocks: toProtoStocks(d.View, userID, firmID),
	}, nil
}
