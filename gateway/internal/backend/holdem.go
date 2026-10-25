package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Holdem are the poker tables of people, which the user service keeps
// with the votes.
type Holdem struct {
	api userv1.UserServiceClient
}

func NewHoldem(api userv1.UserServiceClient) *Holdem {
	return &Holdem{
		api: api,
	}
}

func (h *Holdem) List(ctx context.Context, accessToken, section string) ([]model.HoldemTable, error) {
	resp, err := h.api.ListHoldemTables(withToken(ctx, accessToken), &userv1.ListHoldemTablesRequest{Section: section})
	if err != nil {
		return nil, errs.Wrap("h.api.ListHoldemTables", fromStatus(err))
	}

	tables := make([]model.HoldemTable, 0, len(resp.GetTables()))
	for _, t := range resp.GetTables() {
		tables = append(tables, fromProtoHoldemTable(t))
	}

	return tables, nil
}

func (h *Holdem) Create(ctx context.Context, accessToken string, in model.NewHoldemTable) (*model.HoldemTable, int64, error) {
	if in.Seats < 0 || in.Seats > math.MaxInt32 {
		return nil, 0, failure.New(failure.InvalidArgument, "a table has from 2 to 9 seats")
	}

	resp, err := h.api.CreateHoldemTable(withToken(ctx, accessToken), &userv1.CreateHoldemTableRequest{
		Name: in.Name, Private: in.Private, Seats: int32(in.Seats), BigBlind: in.BigBlind, BuyIn: in.BuyIn,
	})
	if err != nil {
		return nil, 0, errs.Wrap("h.api.CreateHoldemTable", fromStatus(err))
	}

	table, balance := fromProtoHoldemResponse(resp)

	return table, balance, nil
}

func (h *Holdem) Quick(ctx context.Context, accessToken string, bigBlind, buyIn int64) (*model.HoldemTable, int64, error) {
	resp, err := h.api.QuickHoldem(withToken(ctx, accessToken), &userv1.QuickHoldemRequest{BigBlind: bigBlind, BuyIn: buyIn})
	if err != nil {
		return nil, 0, errs.Wrap("h.api.QuickHoldem", fromStatus(err))
	}

	table, balance := fromProtoHoldemResponse(resp)

	return table, balance, nil
}

func (h *Holdem) Get(ctx context.Context, accessToken, id string) (model.HoldemTable, error) {
	resp, err := h.api.GetHoldemTable(withToken(ctx, accessToken), &userv1.HoldemTableRequest{Id: id})
	if err != nil {
		return model.HoldemTable{}, errs.Wrap("h.api.GetHoldemTable", fromStatus(err))
	}

	return fromProtoHoldemTable(resp), nil
}

func (h *Holdem) Sit(ctx context.Context, accessToken, id string, seat int, buyIn int64) (*model.HoldemTable, int64, error) {
	if seat < -1 || seat > math.MaxInt32 {
		return nil, 0, failure.New(failure.InvalidArgument, "the seat is taken")
	}

	resp, err := h.api.SitHoldemTable(withToken(ctx, accessToken), &userv1.SitHoldemTableRequest{Id: id, Seat: int32(seat), BuyIn: buyIn})
	if err != nil {
		return nil, 0, errs.Wrap("h.api.SitHoldemTable", fromStatus(err))
	}

	table, balance := fromProtoHoldemResponse(resp)

	return table, balance, nil
}

func (h *Holdem) Leave(ctx context.Context, accessToken, id string) (*model.HoldemTable, int64, error) {
	resp, err := h.api.LeaveHoldemTable(withToken(ctx, accessToken), &userv1.HoldemTableRequest{Id: id})
	if err != nil {
		return nil, 0, errs.Wrap("h.api.LeaveHoldemTable", fromStatus(err))
	}

	table, balance := fromProtoHoldemResponse(resp)

	return table, balance, nil
}

func (h *Holdem) Act(ctx context.Context, accessToken, id, action string, amount int64) (model.HoldemTable, error) {
	resp, err := h.api.ActHoldemTable(withToken(ctx, accessToken), &userv1.ActHoldemTableRequest{Id: id, Action: action, Amount: amount})
	if err != nil {
		return model.HoldemTable{}, errs.Wrap("h.api.ActHoldemTable", fromStatus(err))
	}

	return fromProtoHoldemTable(resp), nil
}

func (h *Holdem) Back(ctx context.Context, accessToken, id string) (model.HoldemTable, error) {
	resp, err := h.api.BackHoldemTable(withToken(ctx, accessToken), &userv1.HoldemTableRequest{Id: id})
	if err != nil {
		return model.HoldemTable{}, errs.Wrap("h.api.BackHoldemTable", fromStatus(err))
	}

	return fromProtoHoldemTable(resp), nil
}

func (h *Holdem) Invite(ctx context.Context, accessToken, id string, nicknames []string) (model.HoldemTable, error) {
	resp, err := h.api.InviteHoldemTable(withToken(ctx, accessToken), &userv1.InviteHoldemTableRequest{Id: id, Nicknames: nicknames})
	if err != nil {
		return model.HoldemTable{}, errs.Wrap("h.api.InviteHoldemTable", fromStatus(err))
	}

	return fromProtoHoldemTable(resp), nil
}

func fromProtoHoldemResponse(resp *userv1.HoldemTableResponse) (*model.HoldemTable, int64) {
	if resp.GetTable() == nil {
		return nil, resp.GetBalance()
	}

	table := fromProtoHoldemTable(resp.GetTable())

	return &table, resp.GetBalance()
}

func fromProtoHoldemTable(t *userv1.HoldemTable) model.HoldemTable {
	out := model.HoldemTable{
		ID:         t.GetId(),
		Name:       t.GetName(),
		Private:    t.GetPrivate(),
		OwnerID:    t.GetOwnerId(),
		BigBlind:   t.GetBigBlind(),
		SmallBlind: t.GetSmallBlind(),
		MinBuyIn:   t.GetMinBuyIn(),
		MaxBuyIn:   t.GetMaxBuyIn(),
		Street:     t.GetStreet(),
		Hand:       t.GetHand(),
		Button:     int(t.GetButton()),
		Turn:       int(t.GetTurn()),
		TurnMs:     t.GetTurnMs(),
		Board:      t.GetBoard(),
		Pot:        t.GetPot(),
		CurrentBet: t.GetCurrentBet(),
		Version:    t.GetVersion(),
		Players:    int(t.GetPlayers()),
		You:        int(t.GetYou()),
		ToCall:     t.GetToCall(),
		RaiseLeast: t.GetRaiseLeast(),
		RaiseMost:  t.GetRaiseMost(),
		Invited:    t.GetInvited(),
		Seats:      make([]model.HoldemSeat, 0, len(t.GetSeats())),
	}

	for _, s := range t.GetSeats() {
		out.Seats = append(out.Seats, model.HoldemSeat{
			UserID:   s.GetUserId(),
			Stack:    s.GetStack(),
			Bet:      s.GetBet(),
			Cards:    s.GetCards(),
			HasCards: s.GetHasCards(),
			Folded:   s.GetFolded(),
			AllIn:    s.GetAllIn(),
			Away:     s.GetAway(),
			Leaving:  s.GetLeaving(),
			Last:     s.GetLast(),
			Won:      s.GetWon(),
			Hand:     s.GetHand(),
			Best:     s.GetBest(),
		})
	}

	return out
}
