package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPoker(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")

	// Anyone sees the table; a guest has no hand.
	got, err := s.client.GetPoker(context.Background(), &userv1.GetPokerRequest{})
	if err != nil || got.GetPoker().GetMost() != 100 || len(got.GetPoker().GetPays()) != 6 || got.GetDealt() != nil {
		t.Fatalf("GetPoker = %v, %v", got, err)
	}

	tuned := &userv1.SetPokerSettingsRequest{Poker: &userv1.Poker{Least: 2, Most: 10, Pays: []int32{100, 20, 10, 3, 2, 1}}}
	if _, err := s.client.SetPokerSettings(ann, tuned); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetPokerSettings by ann: %v", err)
	}
	if _, err := s.client.SetPokerSettings(admin, &userv1.SetPokerSettingsRequest{Poker: &userv1.Poker{Least: 2, Most: 10, Pays: []int32{1}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetPokerSettings with one pay: %v", err)
	}
	if _, err := s.client.SetPokerSettings(admin, tuned); err != nil {
		t.Fatal(err)
	}

	if _, err := s.client.DealPoker(ann, &userv1.DealPokerRequest{Ante: 5}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DealPoker without votes: %v", err)
	}
	if _, err := s.client.DealPoker(ann, &userv1.DealPokerRequest{Ante: 11}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("DealPoker above the most: %v", err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 50, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	hand, err := s.client.DealPoker(ann, &userv1.DealPokerRequest{Ante: 5})
	if err != nil || hand.GetBalance() != 45 || hand.GetState() != userv1.PokerState_POKER_STATE_DEALT ||
		len(hand.GetPlayer()) != 2 || len(hand.GetBoard()) != 3 || len(hand.GetDealer()) != 0 || hand.GetDealerHand() != "" {
		t.Fatalf("DealPoker = %v, %v", hand, err)
	}
	if _, err := s.client.DealPoker(ann, &userv1.DealPokerRequest{Ante: 5}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DealPoker twice: %v", err)
	}
	if table, err := s.client.GetPoker(ann, &userv1.GetPokerRequest{}); err != nil || table.GetDealt().GetId() != hand.GetId() {
		t.Fatalf("GetPoker of ann = %v, %v", table, err)
	}

	id := hand.GetId()
	if _, err := s.client.CallPoker(bob, &userv1.PokerHandRequest{HandId: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("CallPoker by bob: %v", err)
	}
	if _, err := s.client.FoldPoker(ann, &userv1.PokerHandRequest{HandId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FoldPoker of no id: %v", err)
	}
	if _, err := s.client.FoldPoker(ann, &userv1.PokerHandRequest{HandId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("FoldPoker of nothing: %v", err)
	}

	called, err := s.client.CallPoker(ann, &userv1.PokerHandRequest{HandId: id})
	if err != nil || called.GetState() == userv1.PokerState_POKER_STATE_DEALT || len(called.GetBoard()) != 5 ||
		len(called.GetDealer()) != 2 || called.GetDealerHand() == "" || len(called.GetPlayerBest()) != 5 ||
		called.GetBalance() != 35+called.GetPayout() {
		t.Fatalf("CallPoker = %v, %v", called, err)
	}
	if _, err := s.client.FoldPoker(ann, &userv1.PokerHandRequest{HandId: id}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("FoldPoker after the call: %v", err)
	}

	settings, err := s.client.GetPokerSettings(admin, &userv1.GetPokerSettingsRequest{})
	if err != nil || settings.GetPoker().GetLeast() != 2 || settings.GetHands() != 1 || settings.GetSpent() != 15 || settings.GetWon() != called.GetPayout() {
		t.Fatalf("GetPokerSettings = %v, %v", settings, err)
	}
	if _, err := s.client.GetPokerSettings(ann, &userv1.GetPokerSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetPokerSettings by ann: %v", err)
	}
}
