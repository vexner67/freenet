package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPet(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")

	if _, err := s.client.GetPet(context.Background(), &userv1.GetPetRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetPet by nobody: %v", err)
	}

	state, err := s.client.GetPet(ann, &userv1.GetPetRequest{})
	if err != nil || len(state.GetPets()) != 0 || state.GetShop().GetPrices()["dragon"] != 500 {
		t.Fatalf("GetPet without a pet = %v, %v", state, err)
	}

	if _, err = s.client.AdoptPet(ann, &userv1.AdoptPetRequest{Kind: "cat", Name: "Мурка"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("AdoptPet without votes: %v", err)
	}
	if _, err = s.client.AdoptPet(ann, &userv1.AdoptPetRequest{Kind: "unicorn", Name: "Мурка"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AdoptPet of a unicorn: %v", err)
	}

	for _, who := range []context.Context{ann, bob} {
		created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 200, Usages: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.client.RedeemVoucher(who, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
			t.Fatal(err)
		}
	}

	adopted, err := s.client.AdoptPet(ann, &userv1.AdoptPetRequest{Kind: "cat", Name: "Мурка"})
	if err != nil || adopted.GetBalance() != 100 || adopted.GetPet().GetSatiety() != 80 || adopted.GetPet().GetLevel() != 1 {
		t.Fatalf("AdoptPet = %v, %v", adopted, err)
	}
	cat := adopted.GetPet().GetId()
	if _, err = s.client.FightPet(bob, &userv1.FightPetRequest{PetId: cat}); status.Code(err) != codes.NotFound {
		t.Fatalf("FightPet with the pet of another: %v", err)
	}
	if _, err = s.client.FeedPet(ann, &userv1.FeedPetRequest{PetId: "bad"}); status.Code(err) != codes.NotFound {
		t.Fatalf("FeedPet of a bad pet: %v", err)
	}
	hamster, err := s.client.AdoptPet(bob, &userv1.AdoptPetRequest{Kind: "hamster", Name: "Хома"})
	if err != nil {
		t.Fatal(err)
	}

	played, err := s.client.PlayPet(ann, &userv1.PlayPetRequest{PetId: cat})
	if err != nil || played.GetPet().GetPlayAt() == nil {
		t.Fatalf("PlayPet = %v, %v", played, err)
	}
	if _, err = s.client.PlayPet(ann, &userv1.PlayPetRequest{PetId: cat}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("PlayPet again: %v", err)
	}

	fought, err := s.client.FightPet(ann, &userv1.FightPetRequest{Nickname: "bob", PetId: cat})
	if err != nil || len(fought.GetFight().GetHits()) == 0 || fought.GetPet().GetFights() != 1 {
		t.Fatalf("FightPet = %v, %v", fought, err)
	}
	if fights, err := s.client.ListPetFights(bob, &userv1.ListPetFightsRequest{}); err != nil || len(fights.GetFights()) != 1 {
		t.Fatalf("ListPetFights = %v, %v", fights, err)
	}

	theirs, err := s.client.GetUserPet(context.Background(), &userv1.GetUserPetRequest{Nickname: "bob"})
	if err != nil || len(theirs.GetPets()) != 1 || theirs.GetPets()[0].GetName() != "Хома" {
		t.Fatalf("GetUserPet = %v, %v", theirs, err)
	}
	if _, err = s.client.GetUserPet(context.Background(), &userv1.GetUserPetRequest{Nickname: "nobody"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetUserPet of nobody: %v", err)
	}

	board, err := s.client.ListTop(context.Background(), &userv1.ListTopRequest{Board: "pets"})
	if err != nil || len(board.GetEntries()) != 2 || board.GetEntries()[0].GetPetName() == "" {
		t.Fatalf("ListTop of pets = %v, %v", board, err)
	}
	if _, err = s.client.ListTop(context.Background(), &userv1.ListTopRequest{Board: "poets"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListTop of poets: %v", err)
	}

	if _, err = s.client.GetPetSettings(ann, &userv1.GetPetSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetPetSettings by ann: %v", err)
	}
	settings, err := s.client.GetPetSettings(admin, &userv1.GetPetSettingsRequest{})
	if err != nil || settings.GetPets() != 2 || settings.GetFights() != 1 {
		t.Fatalf("GetPetSettings = %v, %v", settings, err)
	}
	shop := settings.GetShop()
	shop.Fights = 0
	if _, err = s.client.SetPetSettings(admin, &userv1.SetPetSettingsRequest{Shop: shop}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetPetSettings without fights: %v", err)
	}

	if _, err = s.client.ReleasePet(bob, &userv1.ReleasePetRequest{PetId: hamster.GetPet().GetId()}); err != nil {
		t.Fatal(err)
	}
	if mine, err := s.client.GetPet(ann, &userv1.GetPetRequest{}); err != nil || len(mine.GetPets()) != 1 || mine.GetPets()[0].GetId() != cat {
		t.Fatalf("GetPet = %v, %v", mine, err)
	}
}

func TestGuests(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")
	bob, _ := s.member(t, "bob")

	if _, err := s.client.VisitPage(bob, &userv1.VisitPageRequest{Nickname: "nobody"}); status.Code(err) != codes.NotFound {
		t.Fatalf("VisitPage of nobody: %v", err)
	}
	if _, err := s.client.VisitPage(bob, &userv1.VisitPageRequest{Nickname: "ann"}); err != nil {
		t.Fatal(err)
	}

	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil || me.GetNewGuests() != 1 {
		t.Fatalf("GetMe = %v, %v", me, err)
	}

	guests, err := s.client.ListGuests(ann, &userv1.ListGuestsRequest{})
	if err != nil || len(guests.GetGuests()) != 1 || guests.GetSeenAt() != nil {
		t.Fatalf("ListGuests = %v, %v", guests, err)
	}
	if me, _ = s.client.GetMe(ann, &userv1.GetMeRequest{}); me.GetNewGuests() != 0 {
		t.Fatalf("new guests after a look: %d", me.GetNewGuests())
	}
	if _, err = s.client.ListGuests(ann, &userv1.ListGuestsRequest{Cursor: "bad"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListGuests with a bad cursor: %v", err)
	}
}
