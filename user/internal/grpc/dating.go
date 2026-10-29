package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DatingService is "Знакомства".
type DatingService interface {
	Dating(ctx context.Context, userID uuid.UUID, now time.Time) (service.DatingView, error)
	SaveCard(ctx context.Context, c user.DatingCard) error
	Answer(ctx context.Context, a service.DatingAnswer) (bool, error)
}

func (h *UserHandler) GetDating(ctx context.Context, _ *userv1.GetDatingRequest) (*userv1.GetDatingResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	view, err := h.dating.Dating(ctx, userID, time.Now())
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetDatingResponse{
		Deck:    make([]*userv1.DatingCard, len(view.Deck)),
		Matches: make([]*userv1.DatingMatch, len(view.Matches)),
	}
	if view.Card != nil {
		resp.Card = toProtoDatingCard(*view.Card)
	}
	for i, c := range view.Deck {
		resp.Deck[i] = toProtoDatingCard(c)
	}
	for i, m := range view.Matches {
		resp.Matches[i] = &userv1.DatingMatch{UserId: m.UserID.String(), MatchedAt: timestamppb.New(m.At)}
	}

	return resp, nil
}

func (h *UserHandler) SaveDatingCard(ctx context.Context, req *userv1.SaveDatingCardRequest) (*userv1.SaveDatingCardResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	goal, err := user.NewDatingGoal(req.GetGoal())
	if err != nil {
		return nil, err
	}
	seek, err := user.NewDatingSeek(req.GetSeek())
	if err != nil {
		return nil, err
	}
	card, err := user.NewDatingCard(userID, goal, seek, req.GetAbout(), req.GetOn(), time.Now())
	if err != nil {
		return nil, err
	}

	if err = h.dating.SaveCard(ctx, card); err != nil {
		return nil, err
	}

	return &userv1.SaveDatingCardResponse{Card: toProtoDatingCard(card)}, nil
}

func (h *UserHandler) AnswerDating(ctx context.Context, req *userv1.AnswerDatingRequest) (*userv1.AnswerDatingResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	targetID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	matched, err := h.dating.Answer(ctx, service.DatingAnswer{UserID: userID, TargetID: targetID, Like: req.GetLike(), Now: time.Now()})
	if err != nil {
		return nil, err
	}

	return &userv1.AnswerDatingResponse{Matched: matched}, nil
}

func toProtoDatingCard(c user.DatingCard) *userv1.DatingCard {
	return &userv1.DatingCard{
		UserId:    c.UserID.String(),
		Goal:      c.Goal.String(),
		Seek:      c.Seek.String(),
		About:     c.About,
		On:        c.On,
		UpdatedAt: timestamppb.New(c.UpdatedAt),
	}
}
