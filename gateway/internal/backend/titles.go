package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Titles are the awards the administrators give pages, which the user
// service keeps with the people.
type Titles struct {
	api userv1.UserServiceClient
}

func NewTitles(api userv1.UserServiceClient) *Titles {
	return &Titles{
		api: api,
	}
}

func fromProtoTitle(t *userv1.Title) model.Title {
	return model.Title{ID: t.GetId(), Name: t.GetName(), Icon: t.GetIcon(), GrantedAt: fromTimestamp(t.GetGrantedAt())}
}

// UserTitles are the titles of a page, the newest first.
func (t *Titles) UserTitles(ctx context.Context, accessToken, nickname string) ([]model.Title, error) {
	resp, err := t.api.GetUserTitles(withToken(ctx, accessToken), &userv1.GetUserTitlesRequest{Nickname: nickname})
	if err != nil {
		return nil, errs.Wrap("t.api.GetUserTitles", fromStatus(err))
	}

	out := make([]model.Title, 0, len(resp.GetTitles()))
	for _, title := range resp.GetTitles() {
		out = append(out, fromProtoTitle(title))
	}

	return out, nil
}

func (t *Titles) Grant(ctx context.Context, accessToken, nickname, name, icon string) (model.Title, error) {
	resp, err := t.api.GrantTitle(withToken(ctx, accessToken), &userv1.GrantTitleRequest{Nickname: nickname, Name: name, Icon: icon})
	if err != nil {
		return model.Title{}, errs.Wrap("t.api.GrantTitle", fromStatus(err))
	}

	return fromProtoTitle(resp.GetTitle()), nil
}

func (t *Titles) Revoke(ctx context.Context, accessToken, id string) error {
	if _, err := t.api.RevokeTitle(withToken(ctx, accessToken), &userv1.RevokeTitleRequest{TitleId: id}); err != nil {
		return errs.Wrap("t.api.RevokeTitle", fromStatus(err))
	}

	return nil
}
