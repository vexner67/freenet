package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PokerService interface {
	Table(ctx context.Context, userID uuid.UUID) (service.PokerTable, error)
	Deal(ctx context.Context, userID uuid.UUID, ante int64) (service.PokerView, error)
	Fold(ctx context.Context, userID, id uuid.UUID) (service.PokerView, error)
	Call(ctx context.Context, userID, id uuid.UUID) (service.PokerView, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.PokerSettings, error)
	SetPoker(ctx context.Context, adminID uuid.UUID, p user.Poker) error
}

func toProtoPoker(p user.Poker) *userv1.Poker {
	pays := p.Pays()
	out := &userv1.Poker{Least: p.Least(), Most: p.Most(), Pays: make([]int32, 0, len(pays))}

	for _, pay := range pays {
		out.Pays = append(out.Pays, int32(pay)) //nolint:gosec // at most user.MaxPokerPay
	}

	return out
}

var pokerStates = map[user.PokerState]userv1.PokerState{
	user.PokerDealt:       userv1.PokerState_POKER_STATE_DEALT,
	user.PokerFolded:      userv1.PokerState_POKER_STATE_FOLDED,
	user.PokerWon:         userv1.PokerState_POKER_STATE_WON,
	user.PokerLost:        userv1.PokerState_POKER_STATE_LOST,
	user.PokerTied:        userv1.PokerState_POKER_STATE_TIED,
	user.PokerUnqualified: userv1.PokerState_POKER_STATE_UNQUALIFIED,
}

func cardStrings(cards []user.Card) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.String())
	}

	return out
}

func toProtoPokerHand(h *user.PokerHand, balance int64) *userv1.PokerHand {
	player := h.PlayerHand()

	out := &userv1.PokerHand{
		Id:         h.ID().String(),
		Ante:       h.Ante(),
		State:      pokerStates[h.State()],
		Player:     cardStrings(h.Player()),
		Board:      cardStrings(h.Board()),
		Dealer:     cardStrings(h.Dealer()),
		Payout:     h.Payout(),
		PlayerHand: player.Rank.String(),
		PlayerBest: cardStrings(player.Cards[:]),
		Balance:    balance,
	}

	if dealer, ok := h.DealerHand(); ok {
		out.DealerHand = dealer.Rank.String()
		out.DealerBest = cardStrings(dealer.Cards[:])
	}

	return out
}

func (h *UserHandler) GetPoker(ctx context.Context, _ *userv1.GetPokerRequest) (*userv1.PokerTable, error) {
	table, err := h.pokers.Table(ctx, viewerID(ctx))
	if err != nil {
		return nil, err
	}

	resp := &userv1.PokerTable{Poker: toProtoPoker(table.Poker)}
	if table.Dealt != nil {
		resp.Dealt = toProtoPokerHand(table.Dealt, 0)
	}

	return resp, nil
}

func (h *UserHandler) DealPoker(ctx context.Context, req *userv1.DealPokerRequest) (*userv1.PokerHand, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	view, err := h.pokers.Deal(ctx, userID, req.GetAnte())
	if err != nil {
		return nil, err
	}

	return toProtoPokerHand(view.Hand, view.Balance), nil
}

func (h *UserHandler) FoldPoker(ctx context.Context, req *userv1.PokerHandRequest) (*userv1.PokerHand, error) {
	return h.pokerHand(ctx, req, h.pokers.Fold)
}

func (h *UserHandler) CallPoker(ctx context.Context, req *userv1.PokerHandRequest) (*userv1.PokerHand, error) {
	return h.pokerHand(ctx, req, h.pokers.Call)
}

// pokerHand is a hand of the caller folded or called by do.
func (h *UserHandler) pokerHand(
	ctx context.Context,
	req *userv1.PokerHandRequest,
	do func(ctx context.Context, userID, id uuid.UUID) (service.PokerView, error),
) (*userv1.PokerHand, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetHandId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid hand id")
	}

	view, err := do(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return toProtoPokerHand(view.Hand, view.Balance), nil
}

func (h *UserHandler) GetPokerSettings(ctx context.Context, _ *userv1.GetPokerSettingsRequest) (*userv1.GetPokerSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	settings, err := h.pokers.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetPokerSettingsResponse{
		Poker: toProtoPoker(settings.Poker),
		Hands: settings.Stats.Hands,
		Spent: settings.Stats.Spent,
		Won:   settings.Stats.Won,
	}, nil
}

func (h *UserHandler) SetPokerSettings(ctx context.Context, req *userv1.SetPokerSettingsRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	p := req.GetPoker()
	if len(p.GetPays()) != user.PokerPays {
		return nil, status.Error(codes.InvalidArgument, user.ErrPokerPays.Error())
	}

	var pays [user.PokerPays]int
	for i, pay := range p.GetPays() {
		pays[i] = int(pay)
	}

	poker, err := user.NewPoker(p.GetLeast(), p.GetMost(), pays)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.pokers.SetPoker(ctx, adminID, poker); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
