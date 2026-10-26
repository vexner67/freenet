package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBusiness(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	if _, err := s.client.GetBusinesses(context.Background(), &userv1.GetBusinessesRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetBusinesses by nobody: %v", err)
	}

	got, err := s.client.GetBusinesses(ann, &userv1.GetBusinessesRequest{})
	if err != nil || len(got.GetSettings().GetPlans()) != len(user.BusinessKinds) || len(got.GetBusinesses()) != 0 ||
		len(got.GetCatalog()) != len(user.BusinessKinds) || len(got.GetSuppliers()) != 3 {
		t.Fatalf("GetBusinesses = %v, %v", got, err)
	}

	if _, err = s.client.BuyBusiness(ann, &userv1.BuyBusinessRequest{Kind: "bank"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("BuyBusiness of a bank: %v", err)
	}
	if _, err = s.client.BuyBusiness(ann, &userv1.BuyBusinessRequest{Kind: "kiosk"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("BuyBusiness without votes: %v", err)
	}
	if _, err = s.client.RunBusiness(ann, &userv1.RunBusinessRequest{Kind: "kiosk", Action: "repair"}); status.Code(err) != codes.NotFound {
		t.Fatalf("RunBusiness of a kiosk not owned: %v", err)
	}
	if _, err = s.client.RunBusiness(ann, &userv1.RunBusinessRequest{Kind: "kiosk", Action: "steal"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("RunBusiness of an unknown action: %v", err)
	}
	_, err = s.client.RunBusiness(ann, &userv1.RunBusinessRequest{Kind: "kiosk", Action: "collect"})
	if st := status.Convert(err); st.Code() != codes.FailedPrecondition || st.Message() != user.ErrBusinessNotYet.Error() {
		t.Fatalf("RunBusiness collect of a new page: %v", err)
	}

	if _, err = s.client.GetBusinessSettings(ann, &userv1.GetBusinessSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetBusinessSettings by ann: %v", err)
	}

	settings, err := s.client.GetBusinessSettings(admin, &userv1.GetBusinessSettingsRequest{})
	if err != nil || settings.GetSettings().GetTax() != 5 {
		t.Fatalf("GetBusinessSettings = %v, %v", settings, err)
	}

	tuned := settings.GetSettings()
	tuned.Tax = 8
	tuned.Plans[0].Price = 1
	if _, err = s.client.SetBusinessSettings(admin, &userv1.SetBusinessSettingsRequest{Settings: tuned}); err != nil {
		t.Fatal(err)
	}
	tuned.Tax = 200
	if _, err = s.client.SetBusinessSettings(admin, &userv1.SetBusinessSettingsRequest{Settings: tuned}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetBusinessSettings with a tax of 200: %v", err)
	}

	got, err = s.client.GetBusinesses(ann, &userv1.GetBusinessesRequest{})
	if err != nil || got.GetSettings().GetTax() != 8 || got.GetSettings().GetPlans()[0].GetPrice() != 1 {
		t.Fatalf("GetBusinesses after the settings = %v, %v", got, err)
	}

	// The businesses of a page are public.
	shown, err := s.client.GetUserBusinesses(context.Background(), &userv1.GetUserBusinessesRequest{Nickname: "ann"})
	if err != nil || len(shown.GetBusinesses()) != 0 {
		t.Fatalf("GetUserBusinesses = %v, %v", shown, err)
	}
	if _, err = s.client.GetUserBusinesses(context.Background(), &userv1.GetUserBusinessesRequest{Nickname: "nobody"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetUserBusinesses of nobody: %v", err)
	}
}
