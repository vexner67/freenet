package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWorldVotes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newServer(t)
	_, ann := s.member(t, "ann")
	op := uuid.Must(uuid.NewV7()).String()

	tests := []struct {
		name string
		req  *userv1.ChargeVotesRequest
		code codes.Code
	}{
		{"bad operation", &userv1.ChargeVotesRequest{OpId: "x", UserId: ann, Votes: 1}, codes.InvalidArgument},
		{"no votes asked", &userv1.ChargeVotesRequest{OpId: op, UserId: ann}, codes.InvalidArgument},
		{"too few votes", &userv1.ChargeVotesRequest{OpId: op, UserId: ann, Votes: 5}, codes.FailedPrecondition},
		{"not settled", &userv1.ChargeVotesRequest{OpId: op, UserId: ann, Votes: 5, Settled: true}, codes.FailedPrecondition},
		{"unknown page", &userv1.ChargeVotesRequest{OpId: op, UserId: uuid.Must(uuid.NewV7()).String(), Votes: 5}, codes.NotFound},
	}
	for _, tt := range tests {
		if _, err := s.client.ChargeVotes(ctx, tt.req); status.Code(err) != tt.code {
			t.Errorf("%s: ChargeVotes = %v, want %v", tt.name, err, tt.code)
		}
	}

	pay := &userv1.PayVotesRequest{OpId: uuid.Must(uuid.NewV7()).String(), UserId: ann, Votes: 7, Note: "refund"}
	for range 2 {
		if got, err := s.client.PayVotes(ctx, pay); err != nil || got.GetVotes() != 7 {
			t.Fatalf("PayVotes = %v, %v", got, err)
		}
	}

	got, err := s.client.ChargeVotes(ctx, &userv1.ChargeVotesRequest{OpId: uuid.Must(uuid.NewV7()).String(), UserId: ann, Votes: 5, Note: "donate"})
	if err != nil || got.GetVotes() != 2 {
		t.Fatalf("ChargeVotes = %v, %v", got, err)
	}
	if _, err = s.client.ChargeVotes(ctx, &userv1.ChargeVotesRequest{OpId: uuid.Must(uuid.NewV7()).String(), UserId: ann, Votes: 5}); status.Convert(err).Message() != user.ErrNotEnoughVotes.Error() {
		t.Fatalf("ChargeVotes of too many: %v", err)
	}
}
