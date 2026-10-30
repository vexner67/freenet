package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPetGrants(t *testing.T) {
	repo, pool := newRepo(t)
	pets := postgres.NewPetRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ann := savedUser(t, repo, "grant")
	now := time.Now().UTC().Truncate(time.Microsecond)

	if got, err := pets.Granted(ctx, ann, user.PetBig2Z); err != nil || got {
		t.Fatalf("Granted before = %v, %v", got, err)
	}

	// A person with all their pets still gets it.
	for range user.MaxPets {
		if err := pets.AddPet(ctx, ann, user.NewPet(user.PetCat, "Кот", now)); err != nil {
			t.Fatal(err)
		}
	}
	big := user.NewPet(user.PetBig2Z, user.PetBig2ZName, now)
	if err := pets.GrantPet(ctx, ann, big, now); err != nil {
		t.Fatal(err)
	}
	if got, err := pets.Pet(ctx, ann, big.ID()); err != nil || got.Kind() != user.PetBig2Z {
		t.Fatalf("Pet = %+v, %v", got, err)
	}
	if got, err := pets.Granted(ctx, ann, user.PetBig2Z); err != nil || !got {
		t.Fatalf("Granted after = %v, %v", got, err)
	}

	again := user.NewPet(user.PetBig2Z, user.PetBig2ZName, now)
	if err := pets.GrantPet(ctx, ann, again, now); !errors.Is(err, user.ErrPetClaimed) {
		t.Fatalf("a second grant: %v", err)
	}
	if err := pets.GrantPet(ctx, uuid.New(), again, now); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("a grant to nobody: %v", err)
	}
}
