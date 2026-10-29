package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCanvas(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")

	canvas, err := s.client.GetCanvas(context.Background(), &userv1.GetCanvasRequest{})
	if err != nil || len(canvas.GetCells()) != 100*100 || canvas.GetSize() != 100 || canvas.GetNextAt() != nil {
		t.Fatalf("GetCanvas by a guest = %v, %v", canvas.GetSize(), err)
	}
	if _, err = s.client.PaintCanvas(context.Background(), &userv1.PaintCanvasRequest{X: 1, Y: 1, Color: 2}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("PaintCanvas by a guest: %v", err)
	}
	if _, err = s.client.PaintCanvas(ann, &userv1.PaintCanvasRequest{X: 100, Y: 1, Color: 2}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PaintCanvas off the canvas: %v", err)
	}
	if _, err = s.client.PaintCanvas(ann, &userv1.PaintCanvasRequest{X: 1, Y: 1, Color: 16}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PaintCanvas of no color: %v", err)
	}

	painted, err := s.client.PaintCanvas(ann, &userv1.PaintCanvasRequest{X: 1, Y: 2, Color: 5})
	if err != nil || painted.GetNextAt() == nil {
		t.Fatalf("PaintCanvas = %v, %v", painted, err)
	}
	if _, err = s.client.PaintCanvas(ann, &userv1.PaintCanvasRequest{X: 2, Y: 2, Color: 5}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("PaintCanvas within the minute: %v", err)
	}
	if canvas, err = s.client.GetCanvas(ann, &userv1.GetCanvasRequest{}); err != nil || canvas.GetCells()[2*100+1] != 5 || canvas.GetNextAt() == nil {
		t.Fatalf("GetCanvas after a cell = %v", err)
	}

	cell, err := s.client.GetCanvasCell(context.Background(), &userv1.GetCanvasCellRequest{X: 1, Y: 2})
	if err != nil || cell.GetUserId() != annID || cell.GetColor() != 5 {
		t.Fatalf("GetCanvasCell = %v, %v", cell, err)
	}
	if _, err = s.client.GetCanvasCell(context.Background(), &userv1.GetCanvasCellRequest{X: 3, Y: 3}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetCanvasCell never painted: %v", err)
	}
}
