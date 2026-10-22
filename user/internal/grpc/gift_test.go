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

func TestGifts(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")
	cat, _ := s.member(t, "cat")
	anonymous := context.Background()

	// The catalogue is public.
	catalog, err := s.client.GetGiftCatalog(anonymous, &userv1.GetGiftCatalogRequest{})
	if err != nil || len(catalog.GetCollections()) == 0 || len(catalog.GetCollections()[0].GetGifts()) == 0 {
		t.Fatalf("GetGiftCatalog = %v, %v", catalog, err)
	}
	kind := catalog.GetCollections()[0].GetGifts()[0]

	// Every gift costs votes; the administrators make this one free.
	if _, err = s.client.SetGiftPrice(s.signIn(t, adminEmail), &userv1.SetGiftPriceRequest{Gift: kind, Price: 0}); err != nil {
		t.Fatalf("SetGiftPrice: %v", err)
	}

	sent, err := s.client.SendGift(bob, &userv1.SendGiftRequest{UserId: annID, Gift: kind, Caption: "  С праздником!  "})
	if err != nil {
		t.Fatalf("SendGift: %v", err)
	}
	if g := sent.GetGift(); g.GetGift() != kind || g.GetCaption() != "С праздником!" || g.GetSender().GetId() != bobID || g.GetSentAt() == nil {
		t.Fatalf("sent = %v", g)
	}

	if _, err = s.client.SendGift(cat, &userv1.SendGiftRequest{UserId: annID, Gift: kind, Caption: "Тайно", Anonymous: true}); err != nil {
		t.Fatalf("SendGift anonymously: %v", err)
	}

	// Public: an anonymous gift tells strangers neither who nor what.
	for name, viewer := range map[string]context.Context{"anonymous": anonymous, "bob": bob} {
		list, err := s.client.ListGifts(viewer, &userv1.ListGiftsRequest{UserId: annID})
		if err != nil || list.GetTotal() != 2 || len(list.GetGifts()) != 2 || list.GetNextCursor() != "" {
			t.Fatalf("%s: ListGifts = %v, %v", name, list, err)
		}
		if secret := list.GetGifts()[0]; secret.GetSender() != nil || secret.GetCaption() != "" || secret.GetAnonymous() || secret.GetGift() != kind {
			t.Fatalf("%s: the anonymous gift = %v", name, secret)
		}
		if list.GetGifts()[1].GetSender().GetNickname() != "bob" {
			t.Fatalf("%s: the signed gift = %v", name, list.GetGifts()[1])
		}
	}

	own, err := s.client.ListGifts(ann, &userv1.ListGiftsRequest{UserId: annID, Limit: 1})
	if err != nil || len(own.GetGifts()) != 1 || own.GetNextCursor() == "" {
		t.Fatalf("the first page = %v, %v", own, err)
	}
	if secret := own.GetGifts()[0]; secret.GetSender().GetNickname() != "cat" || secret.GetCaption() != "Тайно" || !secret.GetAnonymous() {
		t.Fatalf("the owner's anonymous gift = %v", secret)
	}
	next, err := s.client.ListGifts(ann, &userv1.ListGiftsRequest{UserId: annID, Limit: 1, Cursor: own.GetNextCursor()})
	if err != nil || len(next.GetGifts()) != 1 || next.GetGifts()[0].GetId() != sent.GetGift().GetId() || next.GetNextCursor() != "" {
		t.Fatalf("the second page = %v, %v", next, err)
	}

	// Hidden gifts: GetAccess says so and the list is refused.
	if _, err = s.client.UpdatePrivacy(ann, &userv1.UpdatePrivacyRequest{Privacy: &userv1.PrivacySettings{
		WallWrite: userv1.WallWrite_WALL_WRITE_FOLLOWING, MessagesWrite: userv1.MessagesWrite_MESSAGES_WRITE_EVERYONE,
		ClubsRead: userv1.ClubsRead_CLUBS_READ_EVERYONE, PageRead: userv1.Audience_AUDIENCE_EVERYONE,
		InfoRead: userv1.Audience_AUDIENCE_EVERYONE, PhotosRead: userv1.Audience_AUDIENCE_EVERYONE,
		FriendsRead: userv1.Audience_AUDIENCE_EVERYONE, OnlineRead: userv1.Audience_AUDIENCE_EVERYONE,
		GiftsRead: userv1.Audience_AUDIENCE_ONLY_ME,
	}}); err != nil {
		t.Fatalf("UpdatePrivacy: %v", err)
	}

	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil || me.GetUser().GetPrivacy().GetGiftsRead() != userv1.Audience_AUDIENCE_ONLY_ME {
		t.Fatalf("privacy = %v, %v", me.GetUser().GetPrivacy(), err)
	}

	access, err := s.client.GetAccess(bob, &userv1.GetAccessRequest{UserId: annID})
	if err != nil || !access.GetPage() || access.GetGifts() {
		t.Fatalf("bob's access = %v, %v", access, err)
	}
	if access, err = s.client.GetAccess(ann, &userv1.GetAccessRequest{UserId: annID}); err != nil || !access.GetGifts() {
		t.Fatalf("ann's access = %v, %v", access, err)
	}

	_, err = s.client.ListGifts(bob, &userv1.ListGiftsRequest{UserId: annID})
	if st := status.Convert(err); st.Code() != codes.PermissionDenied || st.Message() != "this user's gifts are hidden" {
		t.Fatalf("hidden gifts = %v", err)
	}

	// Only the owner deletes.
	_, err = s.client.DeleteGift(bob, &userv1.DeleteGiftRequest{GiftId: sent.GetGift().GetId()})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("the sender deletes: %v", err)
	}
	if _, err = s.client.DeleteGift(ann, &userv1.DeleteGiftRequest{GiftId: sent.GetGift().GetId()}); err != nil {
		t.Fatalf("DeleteGift: %v", err)
	}
	_, err = s.client.DeleteGift(ann, &userv1.DeleteGiftRequest{GiftId: sent.GetGift().GetId()})
	if st := status.Convert(err); st.Code() != codes.NotFound || st.Message() != "gift not found" {
		t.Fatalf("deleting twice = %v", err)
	}
	if left, err := s.client.ListGifts(ann, &userv1.ListGiftsRequest{UserId: annID}); err != nil || left.GetTotal() != 1 {
		t.Fatalf("after DeleteGift = %v, %v", left, err)
	}
}

func TestGifts_Errors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")

	if _, err := s.client.Block(ann, &userv1.BlockRequest{UserId: bobID}); err != nil {
		t.Fatal(err)
	}

	for name, tt := range map[string]struct {
		call func() error
		want codes.Code
	}{
		"an unknown gift": {func() error {
			_, err := s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: annID, Gift: "yacht"})
			return err
		}, codes.InvalidArgument},
		"a long caption": {func() error {
			_, err := s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: annID, Gift: "freenet_coin", Caption: strings.Repeat("я", 251)})
			return err
		}, codes.InvalidArgument},
		"not an id": {func() error {
			_, err := s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: "ann", Gift: "freenet_coin"})
			return err
		}, codes.InvalidArgument},
		"to nobody": {func() error {
			_, err := s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: uuid.NewString(), Gift: "freenet_coin"})
			return err
		}, codes.NotFound},
		"across a blacklist": {func() error {
			_, err := s.client.SendGift(bob, &userv1.SendGiftRequest{UserId: annID, Gift: "freenet_coin"})
			return err
		}, codes.PermissionDenied},
		"without a token": {func() error {
			_, err := s.client.SendGift(context.Background(), &userv1.SendGiftRequest{UserId: annID, Gift: "freenet_coin"})
			return err
		}, codes.Unauthenticated},
		"deleting without a token": {func() error {
			_, err := s.client.DeleteGift(context.Background(), &userv1.DeleteGiftRequest{GiftId: uuid.NewString()})
			return err
		}, codes.Unauthenticated},
		"the gifts of nobody": {func() error {
			_, err := s.client.ListGifts(ann, &userv1.ListGiftsRequest{UserId: uuid.NewString()})
			return err
		}, codes.NotFound},
		"a broken cursor": {func() error {
			_, err := s.client.ListGifts(ann, &userv1.ListGiftsRequest{UserId: annID, Cursor: "x"})
			return err
		}, codes.InvalidArgument},
	} {
		if got := status.Code(tt.call()); got != tt.want {
			t.Errorf("%s: code = %v, want %v", name, got, tt.want)
		}
	}
}
