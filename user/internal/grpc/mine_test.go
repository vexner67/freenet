package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMine(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	if _, err := s.client.GetMine(context.Background(), &userv1.GetMineRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetMine by nobody: %v", err)
	}

	got, err := s.client.GetMine(ann, &userv1.GetMineRequest{})
	def := user.DefaultMine()
	if err != nil || got.GetMiner().GetEnergy() != int32(def.Energy()) || len(got.GetSettings().GetVeins()) != len(user.Ores) {
		t.Fatalf("GetMine = %v, %v", got, err)
	}

	dug, err := s.client.DigOre(ann, &userv1.DigOreRequest{Ore: "stone"})
	if err != nil || dug.GetMine().GetMiner().GetDigs() != 1 {
		t.Fatalf("DigOre = %v, %v", dug, err)
	}
	if _, err = s.client.DigOre(ann, &userv1.DigOreRequest{Ore: "tin"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("DigOre of tin: %v", err)
	}
	if _, err = s.client.DigOre(ann, &userv1.DigOreRequest{Ore: "crystal"}); status.Code(err) != codes.FailedPrecondition ||
		status.Convert(err).Message() != user.ErrOreLocked.Error() {
		t.Fatalf("DigOre of a crystal: %v", err)
	}

	// A page made today sells nothing yet.
	if _, err = s.client.SellOre(ann, &userv1.SellOreRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SellOre of a new page: %v", err)
	}

	if _, err = s.client.GetMineSettings(ann, &userv1.GetMineSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetMineSettings by ann: %v", err)
	}

	settings, err := s.client.GetMineSettings(admin, &userv1.GetMineSettingsRequest{})
	if err != nil || settings.GetDigs() != 1 || settings.GetMiners() != 1 {
		t.Fatalf("GetMineSettings = %v, %v", settings, err)
	}

	tuned := settings.GetSettings()
	tuned.Energy = 40
	if _, err = s.client.SetMine(admin, &userv1.SetMineRequest{Settings: tuned}); err != nil {
		t.Fatal(err)
	}
	tuned.Veins = tuned.GetVeins()[1:]
	if _, err = s.client.SetMine(admin, &userv1.SetMineRequest{Settings: tuned}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetMine without stone: %v", err)
	}
	if got, err = s.client.GetMine(ann, &userv1.GetMineRequest{}); err != nil || got.GetSettings().GetEnergy() != 40 {
		t.Fatalf("GetMine after SetMine = %v, %v", got, err)
	}
}
