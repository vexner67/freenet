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

func TestPurchaseRepository(t *testing.T) {
	repo, pool := newRepo(t)
	purchases := postgres.NewPurchaseRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	people := make([]uuid.UUID, 2)
	for i := range people {
		u, err := user.New(user.Email(unique("paid") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		people[i] = u.ID()
	}
	buyer, author := people[0], people[1]
	post, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	added, err := purchases.AddPurchase(ctx, buyer, post, author, 10)
	if err != nil || !added {
		t.Fatalf("AddPurchase = %v, %v", added, err)
	}
	if added, err = purchases.AddPurchase(ctx, buyer, post, author, 10); err != nil || added {
		t.Fatalf("AddPurchase again = %v, %v", added, err)
	}

	bought, err := purchases.Bought(ctx, buyer, []uuid.UUID{post, other})
	if err != nil || len(bought) != 1 || bought[0] != post {
		t.Fatalf("Bought = %v, %v", bought, err)
	}

	payment := user.MessagePayment{ID: uuid.Must(uuid.NewV7()), SenderID: buyer, RecipientID: author, Price: 20, Tax: 2}
	if err = purchases.SavePayment(ctx, payment); err != nil {
		t.Fatal(err)
	}
	if _, err = purchases.TakePayment(ctx, payment.ID, author); !errors.Is(err, user.ErrPaymentNotFound) {
		t.Fatalf("TakePayment by another: %v", err)
	}
	taken, err := purchases.TakePayment(ctx, payment.ID, buyer)
	if err != nil || taken != payment {
		t.Fatalf("TakePayment = %+v, %v", taken, err)
	}
	if _, err = purchases.TakePayment(ctx, payment.ID, buyer); !errors.Is(err, user.ErrPaymentNotFound) {
		t.Fatalf("TakePayment twice: %v", err)
	}
}

func TestUserRepository_MessagePrivacy(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	u, err := user.New(user.Email(unique("privacy") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	privacy := u.Privacy()
	privacy.MessagesPrime = true
	privacy.MessagePrice, _ = user.NewMessagePrice(42)
	if err = u.UpdatePrivacy(privacy); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil || !got.Privacy().MessagesPrime || got.Privacy().MessagePrice != 42 {
		t.Fatalf("GetByID = %+v, %v", got.Privacy(), err)
	}
}
