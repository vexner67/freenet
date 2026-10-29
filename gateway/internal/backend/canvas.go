package backend

import (
	"context"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Canvas is the shared canvas, kept by the user service.
type Canvas struct {
	api userv1.UserServiceClient
}

func NewCanvas(api userv1.UserServiceClient) *Canvas {
	return &Canvas{api: api}
}

func (c *Canvas) Canvas(ctx context.Context, accessToken string) (model.Canvas, error) {
	resp, err := c.api.GetCanvas(withToken(ctx, accessToken), &userv1.GetCanvasRequest{})
	if err != nil {
		return model.Canvas{}, errs.Wrap("c.api.GetCanvas", fromStatus(err))
	}

	return model.Canvas{
		Cells:    resp.GetCells(),
		Size:     int(resp.GetSize()),
		NextAt:   fromTimestamp(resp.GetNextAt()),
		Cooldown: int(resp.GetCooldownSeconds()),
	}, nil
}

func (c *Canvas) Paint(ctx context.Context, accessToken string, p model.CanvasPaint) (time.Time, error) {
	resp, err := c.api.PaintCanvas(withToken(ctx, accessToken), &userv1.PaintCanvasRequest{
		X: int32(p.X), Y: int32(p.Y), Color: int32(p.Color), //nolint:gosec // the user service checks the bounds
	})
	if err != nil {
		return time.Time{}, errs.Wrap("c.api.PaintCanvas", fromStatus(err))
	}

	return fromTimestamp(resp.GetNextAt()), nil
}

func (c *Canvas) Cell(ctx context.Context, accessToken string, x, y int) (model.CanvasCell, error) {
	resp, err := c.api.GetCanvasCell(withToken(ctx, accessToken), &userv1.GetCanvasCellRequest{
		X: int32(x), Y: int32(y), //nolint:gosec // the user service checks the bounds
	})
	if err != nil {
		return model.CanvasCell{}, errs.Wrap("c.api.GetCanvasCell", fromStatus(err))
	}

	return model.CanvasCell{UserID: resp.GetUserId(), Color: int(resp.GetColor()), PaintedAt: fromTimestamp(resp.GetPaintedAt())}, nil
}
