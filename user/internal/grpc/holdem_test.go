package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHoldem(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")
	eve, _ := s.member(t, "eve")

	if _, err := s.client.QuickHoldem(ann, &userv1.QuickHoldemRequest{BigBlind: 3, BuyIn: 100}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("QuickHoldem of no stake: %v", err)
	}
	if _, err := s.client.QuickHoldem(ann, &userv1.QuickHoldemRequest{BigBlind: 2, BuyIn: 100}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("QuickHoldem without votes: %v", err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 500, Usages: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{ann, bob, eve} {
		if _, err = s.client.RedeemVoucher(ctx, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := s.client.QuickHoldem(ann, &userv1.QuickHoldemRequest{BigBlind: 2, BuyIn: 100})
	if err != nil || first.GetBalance() != 400 || first.GetTable().GetYou() != 0 || first.GetTable().GetPlayers() != 1 {
		t.Fatalf("QuickHoldem = %v, %v", first, err)
	}
	id := first.GetTable().GetId()

	second, err := s.client.SitHoldemTable(bob, &userv1.SitHoldemTableRequest{Id: id, Seat: -1, BuyIn: 40})
	if err != nil || second.GetTable().GetPlayers() != 2 || second.GetTable().GetYou() != 1 {
		t.Fatalf("SitHoldemTable = %v, %v", second, err)
	}

	// A guest sees the table and nobody's cards.
	seen, err := s.client.GetHoldemTable(context.Background(), &userv1.HoldemTableRequest{Id: id})
	if err != nil || seen.GetYou() != -1 || len(seen.GetSeats()) != 6 || seen.GetSeats()[0].GetStack() != 100 {
		t.Fatalf("GetHoldemTable = %v, %v", seen, err)
	}
	for _, seat := range seen.GetSeats() {
		if len(seat.GetCards()) != 0 {
			t.Fatalf("a guest sees %v", seat.GetCards())
		}
	}

	if listed, err := s.client.ListHoldemTables(context.Background(), &userv1.ListHoldemTablesRequest{}); err != nil || len(listed.GetTables()) != 1 {
		t.Fatalf("ListHoldemTables = %v, %v", listed, err)
	}
	if _, err := s.client.ListHoldemTables(ann, &userv1.ListHoldemTablesRequest{Section: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListHoldemTables of no section: %v", err)
	}
	if _, err := s.client.GetHoldemTable(ann, &userv1.HoldemTableRequest{Id: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetHoldemTable of nothing: %v", err)
	}
	if _, err := s.client.SitHoldemTable(bob, &userv1.SitHoldemTableRequest{Id: id, Seat: -1, BuyIn: 40}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SitHoldemTable twice: %v", err)
	}
	if _, err := s.client.ActHoldemTable(eve, &userv1.ActHoldemTableRequest{Id: id, Action: "call"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("ActHoldemTable of a stranger: %v", err)
	}

	// A private table is the owner's and the invited's.
	own, err := s.client.CreateHoldemTable(ann, &userv1.CreateHoldemTableRequest{Name: "Свои", Private: true, Seats: 2, BigBlind: 2, BuyIn: 40})
	if err != nil {
		t.Fatal(err)
	}
	ownID := own.GetTable().GetId()
	if _, err := s.client.GetHoldemTable(eve, &userv1.HoldemTableRequest{Id: ownID}); status.Code(err) != codes.NotFound {
		t.Fatalf("a private table to a stranger: %v", err)
	}
	if _, err := s.client.InviteHoldemTable(ann, &userv1.InviteHoldemTableRequest{Id: ownID, Nicknames: []string{"eve", "!"}}); err != nil {
		t.Fatal(err)
	}
	invited, err := s.client.ListHoldemTables(eve, &userv1.ListHoldemTablesRequest{Section: "invited"})
	if err != nil || len(invited.GetTables()) != 1 || !invited.GetTables()[0].GetInvited() {
		t.Fatalf("invited = %v, %v", invited, err)
	}

	left, err := s.client.LeaveHoldemTable(bob, &userv1.HoldemTableRequest{Id: id})
	if err != nil || left.GetBalance() != 500 || left.GetTable() == nil {
		t.Fatalf("LeaveHoldemTable = %v, %v", left, err)
	}
	if left, err = s.client.LeaveHoldemTable(ann, &userv1.HoldemTableRequest{Id: id}); err != nil || left.GetTable() != nil {
		t.Fatalf("LeaveHoldemTable of the last = %v, %v", left, err)
	}
}
