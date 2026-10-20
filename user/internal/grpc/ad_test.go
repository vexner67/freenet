package grpc_test

import (
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestAds(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	cover := uuid.NewString()

	if _, err := s.client.CreateAd(ann, &userv1.CreateAdRequest{Url: "a.example", Title: "A"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("CreateAd by ann: %v", err)
	}
	for _, bad := range []*userv1.CreateAdRequest{
		{Url: "ftp://a.example", Title: "A"},
		{Url: "a.example", Title: " "},
		{Url: "a.example", Title: "A", PhotoId: "x"},
	} {
		if _, err := s.client.CreateAd(admin, bad); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("CreateAd(%v): %v", bad, err)
		}
	}

	created, err := s.client.CreateAd(admin, &userv1.CreateAdRequest{Url: "a.example", Title: "A", Text: "Скидки", PhotoId: cover, Active: true})
	if err != nil || created.GetAd().GetUrl() != "https://a.example" || created.GetAd().GetPhotoId() != cover || created.GetAd().GetPhotoOwnerId() == "" {
		t.Fatalf("CreateAd = %v, %v", created, err)
	}
	id := created.GetAd().GetId()

	if _, err = s.client.CreateAd(admin, &userv1.CreateAdRequest{Url: "b.example", Title: "B"}); err != nil {
		t.Fatal(err)
	}

	// Anyone reads the shown ads, even without a token.
	shown, err := s.client.ListAds(t.Context(), &userv1.ListAdsRequest{})
	if err != nil || len(shown.GetAds()) != 1 || shown.GetAds()[0].GetId() != id {
		t.Fatalf("ListAds = %v, %v", shown, err)
	}
	if _, err = s.client.ListAds(ann, &userv1.ListAdsRequest{All: true}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ListAds all by ann: %v", err)
	}
	if all, err := s.client.ListAds(admin, &userv1.ListAdsRequest{All: true}); err != nil || len(all.GetAds()) != 2 {
		t.Fatalf("ListAds all = %v, %v", all, err)
	}

	// Hiding the ad and taking its cover.
	edited, err := s.client.EditAd(admin, &userv1.EditAdRequest{Id: id, Active: proto.Bool(false), PhotoId: proto.String("")})
	if err != nil || edited.GetAd().GetActive() || edited.GetAd().GetPhotoId() != "" || edited.GetDroppedPhotoId() != cover || edited.GetAd().GetTitle() != "A" {
		t.Fatalf("EditAd = %v, %v", edited, err)
	}
	if _, err = s.client.EditAd(admin, &userv1.EditAdRequest{Id: uuid.NewString(), Title: proto.String("C")}); status.Code(err) != codes.NotFound {
		t.Fatalf("an unknown ad: %v", err)
	}
	if _, err = s.client.EditAd(admin, &userv1.EditAdRequest{Id: id, Url: proto.String("")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an empty link: %v", err)
	}
	if shown, err = s.client.ListAds(ann, &userv1.ListAdsRequest{}); err != nil || len(shown.GetAds()) != 0 {
		t.Fatalf("ListAds after hiding = %v, %v", shown, err)
	}

	if _, err = s.client.DeleteAd(ann, &userv1.DeleteAdRequest{Id: id}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DeleteAd by ann: %v", err)
	}
	if deleted, err := s.client.DeleteAd(admin, &userv1.DeleteAdRequest{Id: id}); err != nil || deleted.GetPhotoId() != "" {
		t.Fatalf("DeleteAd = %v, %v", deleted, err)
	}
	if _, err = s.client.DeleteAd(admin, &userv1.DeleteAdRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("deleted twice: %v", err)
	}
}
