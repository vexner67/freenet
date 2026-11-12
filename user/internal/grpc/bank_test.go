package grpc_test

import (
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetiredBank(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")

	if _, err := s.client.GetBank(ann, &userv1.GetBankRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("GetBank: %v", err)
	}
	if _, err := s.client.PutDeposit(ann, &userv1.BankAmountRequest{Amount: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("PutDeposit: %v", err)
	}
	if _, err := s.client.TakeLoan(ann, &userv1.BankAmountRequest{Amount: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("TakeLoan: %v", err)
	}
	if _, err := s.client.IssueBankCard(ann, &userv1.IssueBankCardRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("IssueBankCard: %v", err)
	}
}
