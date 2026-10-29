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

// CanvasService is "Холст".
type CanvasService interface {
	Canvas(ctx context.Context, viewerID uuid.UUID, now time.Time) (service.CanvasView, error)
	Cell(ctx context.Context, x, y int) (user.CanvasCell, error)
	Paint(ctx context.Context, c user.CanvasCell) (time.Time, error)
}

func (h *UserHandler) GetCanvas(ctx context.Context, _ *userv1.GetCanvasRequest) (*userv1.GetCanvasResponse, error) {
	view, err := h.canvas.Canvas(ctx, viewerID(ctx), time.Now())
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetCanvasResponse{Cells: view.Picture, Size: user.CanvasSize, CooldownSeconds: int32(user.CanvasCooldown / time.Second)}
	if !view.NextAt.IsZero() {
		resp.NextAt = timestamppb.New(view.NextAt)
	}

	return resp, nil
}

func (h *UserHandler) PaintCanvas(ctx context.Context, req *userv1.PaintCanvasRequest) (*userv1.PaintCanvasResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cell, err := user.NewCanvasCell(int(req.GetX()), int(req.GetY()), int(req.GetColor()), userID, time.Now())
	if err != nil {
		return nil, err
	}

	next, err := h.canvas.Paint(ctx, cell)
	if err != nil {
		return nil, err
	}

	return &userv1.PaintCanvasResponse{NextAt: timestamppb.New(next)}, nil
}

func (h *UserHandler) GetCanvasCell(ctx context.Context, req *userv1.GetCanvasCellRequest) (*userv1.GetCanvasCellResponse, error) {
	c, err := h.canvas.Cell(ctx, int(req.GetX()), int(req.GetY()))
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetCanvasCellResponse{Color: int32(c.Color), PaintedAt: timestamppb.New(c.At)} //nolint:gosec // a color is below 16
	if c.UserID != uuid.Nil {
		resp.UserId = c.UserID.String()
	}

	return resp, nil
}
