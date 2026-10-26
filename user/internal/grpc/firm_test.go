package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFirms(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	if _, err := s.client.GetFirms(context.Background(), &userv1.GetFirmsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetFirms by nobody: %v", err)
	}

	got, err := s.client.GetFirms(ann, &userv1.GetFirmsRequest{})
	if err != nil || len(got.GetSettings().GetPlans()) != len(user.FirmKinds) || len(got.GetFirms()) != 0 ||
		len(got.GetCatalog()) != len(user.FirmKinds) || len(got.GetQuotes()) != len(user.Goods) {
		t.Fatalf("GetFirms = %v, %v", got, err)
	}

	for _, tc := range []struct {
		name string
		req  *userv1.RunFirmsRequest
		code codes.Code
	}{
		{"unknown action", &userv1.RunFirmsRequest{Action: "steal"}, codes.InvalidArgument},
		{"unknown kind", &userv1.RunFirmsRequest{Action: "found", Kind: "bank", Name: "Банк"}, codes.InvalidArgument},
		{"no name", &userv1.RunFirmsRequest{Action: "found", Kind: "farm"}, codes.InvalidArgument},
		{"no votes", &userv1.RunFirmsRequest{Action: "found", Kind: "farm", Name: "Ферма"}, codes.FailedPrecondition},
		{"bad id", &userv1.RunFirmsRequest{Action: "upgrade", FirmId: "x"}, codes.InvalidArgument},
		{"none", &userv1.RunFirmsRequest{Action: "upgrade", FirmId: "0192f0a0-0000-7000-8000-000000000000"}, codes.NotFound},
		{"unknown good", &userv1.RunFirmsRequest{Action: "order", Good: "gold"}, codes.InvalidArgument},
		{"new page", &userv1.RunFirmsRequest{Action: "dividend", Votes: 1}, codes.FailedPrecondition},
	} {
		if _, err = s.client.RunFirms(ann, tc.req); status.Code(err) != tc.code {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.code)
		}
	}

	if _, err = s.client.GetFirmSettings(ann, &userv1.GetFirmSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetFirmSettings by ann: %v", err)
	}

	settings, err := s.client.GetFirmSettings(admin, &userv1.GetFirmSettingsRequest{})
	if err != nil || settings.GetSettings().GetFee() != 2 {
		t.Fatalf("GetFirmSettings = %v, %v", settings, err)
	}

	tuned := settings.GetSettings()
	tuned.Fee = 3
	tuned.Plans[0].Price = 1
	if _, err = s.client.SetFirmSettings(admin, &userv1.SetFirmSettingsRequest{Settings: tuned}); err != nil {
		t.Fatal(err)
	}
	tuned.Fee = 80
	if _, err = s.client.SetFirmSettings(admin, &userv1.SetFirmSettingsRequest{Settings: tuned}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetFirmSettings with a fee of 80: %v", err)
	}

	got, err = s.client.GetFirms(ann, &userv1.GetFirmsRequest{})
	if err != nil || got.GetSettings().GetFee() != 3 || got.GetSettings().GetPlans()[0].GetPrice() != 1 {
		t.Fatalf("GetFirms after the settings = %v, %v", got, err)
	}

	// The enterprises of a page are public.
	shown, err := s.client.GetUserFirms(context.Background(), &userv1.GetUserFirmsRequest{Nickname: "ann"})
	if err != nil || len(shown.GetFirms()) != 0 {
		t.Fatalf("GetUserFirms = %v, %v", shown, err)
	}
	if _, err = s.client.GetUserFirms(context.Background(), &userv1.GetUserFirmsRequest{Nickname: "nobody"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetUserFirms of nobody: %v", err)
	}
}
