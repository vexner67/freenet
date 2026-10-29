package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Stocks is the stock exchange looking at the company firmID (none — at
// none).
func (b *Firms) Stocks(ctx context.Context, accessToken, firmID string) (model.Stocks, error) {
	resp, err := b.api.GetStocks(withToken(ctx, accessToken), &userv1.GetStocksRequest{FirmId: firmID})
	if err != nil {
		return model.Stocks{}, errs.Wrap("b.api.GetStocks", fromStatus(err))
	}

	return fromProtoStocks(resp), nil
}

func (b *Firms) RunStocks(ctx context.Context, accessToken string, act model.StockAct) (model.StockDone, error) {
	resp, err := b.api.RunStocks(withToken(ctx, accessToken), &userv1.RunStocksRequest{
		Action: act.Action, FirmId: act.FirmID, Buy: act.Buy, Price: act.Price, Shares: act.Shares, Votes: act.Votes,
		OrderId: act.OrderID,
	})
	if err != nil {
		return model.StockDone{}, errs.Wrap("b.api.RunStocks", fromStatus(err))
	}

	return model.StockDone{
		Spent: resp.GetSpent(), Got: resp.GetGot(), Balance: resp.GetBalance(), Shares: resp.GetShares(),
		Money: resp.GetMoney(), Stocks: fromProtoStocks(resp.GetStocks()),
	}, nil
}

func fromProtoLevels(levels []*userv1.StockLevel) []model.StockLevel {
	out := make([]model.StockLevel, 0, len(levels))
	for _, l := range levels {
		out = append(out, model.StockLevel{Price: l.GetPrice(), Shares: l.GetShares(), Orders: int(l.GetOrders())})
	}

	return out
}

func fromProtoStocks(s *userv1.StocksState) model.Stocks {
	out := model.Stocks{
		News: fromProtoNews(s.GetNews()), Fee: int(s.GetFee()), Account: s.GetAccount(), FirmID: s.GetFirmId(),
		Bids: fromProtoLevels(s.GetBids()), Asks: fromProtoLevels(s.GetAsks()), NextHour: timeOf(s.GetNextHour()),
		Tenders: fromProtoTenders(s.GetTenders()),
	}
	for _, c := range s.GetCompanies() {
		company := model.StockCompany{
			FirmID: c.GetFirmId(), Name: c.GetName(), Kind: c.GetKind(), Level: int(c.GetLevel()), DirectorID: c.GetDirectorId(),
			Shares: c.GetShares(), Pool: c.GetPool(), Float: c.GetFloat(), Book: c.GetBook(), Fair: c.GetFair(),
			Price: c.GetPrice(), Bid: c.GetBid(), Ask: c.GetAsk(), BestBid: c.GetBestBid(), BestAsk: c.GetBestAsk(),
			DayAgo: c.GetDayAgo(), Volume: c.GetVolume(), Profit: c.GetProfit(), Holders: int(c.GetHolders()),
			BandLow: c.GetBandLow(), BandHigh: c.GetBandHigh(),
		}
		for _, k := range c.GetCandles() {
			company.Candles = append(company.Candles, model.StockCandle{
				Hour: timeOf(k.GetHour()), Open: k.GetOpen(), High: k.GetHigh(), Low: k.GetLow(), Close: k.GetClose(),
				Volume: k.GetVolume(),
			})
		}
		out.Companies = append(out.Companies, company)
	}
	for _, p := range s.GetIndex() {
		out.Index = append(out.Index, model.StockIndexPoint{Hour: timeOf(p.GetHour()), Value: p.GetValue()})
	}
	for _, h := range s.GetHoldings() {
		out.Holdings = append(out.Holdings, model.StockHolding{FirmID: h.GetFirmId(), Shares: h.GetShares(), Free: h.GetFree()})
	}
	for _, o := range s.GetOrders() {
		out.Orders = append(out.Orders, model.StockOrder{
			ID: o.GetId(), FirmID: o.GetFirmId(), Buy: o.GetBuy(), Price: o.GetPrice(), Shares: o.GetShares(),
			PlacedAt: timeOf(o.GetPlacedAt()),
		})
	}

	return out
}
