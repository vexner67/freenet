package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Pets are "Гости", "Рейтинги" and "Питомец", which the user service
// keeps with the people and their votes.
type Pets struct {
	api userv1.UserServiceClient
}

func NewPets(api userv1.UserServiceClient) *Pets {
	return &Pets{
		api: api,
	}
}

func (p *Pets) Visit(ctx context.Context, accessToken, nickname string) error {
	if _, err := p.api.VisitPage(withToken(ctx, accessToken), &userv1.VisitPageRequest{Nickname: nickname}); err != nil {
		return errs.Wrap("p.api.VisitPage", fromStatus(err))
	}

	return nil
}

func (p *Pets) Guests(ctx context.Context, accessToken, cursor string, limit int) (model.GuestPage, error) {
	resp, err := p.api.ListGuests(withToken(ctx, accessToken), &userv1.ListGuestsRequest{Cursor: cursor, Limit: int32(limit)}) //nolint:gosec // the page size is checked
	if err != nil {
		return model.GuestPage{}, errs.Wrap("p.api.ListGuests", fromStatus(err))
	}

	page := model.GuestPage{SeenAt: fromTimestamp(resp.GetSeenAt()), NextCursor: resp.GetNextCursor()}
	for _, g := range resp.GetGuests() {
		page.Guests = append(page.Guests, model.Guest{UserID: g.GetUserId(), VisitedAt: fromTimestamp(g.GetVisitedAt())})
	}

	return page, nil
}

func (p *Pets) Top(ctx context.Context, accessToken, board string) ([]model.TopEntry, error) {
	resp, err := p.api.ListTop(withToken(ctx, accessToken), &userv1.ListTopRequest{Board: board})
	if err != nil {
		return nil, errs.Wrap("p.api.ListTop", fromStatus(err))
	}

	entries := make([]model.TopEntry, 0, len(resp.GetEntries()))
	for _, e := range resp.GetEntries() {
		entries = append(entries, model.TopEntry{UserID: e.GetUserId(), Value: e.GetValue(), PetKind: e.GetPetKind(), PetName: e.GetPetName()})
	}

	return entries, nil
}

func (p *Pets) Get(ctx context.Context, accessToken string) (model.PetState, error) {
	resp, err := p.api.GetPet(withToken(ctx, accessToken), &userv1.GetPetRequest{})
	if err != nil {
		return model.PetState{}, errs.Wrap("p.api.GetPet", fromStatus(err))
	}

	return model.PetState{Shop: fromProtoPetShop(resp.GetShop()), Pets: fromProtoPets(resp.GetPets())}, nil
}

// UserPets are the pets of a page, the oldest first, and whose page it is.
func (p *Pets) UserPets(ctx context.Context, accessToken, nickname string) ([]model.Pet, string, error) {
	resp, err := p.api.GetUserPet(withToken(ctx, accessToken), &userv1.GetUserPetRequest{Nickname: nickname})
	if err != nil {
		return nil, "", errs.Wrap("p.api.GetUserPet", fromStatus(err))
	}

	return fromProtoPets(resp.GetPets()), resp.GetOwnerId(), nil
}

func (p *Pets) Adopt(ctx context.Context, accessToken, kind, name string) (model.PetCare, error) {
	resp, err := p.api.AdoptPet(withToken(ctx, accessToken), &userv1.AdoptPetRequest{Kind: kind, Name: name})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.AdoptPet", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

func (p *Pets) Feed(ctx context.Context, accessToken, petID string) (model.PetCare, error) {
	resp, err := p.api.FeedPet(withToken(ctx, accessToken), &userv1.FeedPetRequest{PetId: petID})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.FeedPet", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

func (p *Pets) Play(ctx context.Context, accessToken, petID string) (model.PetCare, error) {
	resp, err := p.api.PlayPet(withToken(ctx, accessToken), &userv1.PlayPetRequest{PetId: petID})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.PlayPet", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

func (p *Pets) Rename(ctx context.Context, accessToken, petID, name string) (model.PetCare, error) {
	resp, err := p.api.RenamePet(withToken(ctx, accessToken), &userv1.RenamePetRequest{PetId: petID, Name: name})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.RenamePet", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

func (p *Pets) Release(ctx context.Context, accessToken, petID string) error {
	if _, err := p.api.ReleasePet(withToken(ctx, accessToken), &userv1.ReleasePetRequest{PetId: petID}); err != nil {
		return errs.Wrap("p.api.ReleasePet", fromStatus(err))
	}

	return nil
}

// Fight sets the caller's pet petID on the best pet of nickname, "" for
// one of a near rating.
func (p *Pets) Fight(ctx context.Context, accessToken, petID, nickname string) (model.PetFought, error) {
	resp, err := p.api.FightPet(withToken(ctx, accessToken), &userv1.FightPetRequest{PetId: petID, Nickname: nickname})
	if err != nil {
		return model.PetFought{}, errs.Wrap("p.api.FightPet", fromStatus(err))
	}

	return model.PetFought{Fight: fromProtoPetFight(resp.GetFight()), Pet: fromProtoPet(resp.GetPet()), Balance: resp.GetBalance()}, nil
}

func (p *Pets) Fights(ctx context.Context, accessToken string) ([]model.PetFight, error) {
	resp, err := p.api.ListPetFights(withToken(ctx, accessToken), &userv1.ListPetFightsRequest{})
	if err != nil {
		return nil, errs.Wrap("p.api.ListPetFights", fromStatus(err))
	}

	fights := make([]model.PetFight, 0, len(resp.GetFights()))
	for _, f := range resp.GetFights() {
		fights = append(fights, fromProtoPetFight(f))
	}

	return fights, nil
}

func (p *Pets) Settings(ctx context.Context, accessToken string) (model.PetShop, model.PetStats, error) {
	resp, err := p.api.GetPetSettings(withToken(ctx, accessToken), &userv1.GetPetSettingsRequest{})
	if err != nil {
		return model.PetShop{}, model.PetStats{}, errs.Wrap("p.api.GetPetSettings", fromStatus(err))
	}

	stats := model.PetStats{Pets: resp.GetPets(), Fights: resp.GetFights(), Spent: resp.GetSpent(), Won: resp.GetWon()}

	return fromProtoPetShop(resp.GetShop()), stats, nil
}

func (p *Pets) Set(ctx context.Context, accessToken string, shop model.PetShop) error {
	fights, err := int32Of(shop.Fights, "fights")
	if err != nil {
		return err
	}

	req := &userv1.SetPetSettingsRequest{Shop: &userv1.PetShop{Prices: shop.Prices, Meal: shop.Meal, Win: shop.Win, Fights: fights}}
	if _, err = p.api.SetPetSettings(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("p.api.SetPetSettings", fromStatus(err))
	}

	return nil
}

func fromProtoPetShop(s *userv1.PetShop) model.PetShop {
	return model.PetShop{Prices: s.GetPrices(), Meal: s.GetMeal(), Win: s.GetWin(), Fights: int(s.GetFights())}
}

func fromProtoPets(ps []*userv1.Pet) []model.Pet {
	pets := make([]model.Pet, 0, len(ps))
	for _, p := range ps {
		pets = append(pets, fromProtoPet(p))
	}

	return pets
}

func fromProtoPet(p *userv1.Pet) model.Pet {
	return model.Pet{
		ID:                  p.GetId(),
		Kind:                p.GetKind(),
		Name:                p.GetName(),
		BornAt:              fromTimestamp(p.GetBornAt()),
		Satiety:             int(p.GetSatiety()),
		Joy:                 int(p.GetJoy()),
		PlayAt:              fromTimestamp(p.GetPlayAt()),
		Experience:          p.GetExperience(),
		Level:               int(p.GetLevel()),
		LevelExperience:     p.GetLevelExperience(),
		NextLevelExperience: p.GetNextLevelExperience(),
		Rating:              int(p.GetRating()),
		Wins:                int(p.GetWins()),
		Losses:              int(p.GetLosses()),
		Fights:              int(p.GetFights()),
		Attack:              int(p.GetAttack()),
		Defense:             int(p.GetDefense()),
		Health:              int(p.GetHealth()),
		Speed:               int(p.GetSpeed()),
		Crit:                int(p.GetCrit()),
	}
}

func fromProtoPetCare(resp *userv1.PetCareResponse) model.PetCare {
	return model.PetCare{Pet: fromProtoPet(resp.GetPet()), Balance: resp.GetBalance()}
}

func fromProtoPetFighter(f *userv1.PetFighter) model.PetFighter {
	return model.PetFighter{Kind: f.GetKind(), Name: f.GetName(), Level: int(f.GetLevel()), Health: int(f.GetHealth())}
}

func fromProtoPetFight(f *userv1.PetFight) model.PetFight {
	fight := model.PetFight{
		ID:         f.GetId(),
		AttackerID: f.GetAttackerId(),
		DefenderID: f.GetDefenderId(),
		FoughtAt:   fromTimestamp(f.GetFoughtAt()),
		Attacker:   fromProtoPetFighter(f.GetAttacker()),
		Defender:   fromProtoPetFighter(f.GetDefender()),
		Hits:       make([]model.PetHit, 0, len(f.GetHits())),
		Won:        f.GetWon(),
		Rating:     int(f.GetRating()),
	}
	for _, h := range f.GetHits() {
		fight.Hits = append(fight.Hits, model.PetHit{
			ByAttacker: h.GetByAttacker(), Dodged: h.GetDodged(), Crit: h.GetCrit(), Damage: int(h.GetDamage()),
			AttackerHealth: int(h.GetAttackerHealth()), DefenderHealth: int(h.GetDefenderHealth()),
		})
	}

	return fight
}
