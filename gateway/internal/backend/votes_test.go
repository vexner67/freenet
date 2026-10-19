package backend

import (
	"context"
	"math"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeVotesAPI struct {
	userv1.UserServiceClient
	md          metadata.MD
	transferred *userv1.TransferVotesRequest
	redeemed    string
	created     *userv1.CreateVoucherRequest
	listed      *userv1.ListVouchersRequest
	deleted     string
	rate        int64
	err         error
}

func (f *fakeVotesAPI) GetVoteRate(context.Context, *userv1.GetVoteRateRequest, ...grpc.CallOption) (*userv1.GetVoteRateResponse, error) {
	return &userv1.GetVoteRateResponse{Kopecks: 700}, f.err
}

func (f *fakeVotesAPI) SetVoteRate(ctx context.Context, req *userv1.SetVoteRateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.rate = req.GetKopecks()

	return &emptypb.Empty{}, f.err
}

func (f *fakeVotesAPI) TransferVotes(ctx context.Context, req *userv1.TransferVotesRequest, _ ...grpc.CallOption) (*userv1.TransferVotesResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.transferred = req

	return &userv1.TransferVotesResponse{Votes: 6}, f.err
}

func (f *fakeVotesAPI) RedeemVoucher(ctx context.Context, req *userv1.RedeemVoucherRequest, _ ...grpc.CallOption) (*userv1.RedeemVoucherResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.redeemed = req.GetToken()

	return &userv1.RedeemVoucherResponse{Votes: 10, Rating: 5, Balance: 12}, f.err
}

func (f *fakeVotesAPI) CreateVoucher(ctx context.Context, req *userv1.CreateVoucherRequest, _ ...grpc.CallOption) (*userv1.CreateVoucherResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.created = req

	return &userv1.CreateVoucherResponse{Voucher: &userv1.Voucher{
		Id: "v1", Token: "ABCDEFGH12345678JKLMNPQR", Votes: req.GetVotes(), Rating: req.GetRating(), UsagesLeft: req.GetUsages(),
	}}, f.err
}

func (f *fakeVotesAPI) ListVouchers(ctx context.Context, req *userv1.ListVouchersRequest, _ ...grpc.CallOption) (*userv1.ListVouchersResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.listed = req

	return &userv1.ListVouchersResponse{
		Vouchers: []*userv1.Voucher{
			{Id: "v2", Token: "WELCOME00000000000000000", Rating: 20, UsagesLeft: -1, Activations: 3},
			{Id: "v1", Token: "ABCDEFGH12345678JKLMNPQR", Votes: 10, Activations: 2},
		},
		NextCursor: "v1",
	}, f.err
}

func (f *fakeVotesAPI) DeleteVoucher(_ context.Context, req *userv1.DeleteVoucherRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.deleted = req.GetVoucherId()
	return &emptypb.Empty{}, f.err
}

func TestVotes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeVotesAPI{}
	votes := NewVotes(api)

	left, err := votes.Transfer(ctx, "tok", "u2", 4)
	if err != nil || left != 6 || api.transferred.GetUserId() != "u2" || api.transferred.GetVotes() != 4 || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("Transfer = %d, %v; asked %v", left, err, api.transferred)
	}

	redeemed, err := votes.Redeem(ctx, "tok", "abcd-efgh")
	if err != nil || redeemed != (model.Redeemed{Votes: 10, Rating: 5, Balance: 12}) || api.redeemed != "abcd-efgh" {
		t.Fatalf("Redeem = %+v, %v; asked %q", redeemed, err, api.redeemed)
	}

	voucher, err := votes.CreateVoucher(ctx, "tok", model.VoucherDraft{Token: "abc", Votes: 10, Rating: 5, Usages: -1})
	if err != nil || api.created.GetToken() != "abc" || api.created.GetVotes() != 10 || api.created.GetRating() != 5 || api.created.GetUsages() != -1 {
		t.Fatalf("created %v, %v", api.created, err)
	}
	if want := (model.Voucher{ID: "v1", Token: "ABCDEFGH12345678JKLMNPQR", Votes: 10, Rating: 5, UsagesLeft: -1}); voucher != want {
		t.Fatalf("CreateVoucher = %+v", voucher)
	}

	// Numbers that do not fit stay out of range rather than wrap around.
	if _, err = votes.CreateVoucher(ctx, "tok", model.VoucherDraft{Votes: 1, Rating: math.MaxInt64, Usages: math.MinInt64}); err != nil ||
		api.created.GetRating() != math.MaxInt32 || api.created.GetUsages() != math.MinInt32 {
		t.Fatalf("clamped %v, %v", api.created, err)
	}

	page, err := votes.ListVouchers(ctx, "tok", "v5", 2)
	if err != nil || api.listed.GetCursor() != "v5" || api.listed.GetLimit() != 2 {
		t.Fatalf("listed %v, %v", api.listed, err)
	}
	if page.NextCursor != "v1" || len(page.Vouchers) != 2 || page.Vouchers[0].UsagesLeft != -1 || page.Vouchers[0].Activations != 3 ||
		page.Vouchers[1].Votes != 10 || page.Vouchers[1].UsagesLeft != 0 {
		t.Fatalf("ListVouchers = %+v", page)
	}

	if err = votes.DeleteVoucher(ctx, "tok", "v1"); err != nil || api.deleted != "v1" {
		t.Fatalf("DeleteVoucher: %v, deleted %q", err, api.deleted)
	}

	// A refused transfer comes with the words of the user service, a
	// refusal to a stranger to the vouchers does not.
	api.err = status.Error(codes.PermissionDenied, "you cannot transfer votes to this user")
	_, err = votes.Transfer(ctx, "tok", "u2", 1)
	if f := failure.From(err); f.Kind != failure.PermissionDenied || f.Message != "you cannot transfer votes to this user" {
		t.Fatalf("a refused transfer: %v", err)
	}

	api.err = status.Error(codes.PermissionDenied, "only administrators can do this")
	_, err = votes.ListVouchers(ctx, "tok", "", 0)
	if f := failure.From(err); f.Kind != failure.PermissionDenied || f.Message != "permission denied" {
		t.Fatalf("vouchers of a stranger: %v", err)
	}

	api.err = status.Error(codes.FailedPrecondition, "voucher is not valid")
	_, err = votes.Redeem(ctx, "tok", "nope")
	if f := failure.From(err); f.Kind != failure.Conflict || f.Message != "voucher is not valid" {
		t.Fatalf("a wrong voucher: %v", err)
	}

	api.err = status.Error(codes.FailedPrecondition, "not enough votes")
	_, err = votes.Transfer(ctx, "tok", "u2", 1)
	if f := failure.From(err); f.Kind != failure.Conflict || f.Message != "not enough votes" {
		t.Fatalf("a transfer of too much: %v", err)
	}
}

func TestVotes_Rate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeVotesAPI{}
	votes := NewVotes(api)

	if kopecks, err := votes.Rate(ctx); err != nil || kopecks != 700 {
		t.Fatalf("Rate = %d, %v", kopecks, err)
	}
	if err := votes.SetRate(ctx, "tok", 1250); err != nil || api.rate != 1250 || api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("SetRate: %v; asked %d", err, api.rate)
	}
}
