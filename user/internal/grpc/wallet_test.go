package grpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVouchersAndVotes(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")

	// GetMe tells who runs the site and what one has.
	if me, err := s.client.GetMe(admin, &userv1.GetMeRequest{}); err != nil || !me.GetUser().GetAdmin() {
		t.Fatalf("the admin's GetMe = %v, %v", me, err)
	}
	if me, err := s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetAdmin() || me.GetUser().GetVotes() != 0 {
		t.Fatalf("ann's GetMe = %v, %v", me, err)
	}

	// Vouchers are the administrators'.
	if _, err := s.client.CreateVoucher(ann, &userv1.CreateVoucherRequest{Votes: 5, Usages: 1}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("CreateVoucher by ann: %v", err)
	}
	if _, err := s.client.ListVouchers(ann, &userv1.ListVouchersRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ListVouchers by ann: %v", err)
	}
	if _, err := s.client.DeleteVoucher(ann, &userv1.DeleteVoucherRequest{VoucherId: uuid.NewString()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DeleteVoucher by ann: %v", err)
	}
	if _, err := s.client.CreateVoucher(context.Background(), &userv1.CreateVoucherRequest{Votes: 5, Usages: 1}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateVoucher by nobody: %v", err)
	}

	invalid := []*userv1.CreateVoucherRequest{
		{Usages: 1},
		{Votes: -1, Usages: 1},
		{Votes: 1, Rating: 1001, Usages: 1},
		{Votes: 1},
		{Votes: 1, Usages: 1, Token: "short"},
	}
	for _, req := range invalid {
		if _, err := s.client.CreateVoucher(admin, req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("CreateVoucher(%v): %v", req, err)
		}
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 10, Rating: 30, Usages: 1})
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	voucher := created.GetVoucher()
	if len(voucher.GetToken()) != 24 || voucher.GetVotes() != 10 || voucher.GetRating() != 30 || voucher.GetUsagesLeft() != 1 || voucher.GetActivations() != 0 {
		t.Fatalf("voucher = %v", voucher)
	}

	named, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Token: "welcome-0000-0000-0000-00000", Rating: 5, Usages: -1})
	if err != nil || named.GetVoucher().GetToken() != "WELCOME00000000000000000" || named.GetVoucher().GetUsagesLeft() != -1 {
		t.Fatalf("a named voucher = %v, %v", named, err)
	}
	if _, err = s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Token: "WELCOME00000000000000000", Votes: 1, Usages: 1}); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("the same serial number: %v", err)
	}

	// Typed in lower case with dashes.
	typed := strings.ToLower(voucher.GetToken()[:12]) + "-" + voucher.GetToken()[12:]
	redeemed, err := s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: typed})
	if err != nil || redeemed.GetVotes() != 10 || redeemed.GetRating() != 30 || redeemed.GetBalance() != 10 {
		t.Fatalf("RedeemVoucher = %v, %v", redeemed, err)
	}

	for name, token := range map[string]string{"used": voucher.GetToken(), "unknown": "ZZZZZZZZZZZZZZZZZZZZZZZZ", "malformed": "what"} {
		_, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: token})
		if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "voucher is not valid" {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err = s.client.RedeemVoucher(bob, &userv1.RedeemVoucherRequest{Token: voucher.GetToken()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a voucher that ran out: %v", err)
	}

	// The rating shows on the page, the votes only to their owner.
	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil || me.GetUser().GetVotes() != 10 || me.GetUser().GetCompleteness() != 40 {
		t.Fatalf("ann's GetMe = %v, %v", me, err)
	}
	page, err := s.client.GetProfile(bob, &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil || page.GetProfile().GetCompleteness() != 40 {
		t.Fatalf("ann's page = %v, %v", page, err)
	}

	list, err := s.client.ListVouchers(admin, &userv1.ListVouchersRequest{Limit: 1})
	if err != nil || len(list.GetVouchers()) != 1 || list.GetNextCursor() == "" || list.GetVouchers()[0].GetId() != named.GetVoucher().GetId() {
		t.Fatalf("the first page = %v, %v", list, err)
	}
	list, err = s.client.ListVouchers(admin, &userv1.ListVouchersRequest{Limit: 1, Cursor: list.GetNextCursor()})
	if err != nil || len(list.GetVouchers()) != 1 || list.GetNextCursor() != "" {
		t.Fatalf("the second page = %v, %v", list, err)
	}
	if used := list.GetVouchers()[0]; used.GetId() != voucher.GetId() || used.GetUsagesLeft() != 0 || used.GetActivations() != 1 {
		t.Fatalf("the used voucher = %v", used)
	}
	if _, err = s.client.ListVouchers(admin, &userv1.ListVouchersRequest{Cursor: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong cursor: %v", err)
	}

	if _, err = s.client.DeleteVoucher(admin, &userv1.DeleteVoucherRequest{VoucherId: named.GetVoucher().GetId()}); err != nil {
		t.Fatalf("DeleteVoucher: %v", err)
	}
	if _, err = s.client.RedeemVoucher(bob, &userv1.RedeemVoucherRequest{Token: "WELCOME00000000000000000"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a deleted voucher: %v", err)
	}
	if _, err = s.client.DeleteVoucher(admin, &userv1.DeleteVoucherRequest{VoucherId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong voucher id: %v", err)
	}

	// Transfers.
	sent, err := s.client.TransferVotes(ann, &userv1.TransferVotesRequest{UserId: bobID, Votes: 4, Note: "За пиццу"})
	if err != nil || sent.GetVotes() != 5 || sent.GetFee() != 1 {
		t.Fatalf("TransferVotes = %v, %v", sent, err)
	}

	statement, err := s.client.ListVoteOperations(bob, &userv1.ListVoteOperationsRequest{})
	if err != nil || len(statement.GetOperations()) != 1 || statement.GetIncome() != 4 || statement.GetExpense() != 0 || statement.GetNextCursor() != "" {
		t.Fatalf("bob's ListVoteOperations = %v, %v", statement, err)
	}
	if op := statement.GetOperations()[0]; op.GetKind() != "transfer" || op.GetAmount() != 4 || op.GetBalance() != 4 ||
		op.GetPeerId() != annID || op.GetNote() != "За пиццу" || op.GetCreatedAt() == nil {
		t.Fatalf("bob's transfer = %v", op)
	}
	statement, err = s.client.ListVoteOperations(ann, &userv1.ListVoteOperationsRequest{Flow: "in", Limit: 1})
	if err != nil || len(statement.GetOperations()) != 1 || statement.GetOperations()[0].GetKind() != "voucher" || statement.GetOperations()[0].GetPeerId() != "" {
		t.Fatalf("what came to ann = %v, %v", statement, err)
	}
	if _, err = s.client.ListVoteOperations(ann, &userv1.ListVoteOperationsRequest{Flow: "up"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong flow: %v", err)
	}
	if _, err = s.client.ListVoteOperations(ann, &userv1.ListVoteOperationsRequest{Cursor: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong cursor: %v", err)
	}
	if _, err = s.client.ListVoteOperations(context.Background(), &userv1.ListVoteOperationsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("a guest's statement: %v", err)
	}
	if me, err = s.client.GetMe(bob, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetVotes() != 4 {
		t.Fatalf("bob's GetMe = %v, %v", me, err)
	}

	refusals := []struct {
		name string
		req  *userv1.TransferVotesRequest
		want codes.Code
	}{
		{"nothing", &userv1.TransferVotesRequest{UserId: bobID}, codes.InvalidArgument},
		{"too much at once", &userv1.TransferVotesRequest{UserId: bobID, Votes: 100_000_001}, codes.InvalidArgument},
		{"more than there is", &userv1.TransferVotesRequest{UserId: bobID, Votes: 5}, codes.FailedPrecondition},
		{"to oneself", &userv1.TransferVotesRequest{UserId: annID, Votes: 1}, codes.InvalidArgument},
		{"to nobody", &userv1.TransferVotesRequest{UserId: uuid.NewString(), Votes: 1}, codes.NotFound},
		{"to a wrong id", &userv1.TransferVotesRequest{UserId: "nope", Votes: 1}, codes.InvalidArgument},
		{"a note of two lines", &userv1.TransferVotesRequest{UserId: bobID, Votes: 1, Note: "раз\nдва"}, codes.InvalidArgument},
	}
	for _, tt := range refusals {
		if _, err = s.client.TransferVotes(ann, tt.req); status.Code(err) != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}

	if _, err = s.client.Block(bob, &userv1.BlockRequest{UserId: annID}); err != nil {
		t.Fatalf("Block: %v", err)
	}
	_, err = s.client.TransferVotes(ann, &userv1.TransferVotesRequest{UserId: bobID, Votes: 1})
	if status.Code(err) != codes.PermissionDenied || status.Convert(err).Message() != "you cannot transfer votes to this user" {
		t.Fatalf("across a blacklist: %v", err)
	}
}

func TestGiftPrices(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	_, bobID := s.member(t, "bob")

	catalog, err := s.client.GetGiftCatalog(context.Background(), &userv1.GetGiftCatalogRequest{})
	if err != nil {
		t.Fatalf("GetGiftCatalog: %v", err)
	}

	var paid *userv1.GiftCollection
	for _, c := range catalog.GetCollections() {
		if c.GetPrice() > 0 {
			paid = c
		}
	}
	if paid == nil || catalog.GetCollections()[0].GetPrice() != 0 {
		t.Fatalf("the catalogue has a free collection first and a paid one: %v", catalog)
	}

	_, err = s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: bobID, Gift: paid.GetGifts()[0]})
	if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "not enough votes" {
		t.Fatalf("a paid gift with no votes: %v", err)
	}

	voucher, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: paid.GetPrice(), Usages: 1})
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: voucher.GetVoucher().GetToken()}); err != nil {
		t.Fatalf("RedeemVoucher: %v", err)
	}

	if _, err = s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: bobID, Gift: paid.GetGifts()[0]}); err != nil {
		t.Fatalf("a paid gift: %v", err)
	}
	if me, err := s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetVotes() != 0 {
		t.Fatalf("after the gift ann has %v, %v", me.GetUser().GetVotes(), err)
	}
}

func TestSetGiftPrice(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	if _, err := s.client.SetGiftPrice(ann, &userv1.SetGiftPriceRequest{Gift: "rose", Price: 2}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("not an administrator prices a gift: %v", err)
	}
	for _, req := range []*userv1.SetGiftPriceRequest{{Gift: "nothing", Price: 2}, {Gift: "rose", Price: -1}, {Gift: "rose", Price: 1_000_001}} {
		if _, err := s.client.SetGiftPrice(admin, req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("SetGiftPrice(%v): %v", req, err)
		}
	}
	if _, err := s.client.SetGiftPrice(admin, &userv1.SetGiftPriceRequest{Gift: "rose", Price: 2}); err != nil {
		t.Fatalf("SetGiftPrice: %v", err)
	}

	catalog, err := s.client.GetGiftCatalog(context.Background(), &userv1.GetGiftCatalogRequest{})
	if err != nil {
		t.Fatalf("GetGiftCatalog: %v", err)
	}
	flowers := catalog.GetCollections()[0]
	if flowers.GetPrice() != 0 || len(flowers.GetPrices()) != len(flowers.GetGifts()) {
		t.Fatalf("the flowers: %v", flowers)
	}
	for i, gift := range flowers.GetGifts() {
		want := int64(0)
		if gift == "rose" {
			want = 2
		}
		if flowers.GetPrices()[i] != want {
			t.Errorf("%s costs %d, want %d", gift, flowers.GetPrices()[i], want)
		}
	}
}

func TestVoteRate(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	// Anyone, even without a token, reads the rate.
	if got, err := s.client.GetVoteRate(context.Background(), &userv1.GetVoteRateRequest{}); err != nil || got.GetKopecks() != 700 {
		t.Fatalf("GetVoteRate = %v, %v", got, err)
	}

	if _, err := s.client.SetVoteRate(ann, &userv1.SetVoteRateRequest{Kopecks: 1000}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("not an administrator sets the rate: %v", err)
	}
	for _, kopecks := range []int64{0, -1, 100_000_001} {
		if _, err := s.client.SetVoteRate(admin, &userv1.SetVoteRateRequest{Kopecks: kopecks}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("SetVoteRate(%d): %v", kopecks, err)
		}
	}
	if _, err := s.client.SetVoteRate(admin, &userv1.SetVoteRateRequest{Kopecks: 1250}); err != nil {
		t.Fatalf("SetVoteRate: %v", err)
	}
	if got, err := s.client.GetVoteRate(ann, &userv1.GetVoteRateRequest{}); err != nil || got.GetKopecks() != 1250 {
		t.Fatalf("GetVoteRate after = %v, %v", got, err)
	}
}
