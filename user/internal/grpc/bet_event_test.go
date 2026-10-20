package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBetEvents(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	question := &userv1.CreateBetEventRequest{Question: "Проиграют ли республиканцы?", Outcomes: []string{"Да", "Нет"}}
	if _, err := s.client.CreateBetEvent(ann, question); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("CreateBetEvent by ann: %v", err)
	}
	if _, err := s.client.CreateBetEvent(admin, &userv1.CreateBetEventRequest{Question: "?", Outcomes: []string{"Да"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateBetEvent with one outcome: %v", err)
	}
	e, err := s.client.CreateBetEvent(admin, question)
	if err != nil || !e.GetOpen() || e.GetState() != "open" || e.GetWinner() != -1 || len(e.GetOutcomes()) != 2 || e.GetClosesAt() != nil {
		t.Fatalf("CreateBetEvent = %v, %v", e, err)
	}

	stake := &userv1.StakeBetRequest{EventId: e.GetId(), Outcome: 1, Amount: 4}
	if _, err := s.client.StakeBet(ann, stake); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("StakeBet without votes: %v", err)
	}
	if _, err := s.client.StakeBet(context.Background(), stake); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("StakeBet by nobody: %v", err)
	}
	if _, err := s.client.StakeBet(ann, &userv1.StakeBetRequest{EventId: e.GetId(), Outcome: 7, Amount: 4}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("StakeBet on outcome 7: %v", err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 10, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.client.StakeBet(ann, stake); err != nil || got.GetBalance() != 6 {
		t.Fatalf("StakeBet = %v, %v", got, err)
	}

	// Anyone sees the events; the caller sees their stakes too.
	list, err := s.client.ListBetEvents(context.Background(), &userv1.ListBetEventsRequest{Popular: true})
	if err != nil || len(list.GetEvents()) != 1 || list.GetEvents()[0].GetOutcomes()[1].GetStaked() != 4 || list.GetEvents()[0].GetOutcomes()[1].GetMine() != 0 {
		t.Fatalf("ListBetEvents by a guest = %v, %v", list, err)
	}

	if _, err := s.client.DeleteBetEvent(admin, &userv1.BetEventRequest{EventId: e.GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteBetEvent of an open event: %v", err)
	}
	if _, err := s.client.ResolveBetEvent(admin, &userv1.ResolveBetEventRequest{EventId: e.GetId(), Winner: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.CancelBetEvent(admin, &userv1.BetEventRequest{EventId: e.GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CancelBetEvent after resolve: %v", err)
	}

	list, err = s.client.ListBetEvents(ann, &userv1.ListBetEventsRequest{})
	if err != nil || len(list.GetEvents()) != 1 {
		t.Fatalf("ListBetEvents = %v, %v", list, err)
	}
	got := list.GetEvents()[0]
	if got.GetState() != "resolved" || got.GetWinner() != 1 || got.GetOpen() || got.GetPayout() != 4 || got.GetOutcomes()[1].GetMine() != 4 || got.GetPeople() != 1 {
		t.Fatalf("resolved event = %v", got)
	}

	if _, err := s.client.CloseBetEvent(admin, &userv1.BetEventRequest{EventId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CloseBetEvent of a bad id: %v", err)
	}
	if _, err := s.client.DeleteBetEvent(admin, &userv1.BetEventRequest{EventId: e.GetId()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.CloseBetEvent(admin, &userv1.BetEventRequest{EventId: e.GetId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("CloseBetEvent of a deleted event: %v", err)
	}
}
