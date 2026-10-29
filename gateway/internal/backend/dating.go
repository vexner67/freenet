package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Dating is "Знакомства", kept by the user service.
type Dating struct {
	api userv1.UserServiceClient
}

func NewDating(api userv1.UserServiceClient) *Dating {
	return &Dating{api: api}
}

func (d *Dating) Dating(ctx context.Context, accessToken string) (model.Dating, error) {
	resp, err := d.api.GetDating(withToken(ctx, accessToken), &userv1.GetDatingRequest{})
	if err != nil {
		return model.Dating{}, errs.Wrap("d.api.GetDating", fromStatus(err))
	}

	var out model.Dating
	if resp.GetCard() != nil {
		card := fromProtoDatingCard(resp.GetCard())
		out.Card = &card
	}
	for _, c := range resp.GetDeck() {
		out.Deck = append(out.Deck, fromProtoDatingCard(c))
	}
	for _, m := range resp.GetMatches() {
		out.Matches = append(out.Matches, model.DatingMatch{UserID: m.GetUserId(), At: fromTimestamp(m.GetMatchedAt())})
	}

	return out, nil
}

func (d *Dating) SaveCard(ctx context.Context, accessToken string, in model.DatingCardInput) (model.DatingCard, error) {
	resp, err := d.api.SaveDatingCard(withToken(ctx, accessToken), &userv1.SaveDatingCardRequest{
		Goal: in.Goal, Seek: in.Seek, About: in.About, On: in.On,
	})
	if err != nil {
		return model.DatingCard{}, errs.Wrap("d.api.SaveDatingCard", fromStatus(err))
	}

	return fromProtoDatingCard(resp.GetCard()), nil
}

func (d *Dating) Answer(ctx context.Context, accessToken, userID string, like bool) (bool, error) {
	resp, err := d.api.AnswerDating(withToken(ctx, accessToken), &userv1.AnswerDatingRequest{UserId: userID, Like: like})
	if err != nil {
		return false, errs.Wrap("d.api.AnswerDating", fromStatus(err))
	}

	return resp.GetMatched(), nil
}

func fromProtoDatingCard(c *userv1.DatingCard) model.DatingCard {
	return model.DatingCard{
		UserID:    c.GetUserId(),
		Goal:      c.GetGoal(),
		Seek:      c.GetSeek(),
		About:     c.GetAbout(),
		On:        c.GetOn(),
		UpdatedAt: fromTimestamp(c.GetUpdatedAt()),
	}
}
