package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/purgeclient"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

type giftNote struct{ gift, sender, recipient uuid.UUID }

// fakeGiftNotifier records the gifts told to the notification service,
// and fails with err when set.
type fakeGiftNotifier struct {
	sent    []giftNote
	deleted []giftNote
	err     error
}

func (f *fakeGiftNotifier) GiftSent(_ context.Context, gift, sender, recipient uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, giftNote{gift, sender, recipient})
	return nil
}

func (f *fakeGiftNotifier) GiftDeleted(_ context.Context, gift, sender, recipient uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, giftNote{gift, sender, recipient})
	return nil
}

type giftWorld struct {
	users    *inmem.UserRepository
	userSvc  *service.UserService
	accounts *service.AccountService
	blocks   *service.BlockService
	follows  *service.FollowService
	gifts    *service.GiftService
	notifier *fakeGiftNotifier
}

func newGiftWorld() giftWorld {
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	blocks := inmem.NewBlockRepository(follows)
	notifier := &fakeGiftNotifier{}
	gifts := inmem.NewGiftRepository(users)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})

	return giftWorld{
		users:    users,
		userSvc:  service.NewUserService(users),
		accounts: service.NewAccountService(users, inmem.TxManager{}, purgeclient.NoReactions{}, time.Hour),
		blocks:   service.NewBlockService(users, blocks, inmem.TxManager{}, &fakeNotifier{}),
		follows: service.NewFollowService(service.FollowDeps{
			Users: users, Follows: follows, Lists: follows, Blocks: blocks, Tx: inmem.TxManager{}, Notifier: &fakeNotifier{},
		}),
		gifts: service.NewGiftService(service.GiftDeps{
			Users:        users,
			Gifts:        gifts,
			Prices:       gifts,
			Stock:        gifts,
			Collectibles: gifts,
			Auctions:     gifts,
			Permissions:  service.NewPermissionService(users, follows, blocks),
			Tx:           inmem.TxManager{},
			Wallets:      users,
			Notifier:     notifier,
			Admins:       admins,
		}),
		notifier: notifier,
	}
}

func (w giftWorld) person(t *testing.T, name string) uuid.UUID {
	t.Helper()

	id, err := w.userSvc.GetOrCreateByEmail(context.Background(), mustEmail(t, name+"@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	// Enough votes for any gift: what gifts cost is tested with the
	// wallets.
	if _, err = w.users.Credit(context.Background(), id, 1000, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	return id
}

func TestGiftService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newGiftWorld()
	ann, bob, cat := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	signed, err := w.gifts.Send(ctx, bob, ann, service.GiftDraft{Kind: "freenet_coin", Caption: "Привет"})
	if err != nil {
		t.Fatal(err)
	}
	if signed.Sender == nil || signed.Sender.ID != bob || signed.Caption != "Привет" || signed.Kind != "freenet_coin" {
		t.Fatalf("sent = %+v", signed)
	}

	secret, err := w.gifts.Send(ctx, cat, ann, service.GiftDraft{Kind: "freenet_ring", Caption: "Тайно", Anonymous: true})
	if err != nil {
		t.Fatal(err)
	}
	if secret.Sender == nil || !secret.Anonymous {
		t.Fatalf("the sender sees the anonymous gift signed: %+v", secret)
	}

	// To oneself: nobody hears.
	own, err := w.gifts.Send(ctx, ann, ann, service.GiftDraft{Kind: "freenet_crystal"})
	if err != nil {
		t.Fatal(err)
	}

	if want := []giftNote{{signed.ID, bob, ann}, {secret.ID, cat, ann}}; len(w.notifier.sent) != 2 || w.notifier.sent[0] != want[0] || w.notifier.sent[1] != want[1] {
		t.Fatalf("notified = %+v, want %+v", w.notifier.sent, want)
	}

	for name, tt := range map[string]struct {
		viewer     uuid.UUID
		secretSeen bool
	}{
		"the owner":  {ann, true},
		"the sender": {cat, true},
		"another":    {bob, false},
		"anonymous":  {uuid.Nil, false},
	} {
		page, err := w.gifts.List(ctx, tt.viewer, ann, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if page.Total != 3 || len(page.Gifts) != 3 || page.NextCursor != uuid.Nil {
			t.Fatalf("%s: page = %+v", name, page)
		}
		// Newest first.
		if page.Gifts[0].ID != own.ID || page.Gifts[1].ID != secret.ID || page.Gifts[2].ID != signed.ID {
			t.Fatalf("%s: order = %+v", name, page.Gifts)
		}

		got := page.Gifts[1]
		if seen := got.Sender != nil && got.Caption == "Тайно" && got.Anonymous; seen != tt.secretSeen {
			t.Fatalf("%s: the anonymous gift = %+v", name, got)
		}
		if !tt.secretSeen && (got.Sender != nil || got.Caption != "" || got.Kind != "freenet_ring") {
			t.Fatalf("%s: the anonymous gift tells %+v", name, got)
		}
		if page.Gifts[2].Sender == nil || page.Gifts[2].Sender.ID != bob {
			t.Fatalf("%s: the signed gift = %+v", name, page.Gifts[2])
		}
	}

	// Pages: one extra row tells whether there is a next one.
	first, err := w.gifts.List(ctx, ann, ann, uuid.Nil, 2)
	if err != nil || len(first.Gifts) != 2 || first.NextCursor != secret.ID {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	rest, err := w.gifts.List(ctx, ann, ann, first.NextCursor, 2)
	if err != nil || len(rest.Gifts) != 1 || rest.Gifts[0].ID != signed.ID || rest.NextCursor != uuid.Nil {
		t.Fatalf("second page = %+v, %v", rest, err)
	}

	// Only the owner deletes; the notification goes with the gift.
	if err = w.gifts.Delete(ctx, bob, signed.ID); !errors.Is(err, user.ErrCannotDeleteGift) {
		t.Fatalf("the sender deletes: %v", err)
	}
	if err = w.gifts.Delete(ctx, ann, signed.ID); err != nil {
		t.Fatal(err)
	}
	if err = w.gifts.Delete(ctx, ann, signed.ID); !errors.Is(err, user.ErrGiftNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
	if err = w.gifts.Delete(ctx, ann, own.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.notifier.deleted) != 1 || w.notifier.deleted[0] != (giftNote{signed.ID, bob, ann}) {
		t.Fatalf("taken back = %+v", w.notifier.deleted)
	}

	// A deleted page of the sender: the gift stays, unsigned.
	if _, err = w.accounts.Deactivate(ctx, cat, ""); err != nil {
		t.Fatal(err)
	}
	page, err := w.gifts.List(ctx, ann, ann, uuid.Nil, 10)
	if err != nil || len(page.Gifts) != 1 || page.Gifts[0].Sender != nil || page.Gifts[0].Caption != "Тайно" {
		t.Fatalf("after the sender is deleted: %+v, %v", page, err)
	}

	// Erased: the same, and the gifts sent to an erased page go with it.
	if _, err = w.gifts.Send(ctx, ann, bob, service.GiftDraft{Kind: "freenet_heart"}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.users.DeleteDue(ctx, time.Now().Add(2*time.Hour), 10); err != nil {
		t.Fatal(err)
	}
	if page, err = w.gifts.List(ctx, ann, ann, uuid.Nil, 10); err != nil || page.Total != 1 || page.Gifts[0].Sender != nil {
		t.Fatalf("after the sender is erased: %+v, %v", page, err)
	}
	if _, err = w.gifts.List(ctx, ann, cat, uuid.Nil, 10); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("the gifts of an erased page: %v", err)
	}
}

func TestGiftService_Refusals(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newGiftWorld()
	ann, bob, cat := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")
	draft := service.GiftDraft{Kind: "freenet_coin"}

	if _, err := w.gifts.Send(ctx, bob, uuid.New(), draft); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("to nobody: %v", err)
	}

	// A block shuts gifts both ways.
	if err := w.blocks.Block(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := w.gifts.Send(ctx, bob, ann, draft); !errors.Is(err, user.ErrGiftRefused) {
		t.Fatalf("to the one who blocked: %v", err)
	}
	if _, err := w.gifts.Send(ctx, ann, bob, draft); !errors.Is(err, user.ErrGiftRefused) {
		t.Fatalf("to the blocked: %v", err)
	}

	// "Кому видны мои подарки": the followed only.
	u, err := w.users.GetByID(ctx, ann)
	if err != nil {
		t.Fatal(err)
	}
	privacy := u.Privacy()
	privacy.GiftsRead = user.AudienceFollowing
	if _, err = w.userSvc.UpdatePrivacy(ctx, ann, privacy); err != nil {
		t.Fatal(err)
	}

	if _, err = w.gifts.Send(ctx, cat, ann, draft); err != nil {
		t.Fatalf("hidden gifts are still taken: %v", err)
	}
	if _, err = w.gifts.List(ctx, cat, ann, uuid.Nil, 10); !errors.Is(err, user.ErrGiftsHidden) {
		t.Fatalf("a stranger reads hidden gifts: %v", err)
	}
	if _, err = w.gifts.List(ctx, uuid.Nil, ann, uuid.Nil, 10); !errors.Is(err, user.ErrGiftsHidden) {
		t.Fatalf("anonymous reads hidden gifts: %v", err)
	}
	if err = w.follows.Follow(ctx, ann, cat); err != nil {
		t.Fatal(err)
	}
	if page, err := w.gifts.List(ctx, cat, ann, uuid.Nil, 10); err != nil || page.Total != 1 {
		t.Fatalf("the followed reads: %+v, %v", page, err)
	}
	if page, err := w.gifts.List(ctx, ann, ann, uuid.Nil, 10); err != nil || page.Total != 1 {
		t.Fatalf("the owner reads: %+v, %v", page, err)
	}

	// A deleted page takes no gifts and shows none.
	if _, err = w.accounts.Deactivate(ctx, ann, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = w.gifts.Send(ctx, cat, ann, draft); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("to a deleted page: %v", err)
	}
	if _, err = w.gifts.List(ctx, cat, ann, uuid.Nil, 10); !errors.Is(err, user.ErrGiftsHidden) {
		t.Fatalf("the gifts of a deleted page: %v", err)
	}

	// A notification that cannot be recorded fails the gift.
	w.notifier.err = errors.New("outbox is down")
	if _, err = w.gifts.Send(ctx, bob, cat, draft); !errors.Is(err, w.notifier.err) {
		t.Fatalf("with a broken outbox: %v", err)
	}
}

func TestGiftService_SetPrice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	blocks := inmem.NewBlockRepository(follows)
	gifts := inmem.NewGiftRepository(users)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	svc := service.NewGiftService(service.GiftDeps{
		Users: users, Gifts: gifts, Prices: gifts, Stock: gifts, Collectibles: gifts, Auctions: gifts, Permissions: service.NewPermissionService(users, follows, blocks),
		Tx: inmem.TxManager{}, Wallets: users, Notifier: &fakeGiftNotifier{}, Admins: admins,
	})

	newUser := func(email string) *user.User {
		t.Helper()
		u, err := user.New(user.Email(email))
		if err != nil {
			t.Fatal(err)
		}
		if err = users.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	admin, ann := newUser("admin@example.com"), newUser("ann@example.com")

	if err := svc.SetPrice(ctx, ann.ID(), "freenet_coin", 3); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("not an administrator prices a gift: %v", err)
	}
	if err := svc.SetPrice(ctx, admin.ID(), "freenet_coin", 3); err != nil {
		t.Fatal(err)
	}

	catalog, err := svc.Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, shelf := range catalog {
		for _, g := range shelf.Gifts {
			want := g.Kind.DefaultPrice()
			if g.Kind == "freenet_coin" {
				want = 3
			}
			if g.Price != want {
				t.Errorf("%s costs %d, want %d", g.Kind, g.Price, want)
			}
		}
	}

	// One without votes cannot send the coin at its new price.
	if _, err = svc.Send(ctx, ann.ID(), ann.ID(), service.GiftDraft{Kind: "freenet_coin"}); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("a priced coin is sent for nothing: %v", err)
	}
	if _, err = users.Credit(ctx, ann.ID(), 5, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, ann.ID(), ann.ID(), service.GiftDraft{Kind: "freenet_coin"}); err != nil {
		t.Fatal(err)
	}
	if left, err := users.Spend(ctx, ann.ID(), 0, user.VoteEntry{}); err != nil || left != 2 {
		t.Fatalf("after a coin of 3 out of 5: %d, %v", left, err)
	}
}
