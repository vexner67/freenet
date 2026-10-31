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

	state := model.PetState{Shop: fromProtoPetShop(resp.GetShop()), Pets: fromProtoPets(resp.GetPets())}
	for _, k := range resp.GetKinds() {
		state.Kinds = append(state.Kinds, model.PetKind{
			Kind: k.GetKind(), Rarity: k.GetRarity(), InShop: k.GetInShop(), Stats: fromProtoPetPower(k.GetStats()), Crit: int(k.GetCrit()),
			Element: k.GetElement(), Special: k.GetSpecial(), Shards: int(k.GetShards()),
		})
	}
	for _, c := range resp.GetCases() {
		odds := make([]int, len(c.GetOdds()))
		for i, o := range c.GetOdds() {
			odds[i] = int(o)
		}
		state.Cases = append(state.Cases, model.PetCase{ID: c.GetId(), Odds: odds, Shards: int(c.GetShards())})
	}
	for _, item := range resp.GetItems() {
		state.Items = append(state.Items, model.PetItem{ID: item.GetId(), Slot: item.GetSlot(), Price: item.GetPrice(), Bonus: fromProtoPetPower(item.GetBonus())})
	}
	state.Keeper = fromProtoPetKeeper(resp.GetKeeper())
	for _, b := range resp.GetBattles() {
		state.Battles = append(state.Battles, fromProtoPetBattle(b))
	}

	return state, nil
}

// UserPets are the pets of a page, the oldest first, and whose page it is.
func (p *Pets) UserPets(ctx context.Context, accessToken, nickname string) ([]model.Pet, string, error) {
	resp, err := p.api.GetUserPet(withToken(ctx, accessToken), &userv1.GetUserPetRequest{Nickname: nickname})
	if err != nil {
		return nil, "", errs.Wrap("p.api.GetUserPet", fromStatus(err))
	}

	return fromProtoPets(resp.GetPets()), resp.GetOwnerId(), nil
}

func (p *Pets) NamedPet(ctx context.Context, accessToken string) (model.NamedPet, error) {
	resp, err := p.api.GetNamedPet(withToken(ctx, accessToken), &userv1.GetNamedPetRequest{})
	if err != nil {
		return model.NamedPet{}, errs.Wrap("p.api.GetNamedPet", fromStatus(err))
	}

	return model.NamedPet{Kind: resp.GetKind(), OpensAt: resp.GetOpensAt().AsTime(), Claimed: resp.GetClaimed()}, nil
}

func (p *Pets) ClaimNamedPet(ctx context.Context, accessToken string) (model.PetCare, error) {
	resp, err := p.api.ClaimNamedPet(withToken(ctx, accessToken), &userv1.ClaimNamedPetRequest{})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.ClaimNamedPet", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
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

// OpenCase opens the case caseID, for shards with withShards: its pet
// waits to be taken or left.
func (p *Pets) OpenCase(ctx context.Context, accessToken, caseID string, withShards bool) (model.PetDrop, error) {
	resp, err := p.api.OpenPetCase(withToken(ctx, accessToken), &userv1.OpenPetCaseRequest{CaseId: caseID, Shards: withShards})
	if err != nil {
		return model.PetDrop{}, errs.Wrap("p.api.OpenPetCase", fromStatus(err))
	}

	return model.PetDrop{Keeper: fromProtoPetKeeper(resp.GetKeeper()), Balance: resp.GetBalance()}, nil
}

// TakeDrop takes the pet of the case, called name.
func (p *Pets) TakeDrop(ctx context.Context, accessToken, name string) (model.PetCare, error) {
	resp, err := p.api.TakePetDrop(withToken(ctx, accessToken), &userv1.TakePetDropRequest{Name: name})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.TakePetDrop", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

// LeaveDrop leaves the pet of the case for its shards.
func (p *Pets) LeaveDrop(ctx context.Context, accessToken string) (model.PetKeeper, error) {
	resp, err := p.api.LeavePetDrop(withToken(ctx, accessToken), &userv1.LeavePetDropRequest{})
	if err != nil {
		return model.PetKeeper{}, errs.Wrap("p.api.LeavePetDrop", fromStatus(err))
	}

	return fromProtoPetKeeper(resp.GetKeeper()), nil
}

// SetStance sets the stance the pet defends in.
func (p *Pets) SetStance(ctx context.Context, accessToken, petID, stance string) (model.PetCare, error) {
	resp, err := p.api.SetPetStance(withToken(ctx, accessToken), &userv1.SetPetStanceRequest{PetId: petID, Stance: stance})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.SetPetStance", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

// StartBattle starts a fight of the pet in stance the caller leads a round
// after another, against the best pet of nickname, "" for one near in
// power.
func (p *Pets) StartBattle(ctx context.Context, accessToken, petID, nickname, stance string) (model.PetBattleTurn, error) {
	resp, err := p.api.StartPetBattle(withToken(ctx, accessToken), &userv1.StartPetBattleRequest{PetId: petID, Nickname: nickname, Stance: stance})
	if err != nil {
		return model.PetBattleTurn{}, errs.Wrap("p.api.StartPetBattle", fromStatus(err))
	}

	return fromProtoPetBattleTurn(resp), nil
}

// MoveBattle makes move in the fight of the pet.
func (p *Pets) MoveBattle(ctx context.Context, accessToken, petID, move string) (model.PetBattleTurn, error) {
	resp, err := p.api.MovePetBattle(withToken(ctx, accessToken), &userv1.MovePetBattleRequest{PetId: petID, Move: move})
	if err != nil {
		return model.PetBattleTurn{}, errs.Wrap("p.api.MovePetBattle", fromStatus(err))
	}

	return fromProtoPetBattleTurn(resp), nil
}

func (p *Pets) BuyItem(ctx context.Context, accessToken, petID, item string) (model.PetCare, error) {
	resp, err := p.api.BuyPetItem(withToken(ctx, accessToken), &userv1.BuyPetItemRequest{PetId: petID, Item: item})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.BuyPetItem", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

// Dress puts item on the pet, or takes it off.
func (p *Pets) Dress(ctx context.Context, accessToken, petID, item string, wear bool) (model.PetCare, error) {
	resp, err := p.api.DressPet(withToken(ctx, accessToken), &userv1.DressPetRequest{PetId: petID, Item: item, Wear: wear})
	if err != nil {
		return model.PetCare{}, errs.Wrap("p.api.DressPet", fromStatus(err))
	}

	return fromProtoPetCare(resp), nil
}

func (p *Pets) Release(ctx context.Context, accessToken, petID string) error {
	if _, err := p.api.ReleasePet(withToken(ctx, accessToken), &userv1.ReleasePetRequest{PetId: petID}); err != nil {
		return errs.Wrap("p.api.ReleasePet", fromStatus(err))
	}

	return nil
}

// Fight sets the caller's pet petID in stance on the best pet of
// nickname, "" for one near in power.
func (p *Pets) Fight(ctx context.Context, accessToken, petID, nickname, stance string) (model.PetFought, error) {
	resp, err := p.api.FightPet(withToken(ctx, accessToken), &userv1.FightPetRequest{PetId: petID, Nickname: nickname, Stance: stance})
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

	casesADay, err := int32Of(shop.CasesADay, "cases a day")
	if err != nil {
		return err
	}

	req := &userv1.SetPetSettingsRequest{Shop: &userv1.PetShop{Prices: shop.Prices, Cases: shop.Cases, Meal: shop.Meal, Win: shop.Win, Fights: fights, CasesADay: casesADay}}
	if _, err = p.api.SetPetSettings(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("p.api.SetPetSettings", fromStatus(err))
	}

	return nil
}

func fromProtoPetShop(s *userv1.PetShop) model.PetShop {
	return model.PetShop{Prices: s.GetPrices(), Cases: s.GetCases(), Meal: s.GetMeal(), Win: s.GetWin(), Fights: int(s.GetFights()), CasesADay: int(s.GetCasesADay())}
}

func fromProtoPetPower(s *userv1.PetStats) model.PetPower {
	return model.PetPower{Attack: int(s.GetAttack()), Defense: int(s.GetDefense()), Health: int(s.GetHealth()), Speed: int(s.GetSpeed())}
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
		Rarity:              p.GetRarity(),
		Wardrobe:            p.GetWardrobe(),
		Outfit:              p.GetOutfit(),
		Breed:               fromProtoPetPower(p.GetBreed()),
		Clothes:             fromProtoPetPower(p.GetClothes()),
		Stance:              p.GetStance(),
		Power:               int(p.GetPower()),
		Fatigue:             int(p.GetFatigue()),
		SalePrice:           p.GetSalePrice(),
		SaleAt:              fromTimestamp(p.GetSaleAt()),
		BossAt:              fromTimestamp(p.GetBossAt()),
	}
}

func fromProtoPetCare(resp *userv1.PetCareResponse) model.PetCare {
	return model.PetCare{Pet: fromProtoPet(resp.GetPet()), Balance: resp.GetBalance()}
}

func fromProtoPetFighter(f *userv1.PetFighter) model.PetFighter {
	return model.PetFighter{
		Kind: f.GetKind(), Name: f.GetName(), Level: int(f.GetLevel()), Health: int(f.GetHealth()), Outfit: f.GetOutfit(),
		Stance: f.GetStance(), Power: int(f.GetPower()), Fatigue: int(f.GetFatigue()),
	}
}

func fromProtoPetHits(hits []*userv1.PetHit) []model.PetHit {
	out := make([]model.PetHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, model.PetHit{
			ByAttacker: h.GetByAttacker(), Move: h.GetMove(), Dodged: h.GetDodged(), Crit: h.GetCrit(), Damage: int(h.GetDamage()),
			Heal: int(h.GetHeal()), Special: h.GetSpecial(), Answer: h.GetAnswer(),
			AttackerHealth: int(h.GetAttackerHealth()), DefenderHealth: int(h.GetDefenderHealth()),
		})
	}

	return out
}

func fromProtoPetFight(f *userv1.PetFight) model.PetFight {
	return model.PetFight{
		ID:         f.GetId(),
		AttackerID: f.GetAttackerId(),
		DefenderID: f.GetDefenderId(),
		FoughtAt:   fromTimestamp(f.GetFoughtAt()),
		Attacker:   fromProtoPetFighter(f.GetAttacker()),
		Defender:   fromProtoPetFighter(f.GetDefender()),
		Hits:       fromProtoPetHits(f.GetHits()),
		Won:        f.GetWon(),
		Rating:     int(f.GetRating()),
		Ratio:      int(f.GetRatio()),
		Reward:     f.GetReward(),
	}
}

func fromProtoPetKeeper(k *userv1.PetKeeper) model.PetKeeper {
	return model.PetKeeper{
		Shards: int(k.GetShards()), Pity: int(k.GetPity()), PityLimit: int(k.GetPityLimit()),
		Cases: int(k.GetCases()), CasesLimit: int(k.GetCasesLimit()), DropKind: k.GetDropKind(), DropCase: k.GetDropCase(),
	}
}

func fromProtoPetBattle(b *userv1.PetBattle) model.PetBattle {
	return model.PetBattle{
		PetID:          b.GetPetId(),
		DefenderID:     b.GetDefenderId(),
		Attacker:       fromProtoPetFighter(b.GetAttacker()),
		Defender:       fromProtoPetFighter(b.GetDefender()),
		Hits:           fromProtoPetHits(b.GetHits()),
		Round:          int(b.GetRound()),
		Rounds:         int(b.GetRounds()),
		AttackerHealth: int(b.GetAttackerHealth()),
		DefenderHealth: int(b.GetDefenderHealth()),
		Ratio:          int(b.GetRatio()),
		Win:            int(b.GetWin()),
		Loss:           int(b.GetLoss()),
	}
}

func fromProtoPetBattleTurn(resp *userv1.PetBattleResponse) model.PetBattleTurn {
	turn := model.PetBattleTurn{Battle: fromProtoPetBattle(resp.GetBattle()), Pet: fromProtoPet(resp.GetPet()), Balance: resp.GetBalance()}
	if resp.GetFight() != nil {
		fight := fromProtoPetFight(resp.GetFight())
		turn.Fight = &fight
	}

	return turn
}
