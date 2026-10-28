package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestWorldService_ChargeAndPay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	world := service.NewWorldService(w.users, w.users, inmem.NewWorldVoteRepository(), inmem.TxManager{})
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	charge := service.WorldVotes{OpID: uuid.Must(uuid.NewV7()), UserID: ann, Votes: 30, Note: "donate"}
	if _, err := world.ChargeVotes(ctx, charge); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("without votes: %v", err)
	}

	w.credit(t, admin, ann, 100)

	charge.OpID = uuid.Must(uuid.NewV7())
	left, err := world.ChargeVotes(ctx, charge)
	if err != nil || left != 70 {
		t.Fatalf("ChargeVotes = %d, %v", left, err)
	}
	// Asked again, it takes nothing.
	if left, err = world.ChargeVotes(ctx, charge); err != nil || left != 70 || w.votes(t, ann) != 70 {
		t.Fatalf("ChargeVotes again = %d, %v", left, err)
	}

	pay := service.WorldVotes{OpID: uuid.Must(uuid.NewV7()), UserID: ann, Votes: 30, Note: "refund"}
	for range 2 {
		if votes, err := world.PayVotes(ctx, pay); err != nil || votes != 100 {
			t.Fatalf("PayVotes = %d, %v", votes, err)
		}
	}
}

func TestWorldService_Refuses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	world := service.NewWorldService(w.users, w.users, inmem.NewWorldVoteRepository(), inmem.TxManager{})
	ann := w.person(t, "ann")

	tests := []struct {
		name string
		v    service.WorldVotes
		want error
	}{
		{"no votes", service.WorldVotes{OpID: uuid.Must(uuid.NewV7()), UserID: ann}, service.ErrWorldVotes},
		{"no operation", service.WorldVotes{UserID: ann, Votes: 1}, service.ErrWorldVotes},
		{"unknown page", service.WorldVotes{OpID: uuid.Must(uuid.NewV7()), UserID: uuid.Must(uuid.NewV7()), Votes: 1}, user.ErrNotFound},
		{"not settled", service.WorldVotes{OpID: uuid.Must(uuid.NewV7()), UserID: ann, Votes: 1, Settled: true}, user.ErrFirmNotYet},
	}
	for _, tt := range tests {
		if _, err := world.ChargeVotes(ctx, tt.v); !errors.Is(err, tt.want) {
			t.Errorf("%s: ChargeVotes = %v, want %v", tt.name, err, tt.want)
		}
	}
	if _, err := world.PayVotes(ctx, service.WorldVotes{UserID: ann, Votes: 1}); !errors.Is(err, service.ErrWorldVotes) {
		t.Errorf("PayVotes without operation = %v", err)
	}
}
