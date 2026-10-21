package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRocket(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")

	// Anyone sees the limits and the top, not the return.
	got, err := s.client.GetCasino(context.Background(), &userv1.GetCasinoRequest{})
	if err != nil || got.GetRocket().GetMost() != 100 || got.GetRocket().GetTop() != 1000 || got.GetRocket().GetReturn() != 0 {
		t.Fatalf("GetCasino = %v, %v", got, err)
	}

	// A rocket that explodes at the start but once in 10000.
	tuned := &userv1.SetCasinoRocketRequest{Rocket: &userv1.CasinoRocket{Least: 1, Most: 100, Return: 1, Top: 1000}}
	if _, err := s.client.SetCasinoRocket(ann, tuned); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetCasinoRocket by ann: %v", err)
	}
	if _, err := s.client.SetCasinoRocket(admin, &userv1.SetCasinoRocketRequest{Rocket: &userv1.CasinoRocket{Least: 1, Most: 100, Return: 1, Top: 100}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasinoRocket with a top of ×1: %v", err)
	}
	if _, err := s.client.SetCasinoRocket(admin, tuned); err != nil {
		t.Fatalf("SetCasinoRocket: %v", err)
	}

	settings, err := s.client.GetCasinoSettings(admin, &userv1.GetCasinoSettingsRequest{})
	if err != nil || settings.GetRocket().GetReturn() != 1 || settings.GetRocket().GetTop() != 1000 {
		t.Fatalf("GetCasinoSettings = %v, %v", settings, err)
	}

	if _, err := s.client.LaunchRocket(ann, &userv1.LaunchRocketRequest{Amount: 10}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("LaunchRocket without votes: %v", err)
	}
	if _, err := s.client.LaunchRocket(ann, &userv1.LaunchRocketRequest{Amount: 101}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("LaunchRocket above the most: %v", err)
	}
	if _, err := s.client.LaunchRocket(ann, &userv1.LaunchRocketRequest{Amount: 10, Auto: 100}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("LaunchRocket cashing at ×1: %v", err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 50, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	round, err := s.client.LaunchRocket(ann, &userv1.LaunchRocketRequest{Amount: 10})
	if err != nil || round.GetBalance() != 40 || round.GetState() != userv1.RocketState_ROCKET_STATE_CRASHED || round.GetCrash() != 100 {
		t.Fatalf("LaunchRocket = %v, %v", round, err)
	}

	id := round.GetId()
	if _, err := s.client.GetRocket(bob, &userv1.RocketRequest{RocketId: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetRocket by bob: %v", err)
	}
	if _, err := s.client.GetRocket(ann, &userv1.RocketRequest{RocketId: "nope"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("GetRocket of no id: %v", err)
	}
	if _, err := s.client.CashRocket(ann, &userv1.RocketRequest{RocketId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Fatalf("CashRocket of nothing: %v", err)
	}

	cashed, err := s.client.CashRocket(ann, &userv1.RocketRequest{RocketId: id})
	if err != nil || cashed.GetState() != userv1.RocketState_ROCKET_STATE_CRASHED || cashed.GetPayout() != 0 || cashed.GetBalance() != 40 {
		t.Fatalf("CashRocket = %v, %v", cashed, err)
	}

	auto, err := s.client.LaunchRocket(ann, &userv1.LaunchRocketRequest{Amount: 10, Auto: 200})
	if err != nil || auto.GetState() != userv1.RocketState_ROCKET_STATE_CRASHED || auto.GetAuto() != 200 || auto.GetBalance() != 30 {
		t.Fatalf("LaunchRocket by itself = %v, %v", auto, err)
	}
	if _, err := s.client.CashRocket(ann, &userv1.RocketRequest{RocketId: auto.GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CashRocket of auto: %v", err)
	}
}
