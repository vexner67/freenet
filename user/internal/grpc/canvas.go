package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CanvasService is "Холст".
type CanvasService interface {
	Canvas(ctx context.Context) ([]byte, error)
	Cell(ctx context.Context, x, y int) (user.CanvasCell, error)
	Paint(ctx context.Context, c user.CanvasCell) error
}

func (h *UserHandler) GetCanvas(ctx context.Context, _ *userv1.GetCanvasRequest) (*userv1.GetCanvasResponse, error) {
	picture, err := h.canvas.Canvas(ctx)
	if err != nil {
		return nil, err
	}

	return &userv1.GetCanvasResponse{Cells: picture, Size: user.CanvasSize}, nil
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

	if err = h.canvas.Paint(ctx, cell); err != nil {
		return nil, err
	}

	return &userv1.PaintCanvasResponse{}, nil
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
