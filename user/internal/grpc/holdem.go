package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type HoldemService interface {
	Tables(ctx context.Context, userID uuid.UUID, section service.TableSection) ([]*user.HoldemTable, error)
	Create(ctx context.Context, userID uuid.UUID, in service.NewTableInput) (service.HoldemView, error)
	Quick(ctx context.Context, userID uuid.UUID, bigBlind, buyIn int64) (service.HoldemView, error)
	Table(ctx context.Context, viewerID, id uuid.UUID) (*user.HoldemTable, error)
	Sit(ctx context.Context, userID, id uuid.UUID, seat int, buyIn int64) (service.HoldemView, error)
	Leave(ctx context.Context, userID, id uuid.UUID) (service.HoldemView, error)
	Act(ctx context.Context, userID, id uuid.UUID, action user.TableAction, amount int64) (*user.HoldemTable, error)
	Back(ctx context.Context, userID, id uuid.UUID) (*user.HoldemTable, error)
	Invite(ctx context.Context, userID, id uuid.UUID, nicknames []user.Nickname) (*user.HoldemTable, error)
}

// holdemInvalid are the wrong requests of the poker tables.
var holdemInvalid = []error{
	user.ErrTableName, user.ErrTableSeats, user.ErrTableBlind, user.ErrTableStake, user.ErrTableBuyIn,
	user.ErrTableRaise, user.ErrTableInvites, service.ErrTableSection,
}

// holdemRefused are what a poker table does not allow now.
var holdemRefused = []error{
	user.ErrTableFull, user.ErrTableSeatTaken, user.ErrTableSeated, user.ErrTableNotSeated,
	user.ErrTableNotInvited, user.ErrTableNotYourTurn, user.ErrTableAction,
}

// toProtoHoldemTable is table as viewerID sees it at now: the cards of the
// others are hidden until shown.
func toProtoHoldemTable(t *user.HoldemTable, viewerID uuid.UUID, now time.Time) *userv1.HoldemTable {
	you := t.SeatOf(viewerID)
	least, most := t.RaiseLimits(you)

	out := &userv1.HoldemTable{
		Id:         t.ID().String(),
		Name:       t.Name(),
		Private:    t.Private(),
		OwnerId:    t.OwnerID().String(),
		BigBlind:   t.BigBlind(),
		SmallBlind: t.SmallBlind(),
		MinBuyIn:   t.MinBuyIn(),
		MaxBuyIn:   t.MaxBuyIn(),
		Street:     string(t.Street()),
		Hand:       t.Hand(),
		Button:     int32(t.Button()), //nolint:gosec // at most user.MaxTableSeats
		Turn:       int32(t.Turn()),   //nolint:gosec // at most user.MaxTableSeats
		Board:      cardStrings(t.Board()),
		Pot:        t.Pot(),
		CurrentBet: t.CurrentBet(),
		Version:    t.Version(),
		Players:    int32(t.Players()), //nolint:gosec // at most user.MaxTableSeats
		You:        int32(you),         //nolint:gosec // at most user.MaxTableSeats
		ToCall:     t.ToCall(you),
		RaiseLeast: least,
		RaiseMost:  most,
		Invited:    viewerID != uuid.Nil && t.IsInvited(viewerID) && t.Private(),
	}
	if d := t.Deadline(); !d.IsZero() {
		out.TurnMs = max(d.Sub(now).Milliseconds(), 0)
	}

	for i, p := range t.Seats() {
		out.Seats = append(out.Seats, toProtoHoldemSeat(t, i, p, i == you))
	}

	return out
}

func toProtoHoldemSeat(t *user.HoldemTable, i int, p user.TableSeat, own bool) *userv1.HoldemSeat {
	if !p.Taken() {
		return &userv1.HoldemSeat{}
	}

	seat := &userv1.HoldemSeat{
		UserId:   p.UserID.String(),
		Stack:    p.Stack,
		Bet:      p.Bet,
		HasCards: len(p.Cards) > 0 && !p.Folded,
		Folded:   p.Folded,
		AllIn:    p.AllIn,
		Away:     p.Away,
		Leaving:  p.Leaving,
		Last:     string(p.Last),
		Won:      p.Won,
	}
	if !own && !p.Shown {
		return seat
	}

	seat.Cards = cardStrings(p.Cards)
	if hand, ok := t.HandOf(i); ok {
		seat.Hand = hand.Rank.String()
		seat.Best = cardStrings(hand.Cards[:])
	}

	return seat
}

func (h *UserHandler) holdemTableID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid table id")
	}

	return parsed, nil
}

func toProtoHoldemView(view service.HoldemView, userID uuid.UUID) *userv1.HoldemTableResponse {
	resp := &userv1.HoldemTableResponse{Balance: view.Balance}
	if view.Table != nil {
		resp.Table = toProtoHoldemTable(view.Table, userID, time.Now())
	}

	return resp
}

func (h *UserHandler) ListHoldemTables(ctx context.Context, req *userv1.ListHoldemTablesRequest) (*userv1.ListHoldemTablesResponse, error) {
	viewer := viewerID(ctx)

	section := service.TableSection(req.GetSection())
	if section == "" {
		section = service.TablesPublic
	}
	if section != service.TablesPublic && viewer == uuid.Nil {
		return &userv1.ListHoldemTablesResponse{}, nil
	}

	tables, err := h.holdems.Tables(ctx, viewer, section)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	resp := &userv1.ListHoldemTablesResponse{Tables: make([]*userv1.HoldemTable, 0, len(tables))}
	for _, t := range tables {
		resp.Tables = append(resp.Tables, toProtoHoldemTable(t, viewer, now))
	}

	return resp, nil
}

func (h *UserHandler) CreateHoldemTable(ctx context.Context, req *userv1.CreateHoldemTableRequest) (*userv1.HoldemTableResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	view, err := h.holdems.Create(ctx, userID, service.NewTableInput{
		Name:     req.GetName(),
		Private:  req.GetPrivate(),
		Seats:    int(req.GetSeats()),
		BigBlind: req.GetBigBlind(),
		BuyIn:    req.GetBuyIn(),
	})
	if err != nil {
		return nil, err
	}

	return toProtoHoldemView(view, userID), nil
}

func (h *UserHandler) QuickHoldem(ctx context.Context, req *userv1.QuickHoldemRequest) (*userv1.HoldemTableResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	view, err := h.holdems.Quick(ctx, userID, req.GetBigBlind(), req.GetBuyIn())
	if err != nil {
		return nil, err
	}

	return toProtoHoldemView(view, userID), nil
}

func (h *UserHandler) GetHoldemTable(ctx context.Context, req *userv1.HoldemTableRequest) (*userv1.HoldemTable, error) {
	id, err := h.holdemTableID(req.GetId())
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)

	table, err := h.holdems.Table(ctx, viewer, id)
	if err != nil {
		return nil, err
	}

	return toProtoHoldemTable(table, viewer, time.Now()), nil
}

func (h *UserHandler) SitHoldemTable(ctx context.Context, req *userv1.SitHoldemTableRequest) (*userv1.HoldemTableResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.holdemTableID(req.GetId())
	if err != nil {
		return nil, err
	}

	view, err := h.holdems.Sit(ctx, userID, id, int(req.GetSeat()), req.GetBuyIn())
	if err != nil {
		return nil, err
	}

	return toProtoHoldemView(view, userID), nil
}

func (h *UserHandler) LeaveHoldemTable(ctx context.Context, req *userv1.HoldemTableRequest) (*userv1.HoldemTableResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.holdemTableID(req.GetId())
	if err != nil {
		return nil, err
	}

	view, err := h.holdems.Leave(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return toProtoHoldemView(view, userID), nil
}

func (h *UserHandler) ActHoldemTable(ctx context.Context, req *userv1.ActHoldemTableRequest) (*userv1.HoldemTable, error) {
	return h.holdemTable(ctx, req.GetId(), func(userID, id uuid.UUID) (*user.HoldemTable, error) {
		return h.holdems.Act(ctx, userID, id, user.TableAction(req.GetAction()), req.GetAmount())
	})
}

func (h *UserHandler) BackHoldemTable(ctx context.Context, req *userv1.HoldemTableRequest) (*userv1.HoldemTable, error) {
	return h.holdemTable(ctx, req.GetId(), func(userID, id uuid.UUID) (*user.HoldemTable, error) {
		return h.holdems.Back(ctx, userID, id)
	})
}

func (h *UserHandler) InviteHoldemTable(ctx context.Context, req *userv1.InviteHoldemTableRequest) (*userv1.HoldemTable, error) {
	var nicknames []user.Nickname
	for _, raw := range req.GetNicknames() {
		n, err := user.NewNickname(raw)
		if err != nil {
			continue
		}
		nicknames = append(nicknames, n)
	}

	return h.holdemTable(ctx, req.GetId(), func(userID, id uuid.UUID) (*user.HoldemTable, error) {
		return h.holdems.Invite(ctx, userID, id, nicknames)
	})
}

// holdemTable is the table id changed by the caller with do.
func (h *UserHandler) holdemTable(
	ctx context.Context,
	rawID string,
	do func(userID, id uuid.UUID) (*user.HoldemTable, error),
) (*userv1.HoldemTable, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.holdemTableID(rawID)
	if err != nil {
		return nil, err
	}

	table, err := do(userID, id)
	if err != nil {
		return nil, err
	}

	return toProtoHoldemTable(table, userID, time.Now()), nil
}
