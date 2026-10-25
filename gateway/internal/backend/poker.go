package backend

import (
	"context"
	"math"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Poker is "Покер" of the casino, which the user service keeps with the
// votes.
type Poker struct {
	api userv1.UserServiceClient
}

func NewPoker(api userv1.UserServiceClient) *Poker {
	return &Poker{
		api: api,
	}
}

// Get is the table and, with a token, the hand of the caller waiting.
func (p *Poker) Get(ctx context.Context, accessToken string) (model.PokerTable, error) {
	resp, err := p.api.GetPoker(withToken(ctx, accessToken), &userv1.GetPokerRequest{})
	if err != nil {
		return model.PokerTable{}, errs.Wrap("p.api.GetPoker", fromStatus(err))
	}

	table := model.PokerTable{Poker: fromProtoPoker(resp.GetPoker())}
	if resp.GetDealt() != nil {
		hand := fromProtoPokerHand(resp.GetDealt())
		table.Dealt = &hand
	}

	return table, nil
}

func (p *Poker) Deal(ctx context.Context, accessToken string, ante int64) (model.PokerHand, error) {
	resp, err := p.api.DealPoker(withToken(ctx, accessToken), &userv1.DealPokerRequest{Ante: ante})
	if err != nil {
		return model.PokerHand{}, errs.Wrap("p.api.DealPoker", fromStatus(err))
	}

	return fromProtoPokerHand(resp), nil
}

// Play calls or folds a hand of the caller.
func (p *Poker) Play(ctx context.Context, accessToken, id string, call bool) (model.PokerHand, error) {
	req := &userv1.PokerHandRequest{HandId: id}

	if call {
		resp, err := p.api.CallPoker(withToken(ctx, accessToken), req)
		if err != nil {
			return model.PokerHand{}, errs.Wrap("p.api.CallPoker", fromStatus(err))
		}

		return fromProtoPokerHand(resp), nil
	}

	resp, err := p.api.FoldPoker(withToken(ctx, accessToken), req)
	if err != nil {
		return model.PokerHand{}, errs.Wrap("p.api.FoldPoker", fromStatus(err))
	}

	return fromProtoPokerHand(resp), nil
}

func (p *Poker) Settings(ctx context.Context, accessToken string) (model.Poker, model.PokerStats, error) {
	resp, err := p.api.GetPokerSettings(withToken(ctx, accessToken), &userv1.GetPokerSettingsRequest{})
	if err != nil {
		return model.Poker{}, model.PokerStats{}, errs.Wrap("p.api.GetPokerSettings", fromStatus(err))
	}

	return fromProtoPoker(resp.GetPoker()), model.PokerStats{Hands: resp.GetHands(), Spent: resp.GetSpent(), Won: resp.GetWon()}, nil
}

func (p *Poker) Set(ctx context.Context, accessToken string, poker model.Poker) error {
	pays := make([]int32, 0, len(poker.Pays))
	for _, pay := range poker.Pays {
		if pay < 0 || pay > math.MaxInt32 {
			return failure.New(failure.InvalidArgument, "a pay of poker must be from 1 to 10000")
		}

		pays = append(pays, int32(pay))
	}

	req := &userv1.SetPokerSettingsRequest{Poker: &userv1.Poker{Least: poker.Least, Most: poker.Most, Pays: pays}}
	if _, err := p.api.SetPokerSettings(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("p.api.SetPokerSettings", fromStatus(err))
	}

	return nil
}

func fromProtoPoker(p *userv1.Poker) model.Poker {
	out := model.Poker{Least: p.GetLeast(), Most: p.GetMost(), Pays: make([]int, 0, len(p.GetPays()))}
	for _, pay := range p.GetPays() {
		out.Pays = append(out.Pays, int(pay))
	}

	return out
}

var pokerStates = map[userv1.PokerState]string{
	userv1.PokerState_POKER_STATE_DEALT:       "dealt",
	userv1.PokerState_POKER_STATE_FOLDED:      "folded",
	userv1.PokerState_POKER_STATE_WON:         "won",
	userv1.PokerState_POKER_STATE_LOST:        "lost",
	userv1.PokerState_POKER_STATE_TIED:        "tied",
	userv1.PokerState_POKER_STATE_UNQUALIFIED: "unqualified",
}

func fromProtoPokerHand(h *userv1.PokerHand) model.PokerHand {
	return model.PokerHand{
		ID:         h.GetId(),
		Ante:       h.GetAnte(),
		State:      pokerStates[h.GetState()],
		Player:     h.GetPlayer(),
		Board:      h.GetBoard(),
		Dealer:     h.GetDealer(),
		Payout:     h.GetPayout(),
		PlayerHand: h.GetPlayerHand(),
		DealerHand: h.GetDealerHand(),
		PlayerBest: h.GetPlayerBest(),
		DealerBest: h.GetDealerBest(),
		Balance:    h.GetBalance(),
	}
}
