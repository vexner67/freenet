package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Decor is the New Year decoration of the site, which the user service
// keeps.
type Decor struct {
	api userv1.UserServiceClient
}

func NewDecor(api userv1.UserServiceClient) *Decor {
	return &Decor{
		api: api,
	}
}

var decorModes = map[userv1.Decor]string{
	userv1.Decor_DECOR_AUTO: "auto",
	userv1.Decor_DECOR_ON:   "on",
	userv1.Decor_DECOR_OFF:  "off",
}

func (d *Decor) Get(ctx context.Context) (model.Decor, error) {
	resp, err := d.api.GetDecor(ctx, &userv1.GetDecorRequest{})
	if err != nil {
		return model.Decor{}, errs.Wrap("d.api.GetDecor", fromStatus(err))
	}

	mode, ok := decorModes[resp.GetDecor()]
	if !ok {
		mode = "auto"
	}

	return model.Decor{Mode: mode, Festive: resp.GetFestive()}, nil
}

func (d *Decor) Set(ctx context.Context, accessToken, mode string) error {
	var decor userv1.Decor
	for k, v := range decorModes {
		if v == mode {
			decor = k
		}
	}
	if decor == userv1.Decor_DECOR_UNSPECIFIED {
		return failure.New(failure.InvalidArgument, "decor must be auto, on or off")
	}

	if _, err := d.api.SetDecor(withToken(ctx, accessToken), &userv1.SetDecorRequest{Decor: decor}); err != nil {
		return errs.Wrap("d.api.SetDecor", fromStatus(err))
	}

	return nil
}
