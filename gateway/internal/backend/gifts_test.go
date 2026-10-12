package backend

import (
	"context"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeGiftAPI struct {
	userv1.UserServiceClient
	md      metadata.MD
	sent    *userv1.SendGiftRequest
	listed  *userv1.ListGiftsRequest
	deleted string
	err     error
}

var giftSentAt = time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)

func (f *fakeGiftAPI) GetGiftCatalog(context.Context, *userv1.GetGiftCatalogRequest, ...grpc.CallOption) (*userv1.GetGiftCatalogResponse, error) {
	return &userv1.GetGiftCatalogResponse{Collections: []*userv1.GiftCollection{{Id: "flowers", Gifts: []string{"rose", "tulip"}}}}, f.err
}

func (f *fakeGiftAPI) SendGift(ctx context.Context, req *userv1.SendGiftRequest, _ ...grpc.CallOption) (*userv1.SendGiftResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.sent = req

	return &userv1.SendGiftResponse{Gift: &userv1.Gift{
		Id: "g1", Gift: req.GetGift(), Caption: req.GetCaption(), Anonymous: req.GetAnonymous(), SentAt: timestamppb.New(giftSentAt),
		Sender: &userv1.PublicProfile{Id: "u1", Nickname: "ann"},
	}}, f.err
}

func (f *fakeGiftAPI) ListGifts(ctx context.Context, req *userv1.ListGiftsRequest, _ ...grpc.CallOption) (*userv1.ListGiftsResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.listed = req

	return &userv1.ListGiftsResponse{
		Gifts: []*userv1.Gift{
			{Id: "g2", Gift: "cake", SentAt: timestamppb.New(giftSentAt)},
			{Id: "g1", Gift: "rose", Caption: "Привет", SentAt: timestamppb.New(giftSentAt), Sender: &userv1.PublicProfile{Id: "u2", Nickname: "bob"}},
		},
		NextCursor: "g1", Total: 7,
	}, f.err
}

func (f *fakeGiftAPI) DeleteGift(_ context.Context, req *userv1.DeleteGiftRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.deleted = req.GetGiftId()
	return &emptypb.Empty{}, f.err
}

func TestGifts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeGiftAPI{}
	gifts := NewGifts(api)

	catalog, err := gifts.Catalog(ctx)
	if err != nil || len(catalog) != 1 || catalog[0].ID != "flowers" || len(catalog[0].Gifts) != 2 {
		t.Fatalf("Catalog = %+v, %v", catalog, err)
	}

	sent, err := gifts.Send(ctx, "tok", "u2", model.GiftDraft{Gift: "rose", Caption: "Привет", Anonymous: true})
	if err != nil {
		t.Fatal(err)
	}
	if api.sent.GetUserId() != "u2" || api.sent.GetGift() != "rose" || api.sent.GetCaption() != "Привет" || !api.sent.GetAnonymous() ||
		api.md.Get("authorization")[0] != "Bearer tok" {
		t.Fatalf("sent %v", api.sent)
	}
	if sent.ID != "g1" || sent.Sender == nil || sent.Sender.Profile.Nickname != "ann" || !sent.SentAt.Equal(giftSentAt) || !sent.Anonymous {
		t.Fatalf("Send = %+v", sent)
	}

	// Without a token the list is asked anonymously.
	page, err := gifts.List(ctx, "", "u1", "g5", 3)
	if err != nil || api.listed.GetUserId() != "u1" || api.listed.GetCursor() != "g5" || api.listed.GetLimit() != 3 || len(api.md.Get("authorization")) != 0 {
		t.Fatalf("listed %v, %v", api.listed, err)
	}
	if page.Total != 7 || page.NextCursor != "g1" || len(page.Gifts) != 2 || page.Gifts[0].Sender != nil || page.Gifts[1].Sender.ID != "u2" ||
		page.Gifts[1].Caption != "Привет" {
		t.Fatalf("List = %+v", page)
	}

	if err = gifts.Delete(ctx, "tok", "g1"); err != nil || api.deleted != "g1" {
		t.Fatalf("Delete: %v, deleted %q", err, api.deleted)
	}

	// A refusal comes with the words of the user service.
	api.err = status.Error(codes.PermissionDenied, "you cannot send a gift to this user")
	_, err = gifts.Send(ctx, "tok", "u2", model.GiftDraft{Gift: "rose"})
	if f := failure.From(err); f.Kind != failure.PermissionDenied || f.Message != "you cannot send a gift to this user" {
		t.Fatalf("a refused gift: %v", err)
	}

	api.err = status.Error(codes.NotFound, "gift not found")
	if err = gifts.Delete(ctx, "tok", "g1"); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("no such gift: %v", err)
	}
}
