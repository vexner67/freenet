package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
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

	if _, err = s.client.FightPet(ann, &userv1.FightPetRequest{Nickname: "bob", PetId: cat, Stance: "rage"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FightPet in an unknown stance: %v", err)
	}
	fought, err := s.client.FightPet(ann, &userv1.FightPetRequest{Nickname: "bob", PetId: cat, Stance: "agility"})
	if err != nil || len(fought.GetFight().GetHits()) == 0 || fought.GetPet().GetFights() != 1 || fought.GetFight().GetAttacker().GetStance() != "agility" ||
		fought.GetFight().GetRatio() == 0 || fought.GetPet().GetFatigue() != 1 {
		t.Fatalf("FightPet = %v, %v", fought, err)
	}

	// Ann leads a fight a round after another; it shows in her state.
	stood, err := s.client.SetPetStance(bob, &userv1.SetPetStanceRequest{PetId: hamster.GetPet().GetId(), Stance: "attack"})
	if err != nil || stood.GetPet().GetStance() != "attack" {
		t.Fatalf("SetPetStance = %v, %v", stood, err)
	}
	if _, err = s.client.FeedPet(ann, &userv1.FeedPetRequest{PetId: cat}); err != nil {
		t.Fatal(err)
	}
	started, err := s.client.StartPetBattle(ann, &userv1.StartPetBattleRequest{PetId: cat, Nickname: "bob", Stance: "defense"})
	if err != nil || started.GetBattle().GetRounds() == 0 || started.GetBattle().GetDefender().GetStance() != "attack" || started.GetFight() != nil {
		t.Fatalf("StartPetBattle = %v, %v", started, err)
	}
	if mine, err := s.client.GetPet(ann, &userv1.GetPetRequest{}); err != nil || len(mine.GetBattles()) != 1 || mine.GetKeeper().GetPityLimit() == 0 {
		t.Fatalf("GetPet in a fight = %v, %v", mine, err)
	}
	if _, err = s.client.MovePetBattle(ann, &userv1.MovePetBattleRequest{PetId: cat, Move: "flee"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an unknown move: %v", err)
	}
	moved, err := s.client.MovePetBattle(ann, &userv1.MovePetBattleRequest{PetId: cat, Move: "auto"})
	if err != nil || moved.GetFight() == nil || moved.GetBattle().GetRound() == 0 {
		t.Fatalf("MovePetBattle = %v, %v", moved, err)
	}
	if _, err = s.client.MovePetBattle(ann, &userv1.MovePetBattleRequest{PetId: cat, Move: "strike"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a move after the end: %v", err)
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
	if err != nil || settings.GetPets() != 2 || settings.GetFights() != 2 {
		t.Fatalf("GetPetSettings = %v, %v", settings, err)
	}
	shop := settings.GetShop()
	shop.Fights = 0
	if _, err = s.client.SetPetSettings(admin, &userv1.SetPetSettingsRequest{Shop: shop}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetPetSettings without fights: %v", err)
	}

	arena, err := s.client.GetPetArena(ann, &userv1.GetPetArenaRequest{})
	if err != nil || arena.GetBoss().GetHealth() != arena.GetBoss().GetMaxHealth() || len(arena.GetQuests()) != 3 || len(arena.GetTop()) != 2 {
		t.Fatalf("GetPetArena = %v, %v", arena, err)
	}
	joined, err := s.client.RunPetArena(bob, &userv1.RunPetArenaRequest{Action: "join", PetId: hamster.GetPet().GetId()})
	if err != nil || len(joined.GetArena().GetTournament().GetEntrants()) != 1 {
		t.Fatalf("RunPetArena join = %v, %v", joined, err)
	}
	if _, err = s.client.RunPetArena(ann, &userv1.RunPetArenaRequest{Action: "bet", PetId: hamster.GetPet().GetId(), Votes: 5}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bet of 5: %v", err)
	}
	if _, err = s.client.RunPetArena(ann, &userv1.RunPetArenaRequest{Action: "sell", PetId: cat, Votes: 100}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("selling a pet just adopted: %v", err)
	}
	if _, err = s.client.RunPetArena(ann, &userv1.RunPetArenaRequest{Action: "dance", PetId: cat}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an unknown action: %v", err)
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

func TestPetCasesAndClothes(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	state, err := s.client.GetPet(ann, &userv1.GetPetRequest{})
	if err != nil || len(state.GetKinds()) != 13 || len(state.GetCases()) != 3 || len(state.GetItems()) != 12 || state.GetShop().GetCases()["gold"] != 1200 {
		t.Fatalf("the catalogue = %v, %v", state, err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 300, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	if _, err = s.client.OpenPetCase(ann, &userv1.OpenPetCaseRequest{CaseId: "diamond"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("OpenPetCase of an unknown case: %v", err)
	}
	if _, err = s.client.OpenPetCase(ann, &userv1.OpenPetCaseRequest{CaseId: "bronze", Shards: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("OpenPetCase without shards: %v", err)
	}
	// A task of the day to open a chest, if Ann has one, pays for it.
	opened, err := s.client.OpenPetCase(ann, &userv1.OpenPetCaseRequest{CaseId: "bronze"})
	quest := opened.GetBalance() - 150
	if err != nil || (quest != 0 && quest != user.PetQuestReward) || opened.GetKeeper().GetDropKind() == "" || opened.GetKeeper().GetPity() != 1 {
		t.Fatalf("OpenPetCase = %v, %v", opened, err)
	}
	if _, err = s.client.TakePetDrop(ann, &userv1.TakePetDropRequest{Name: ""}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("TakePetDrop without a name: %v", err)
	}
	taken, err := s.client.TakePetDrop(ann, &userv1.TakePetDropRequest{Name: "Находка"})
	if err != nil || taken.GetPet().GetKind() != opened.GetKeeper().GetDropKind() || taken.GetPet().GetBreed().GetHealth() == 0 {
		t.Fatalf("TakePetDrop = %v, %v", taken, err)
	}
	if _, err = s.client.LeavePetDrop(ann, &userv1.LeavePetDropRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("LeavePetDrop without a pet: %v", err)
	}
	pet := taken.GetPet().GetId()

	if _, err = s.client.BuyPetItem(ann, &userv1.BuyPetItemRequest{PetId: pet, Item: "tie"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("BuyPetItem of an unknown thing: %v", err)
	}
	bought, err := s.client.BuyPetItem(ann, &userv1.BuyPetItemRequest{PetId: pet, Item: "scarf"})
	if err != nil || bought.GetBalance() != quest || bought.GetPet().GetClothes().GetHealth() != 12 || len(bought.GetPet().GetOutfit()) != 1 {
		t.Fatalf("BuyPetItem = %v, %v", bought, err)
	}
	if _, err = s.client.BuyPetItem(ann, &userv1.BuyPetItemRequest{PetId: pet, Item: "scarf"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("buying the scarf again: %v", err)
	}
	dressed, err := s.client.DressPet(ann, &userv1.DressPetRequest{PetId: pet, Item: "scarf"})
	if err != nil || len(dressed.GetPet().GetOutfit()) != 0 || len(dressed.GetPet().GetWardrobe()) != 1 {
		t.Fatalf("DressPet = %v, %v", dressed, err)
	}
	if _, err = s.client.DressPet(ann, &userv1.DressPetRequest{PetId: pet, Item: "cape", Wear: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("wearing what was not bought: %v", err)
	}
}
