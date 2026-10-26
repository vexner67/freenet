package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

type stockCandleJSON struct {
	Hour   string `json:"hour"`
	Open   int64  `json:"open"`
	High   int64  `json:"high"`
	Low    int64  `json:"low"`
	Close  int64  `json:"close"`
	Volume int64  `json:"volume"`
}

type stockCompanyJSON struct {
	Firm     string                 `json:"firm"`
	Name     string                 `json:"name"`
	Kind     string                 `json:"kind"`
	Level    int                    `json:"level"`
	Director *publicProfileResponse `json:"director"`
	Shares   int64                  `json:"shares"`
	Pool     int64                  `json:"pool"`
	Float    int64                  `json:"float"`
	Book     int64                  `json:"book"`
	Fair     int64                  `json:"fair"`
	Price    int64                  `json:"price"`
	Bid      int64                  `json:"bid"`
	Ask      int64                  `json:"ask"`
	BestBid  int64                  `json:"bestBid"`
	BestAsk  int64                  `json:"bestAsk"`
	DayAgo   int64                  `json:"dayAgo"`
	Volume   int64                  `json:"volume"`
	Profit   int64                  `json:"profit"`
	Holders  int                    `json:"holders"`
	Candles  []stockCandleJSON      `json:"candles"`
}

type stockIndexJSON struct {
	Hour  string `json:"hour"`
	Value int64  `json:"value"`
}

type stockOrderJSON struct {
	ID       string `json:"id"`
	Firm     string `json:"firm"`
	Buy      bool   `json:"buy"`
	Price    int64  `json:"price"`
	Shares   int64  `json:"shares"`
	PlacedAt string `json:"placedAt"`
}

type stockLevelJSON struct {
	Price  int64 `json:"price"`
	Shares int64 `json:"shares"`
	Orders int   `json:"orders"`
}

type stockHoldingJSON struct {
	Firm   string `json:"firm"`
	Shares int64  `json:"shares"`
	Free   int64  `json:"free"`
}

type stocksJSON struct {
	Companies []stockCompanyJSON `json:"companies"`
	Index     []stockIndexJSON   `json:"index"`
	News      []firmEventJSON    `json:"news"`
	Fee       int                `json:"fee"`
	Account   int64              `json:"account"`
	Holdings  []stockHoldingJSON `json:"holdings"`
	Orders    []stockOrderJSON   `json:"orders"`
	Firm      string             `json:"firm"`
	Bids      []stockLevelJSON   `json:"bids"`
	Asks      []stockLevelJSON   `json:"asks"`
	NextHour  string             `json:"nextHour"`
}

// stocksResponse is the stock exchange in JSON with the directors of the
// companies.
func (a *api) stocksResponse(r *http.Request, s model.Stocks) (stocksJSON, error) {
	ids := make([]string, 0, len(s.Companies))
	for _, c := range s.Companies {
		ids = append(ids, c.DirectorID)
	}

	people, err := a.profiles(r, ids)
	if err != nil {
		return stocksJSON{}, err
	}

	level := func(l model.StockLevel) stockLevelJSON { return stockLevelJSON(l) }

	return stocksJSON{
		Companies: listJSON(s.Companies, func(c model.StockCompany) stockCompanyJSON {
			return stockCompanyJSON{
				Firm: c.FirmID, Name: c.Name, Kind: c.Kind, Level: c.Level, Director: person(people, c.DirectorID),
				Shares: c.Shares, Pool: c.Pool, Float: c.Float, Book: c.Book, Fair: c.Fair, Price: c.Price, Bid: c.Bid,
				Ask: c.Ask, BestBid: c.BestBid, BestAsk: c.BestAsk, DayAgo: c.DayAgo, Volume: c.Volume, Profit: c.Profit,
				Holders: c.Holders,
				Candles: listJSON(c.Candles, func(k model.StockCandle) stockCandleJSON {
					return stockCandleJSON{
						Hour: timeString(k.Hour), Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume,
					}
				}),
			}
		}),
		Index: listJSON(s.Index, func(p model.StockIndexPoint) stockIndexJSON {
			return stockIndexJSON{Hour: timeString(p.Hour), Value: p.Value}
		}),
		News:    toFirmEventsJSON(s.News),
		Fee:     s.Fee,
		Account: s.Account,
		Holdings: listJSON(s.Holdings, func(h model.StockHolding) stockHoldingJSON {
			return stockHoldingJSON{Firm: h.FirmID, Shares: h.Shares, Free: h.Free}
		}),
		Orders: listJSON(s.Orders, func(o model.StockOrder) stockOrderJSON {
			return stockOrderJSON{
				ID: o.ID, Firm: o.FirmID, Buy: o.Buy, Price: o.Price, Shares: o.Shares, PlacedAt: timeString(o.PlacedAt),
			}
		}),
		Firm:     s.FirmID,
		Bids:     listJSON(s.Bids, level),
		Asks:     listJSON(s.Asks, level),
		NextHour: timeString(s.NextHour),
	}, nil
}

// stocks is "Акции": the stock exchange, looking at the company ?firm=.
func (a *api) stocks(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	s, err := a.firmsBackend.Stocks(r.Context(), token, r.URL.Query().Get("firm"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out, err := a.stocksResponse(r, s)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// runStocks does an action on the stock exchange and answers with the
// exchange after it, looking at the company of the action.
func (a *api) runStocks(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req struct {
		Firm   string `json:"firm"`
		Buy    bool   `json:"buy"`
		Price  int64  `json:"price"`
		Shares int64  `json:"shares"`
		Votes  int64  `json:"votes"`
		Order  string `json:"order"`
	}
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.firmsBackend.RunStocks(r.Context(), token, model.StockAct{
		Action: r.PathValue("action"), FirmID: req.Firm, Buy: req.Buy, Price: req.Price, Shares: req.Shares,
		Votes: req.Votes, OrderID: req.Order,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	stocks, err := a.stocksResponse(r, d.Stocks)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Spent   int64      `json:"spent"`
		Got     int64      `json:"got"`
		Balance int64      `json:"balance"`
		Shares  int64      `json:"shares"`
		Money   int64      `json:"money"`
		Stocks  stocksJSON `json:"stocks"`
	}{d.Spent, d.Got, d.Balance, d.Shares, d.Money, stocks})
}
