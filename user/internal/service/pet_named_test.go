package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestClaimBig2Z(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")
	repo := inmem.NewPetRepository(w.users)
	pets := service.NewPetService(w.users, service.PetStores{Pets: repo, Keepers: repo, Battles: repo, Events: repo, Seasons: repo, Quests: repo, Market: repo}, w.users, inmem.TxManager{}, user.Admins{})

	if _, err := pets.ClaimBig2Z(ctx, ann, user.PetBig2ZOpensAt.Add(-time.Second)); !errors.Is(err, user.ErrPetLocked) {
		t.Fatalf("claim before it opens: %v", err)
	}
	if named, err := pets.Big2Z(ctx, ann); err != nil || named.Claimed || !named.OpensAt.Equal(user.PetBig2ZOpensAt) {
		t.Fatalf("Big2Z = %+v, %v", named, err)
	}

	pet, err := pets.ClaimBig2Z(ctx, ann, user.PetBig2ZOpensAt)
	if err != nil || pet.Kind() != user.PetBig2Z || pet.Name() != user.PetBig2ZName {
		t.Fatalf("claim = %+v, %v", pet, err)
	}
	if _, err = pets.ClaimBig2Z(ctx, ann, user.PetBig2ZOpensAt); !errors.Is(err, user.ErrPetClaimed) {
		t.Fatalf("second claim: %v", err)
	}
	if named, err := pets.Big2Z(ctx, ann); err != nil || !named.Claimed {
		t.Fatalf("Big2Z after claim = %+v, %v", named, err)
	}
}
