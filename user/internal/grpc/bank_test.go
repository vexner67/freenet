package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBank(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	if _, err := s.client.GetBank(context.Background(), &userv1.GetBankRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetBank by nobody: %v", err)
	}

	got, err := s.client.GetBank(ann, &userv1.GetBankRequest{})
	if err != nil || got.GetSettings().GetLoanMost() != user.DefaultBank.LoanMost || got.GetAccount().GetDeposit() != 0 ||
		got.GetAccount().GetDueAt() != nil {
		t.Fatalf("GetBank = %v, %v", got, err)
	}

	if _, err = s.client.PutDeposit(ann, &userv1.BankAmountRequest{Amount: 0}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PutDeposit of nothing: %v", err)
	}
	if _, err = s.client.PutDeposit(ann, &userv1.BankAmountRequest{Amount: 5}); status.Code(err) != codes.FailedPrecondition ||
		status.Convert(err).Message() != user.ErrNotEnoughVotes.Error() {
		t.Fatalf("PutDeposit without votes: %v", err)
	}
	if _, err = s.client.TakeDeposit(ann, &userv1.BankAmountRequest{Amount: 5}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("TakeDeposit of an empty deposit: %v", err)
	}
	if _, err = s.client.RepayLoan(ann, &userv1.BankAmountRequest{}); status.Code(err) != codes.FailedPrecondition ||
		status.Convert(err).Message() != user.ErrNoLoan.Error() {
		t.Fatalf("RepayLoan without a loan: %v", err)
	}

	// A page made today borrows nothing yet.
	if _, err = s.client.TakeLoan(ann, &userv1.BankAmountRequest{Amount: 10}); status.Code(err) != codes.FailedPrecondition ||
		status.Convert(err).Message() != user.ErrBankNotYet.Error() {
		t.Fatalf("TakeLoan of a new page: %v", err)
	}

	if _, err = s.client.GetBankSettings(ann, &userv1.GetBankSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetBankSettings by ann: %v", err)
	}

	settings, err := s.client.GetBankSettings(admin, &userv1.GetBankSettingsRequest{})
	if err != nil || settings.GetSettings().GetLoanDays() != int32(user.DefaultBank.LoanDays) {
		t.Fatalf("GetBankSettings = %v, %v", settings, err)
	}

	tuned := settings.GetSettings()
	tuned.LoanDays = 30
	if _, err = s.client.SetBankSettings(admin, &userv1.SetBankSettingsRequest{Settings: tuned}); err != nil {
		t.Fatal(err)
	}
	tuned.LoanDays = 0
	if _, err = s.client.SetBankSettings(admin, &userv1.SetBankSettingsRequest{Settings: tuned}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetBankSettings without a term: %v", err)
	}
	if got, err = s.client.GetBank(ann, &userv1.GetBankRequest{}); err != nil || got.GetSettings().GetLoanDays() != 30 {
		t.Fatalf("GetBank after SetBankSettings = %v, %v", got, err)
	}
}
