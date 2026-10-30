package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GuestService interface {
	Visit(ctx context.Context, guestID uuid.UUID, nickname user.Nickname, now time.Time) error
	Guests(ctx context.Context, ownerID uuid.UUID, cursor string, limit int, now time.Time) (service.GuestPage, error)
	NewGuests(ctx context.Context, ownerID uuid.UUID, now time.Time) (int, error)
}

type TopService interface {
	Top(ctx context.Context, board user.TopBoard, now time.Time) ([]user.TopEntry, error)
}

type PetService interface {
	Shop(ctx context.Context) (user.PetShop, error)
	Pets(ctx context.Context, userID uuid.UUID) ([]*user.Pet, error)
	UserPets(ctx context.Context, nickname user.Nickname, now time.Time) ([]*user.Pet, uuid.UUID, error)
	Adopt(ctx context.Context, userID uuid.UUID, kind user.PetKind, name user.PetName, now time.Time) (service.PetCare, error)
	Big2Z(ctx context.Context, userID uuid.UUID) (service.NamedPet, error)
	ClaimBig2Z(ctx context.Context, userID uuid.UUID, now time.Time) (*user.Pet, error)
	Feed(ctx context.Context, userID, petID uuid.UUID, now time.Time) (service.PetCare, error)
	Play(ctx context.Context, userID, petID uuid.UUID, now time.Time) (*user.Pet, error)
	Rename(ctx context.Context, userID, petID uuid.UUID, name user.PetName) (*user.Pet, error)
	Release(ctx context.Context, userID, petID uuid.UUID) (*user.PetKeeper, error)
	SetStance(ctx context.Context, userID, petID uuid.UUID, stance user.PetStance) (*user.Pet, error)
	Fight(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, stance user.PetStance, now time.Time) (service.PetFought, error)
	Fights(ctx context.Context, userID uuid.UUID) ([]user.PetFight, error)
	Battles(ctx context.Context, userID uuid.UUID) ([]*user.PetBattle, error)
	StartBattle(ctx context.Context, userID, petID uuid.UUID, nickname user.Nickname, stance user.PetStance, now time.Time) (service.PetBattleTurn, error)
	MoveBattle(ctx context.Context, userID, petID uuid.UUID, move user.PetMove, now time.Time) (service.PetBattleTurn, error)
	Keeper(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error)
	OpenCase(ctx context.Context, userID uuid.UUID, c user.PetCase, withShards bool, now time.Time) (service.PetDrop, error)
	TakeDrop(ctx context.Context, userID uuid.UUID, name user.PetName, now time.Time) (*user.Pet, error)
	LeaveDrop(ctx context.Context, userID uuid.UUID) (*user.PetKeeper, error)
	BuyItem(ctx context.Context, userID, petID uuid.UUID, item user.PetItem) (service.PetCare, error)
	Dress(ctx context.Context, userID, petID uuid.UUID, item user.PetItem, wear bool) (*user.Pet, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.PetSettings, error)
	SetShop(ctx context.Context, adminID uuid.UUID, shop user.PetShop) error
	Arena(ctx context.Context, userID uuid.UUID, now time.Time) (service.PetArena, error)
	RunArena(ctx context.Context, a service.PetArenaAct) (service.PetArenaDone, error)
}

// petOf is the ID of a pet of the caller; one of a wrong form is no pet.
func petOf(id string) (uuid.UUID, error) {
	petID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, user.ErrPetNotFound
	}

	return petID, nil
}

// petCaller is the caller and the pet they name.
func petCaller(ctx context.Context, id string) (userID, petID uuid.UUID, err error) {
	if userID, err = currentUserID(ctx); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	petID, err = petOf(id)

	return userID, petID, err
}

// toProtoPets are pets at now.
func toProtoPets(pets []*user.Pet, now time.Time) []*userv1.Pet {
	out := make([]*userv1.Pet, len(pets))
	for i, p := range pets {
		out[i] = toProtoPet(p, now)
	}

	return out
}

// pageOwner is the nickname of a page; one of a wrong form names nobody.
func pageOwner(nickname string) (user.Nickname, error) {
	n, err := user.NewNickname(nickname)
	if err != nil {
		return "", user.ErrNotFound
	}

	return n, nil
}

func (h *UserHandler) VisitPage(ctx context.Context, req *userv1.VisitPageRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	if err = h.guests.Visit(ctx, userID, nickname, time.Now()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) ListGuests(ctx context.Context, req *userv1.ListGuestsRequest) (*userv1.ListGuestsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, err := h.guests.Guests(ctx, userID, req.GetCursor(), int(req.GetLimit()), time.Now())
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListGuestsResponse{NextCursor: page.NextCursor}
	if !page.SeenAt.IsZero() {
		resp.SeenAt = timestamppb.New(page.SeenAt)
	}
	for _, g := range page.Guests {
		resp.Guests = append(resp.Guests, &userv1.Guest{UserId: g.GuestID.String(), VisitedAt: timestamppb.New(g.VisitedAt)})
	}

	return resp, nil
}

func (h *UserHandler) ListTop(ctx context.Context, req *userv1.ListTopRequest) (*userv1.ListTopResponse, error) {
	board, err := user.NewTopBoard(req.GetBoard())
	if err != nil {
		return nil, err
	}

	entries, err := h.tops.Top(ctx, board, time.Now())
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListTopResponse{}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &userv1.TopEntry{
			UserId: e.UserID.String(), Value: e.Value, PetKind: e.PetKind.String(), PetName: e.PetName.String(),
		})
	}

	return resp, nil
}

func toProtoPetShop(s user.PetShop) *userv1.PetShop {
	prices := make(map[string]int64, len(s.Prices()))
	for kind, price := range s.Prices() {
		prices[kind.String()] = price
	}

	cases := make(map[string]int64, len(s.Cases()))
	for c, price := range s.Cases() {
		cases[c.String()] = price
	}

	return &userv1.PetShop{Prices: prices, Cases: cases, Meal: s.Meal(), Win: s.Win(), Fights: int32(s.Fights())} //nolint:gosec // at most user.MaxPetFightsADay
}

//nolint:gosec // the stats of a pet are small
func toProtoPetStats(s user.PetStats) *userv1.PetStats {
	return &userv1.PetStats{Attack: int32(s.Attack), Defense: int32(s.Defense), Health: int32(s.Health), Speed: int32(s.Speed)}
}

func itemNames(items []user.PetItem) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.String()
	}

	return names
}

// petCatalogue is what the shop knows: the kinds, the cases and the
// clothes.
//
//nolint:gosec // odds are in a thousand, crits in a hundred
func petCatalogue(state *userv1.PetState) *userv1.PetState {
	shop := map[user.PetKind]bool{}
	for _, k := range user.ShopPetKinds {
		shop[k] = true
	}
	for _, k := range user.PetKinds {
		breed := user.RestorePet(user.PetState{Kind: k})
		state.Kinds = append(state.Kinds, &userv1.PetKind{
			Kind: k.String(), Rarity: k.Rarity().String(), InShop: shop[k], Stats: toProtoPetStats(breed.BreedStats()), Crit: int32(breed.Crit()),
			Element: k.Element().String(), Special: k.Special().String(), Shards: int32(k.Rarity().Shards()),
		})
	}
	for _, c := range user.PetCases {
		pc := &userv1.PetCase{Id: c.String(), Shards: int32(c.Shards())}
		for _, odds := range c.Odds() {
			pc.Odds = append(pc.Odds, int32(odds))
		}
		state.Cases = append(state.Cases, pc)
	}
	for _, item := range user.PetItems {
		state.Items = append(state.Items, &userv1.PetItem{
			Id: item.Item.String(), Slot: string(item.Slot), Price: item.Price, Bonus: toProtoPetStats(item.Bonus),
		})
	}

	return state
}

func fromProtoPetShop(s *userv1.PetShop) (user.PetShop, error) {
	prices := make(map[user.PetKind]int64, len(s.GetPrices()))
	for kind, price := range s.GetPrices() {
		k, err := user.NewPetKind(kind)
		if err != nil {
			return user.PetShop{}, err
		}
		prices[k] = price
	}

	shop, err := user.NewPetShop(prices, s.GetMeal(), s.GetWin(), int(s.GetFights()))
	if err != nil || s.GetCases() == nil {
		return shop, err
	}

	cases := make(map[user.PetCase]int64, len(s.GetCases()))
	for id, price := range s.GetCases() {
		c, err := user.NewPetCase(id)
		if err != nil {
			return user.PetShop{}, err
		}
		cases[c] = price
	}

	return shop.WithCases(cases)
}

//nolint:gosec // the numbers of a pet are small: feelings to 100, levels to 30, stats and fights of a day
func toProtoPet(p *user.Pet, now time.Time) *userv1.Pet {
	if p == nil {
		return nil
	}

	stats, level := p.Stats(), p.Level()
	pet := &userv1.Pet{
		Id:              p.ID().String(),
		Kind:            p.Kind().String(),
		Name:            p.Name().String(),
		BornAt:          timestamppb.New(p.BornAt()),
		Satiety:         int32(p.SatietyAt(now)),
		Joy:             int32(p.JoyAt(now)),
		Experience:      p.Experience(),
		Level:           int32(level),
		LevelExperience: user.PetLevelExperience(level),
		Rating:          int32(p.Rating()),
		Wins:            int32(p.Wins()),
		Losses:          int32(p.Losses()),
		Fights:          int32(p.FightsAt(now)),
		Attack:          int32(stats.Attack),
		Defense:         int32(stats.Defense),
		Health:          int32(stats.Health),
		Speed:           int32(stats.Speed),
		Crit:            int32(p.Crit()),
		Rarity:          p.Kind().Rarity().String(),
		Wardrobe:        itemNames(p.Wardrobe()),
		Outfit:          itemNames(p.Outfit()),
		Breed:           toProtoPetStats(p.BreedStats()),
		Clothes:         toProtoPetStats(p.ClothesStats()),
		Stance:          p.Stance().String(),
		Power:           int32(p.Power()),
		Fatigue:         int32(p.FatigueAt(now)),
		SalePrice:       p.SalePrice(),
	}
	if at := p.SaleAt(); at.After(now) {
		pet.SaleAt = timestamppb.New(at)
	}
	if at := p.BossHitAt(); at.After(now) {
		pet.BossAt = timestamppb.New(at)
	}
	if level < user.MaxPetLevel {
		pet.NextLevelExperience = user.PetLevelExperience(level + 1)
	}
	if at := p.PlayAt(); at.After(now) {
		pet.PlayAt = timestamppb.New(at)
	}

	return pet
}

//nolint:gosec // the numbers of a fight are small: health, damage, levels and power
func toProtoPetFighter(f user.PetFighter) *userv1.PetFighter {
	return &userv1.PetFighter{
		Kind: f.Kind.String(), Name: f.Name.String(), Level: int32(f.Level), Health: int32(f.Health), Outfit: itemNames(f.Outfit),
		Stance: f.Stance.String(), Power: int32(f.Power), Fatigue: int32(f.Fatigue),
	}
}

//nolint:gosec // the numbers of a fight are small: health and damage
func toProtoPetHits(hits []user.PetHit) []*userv1.PetHit {
	out := make([]*userv1.PetHit, len(hits))
	for i, h := range hits {
		out[i] = &userv1.PetHit{
			ByAttacker: h.ByAttacker, Dodged: h.Dodged, Crit: h.Crit, Damage: int32(h.Damage),
			AttackerHealth: int32(h.AttackerHealth), DefenderHealth: int32(h.DefenderHealth),
			Move: h.Move.String(), Heal: int32(h.Heal), Special: h.Special.String(), Answer: h.Answer.String(),
		}
	}

	return out
}

//nolint:gosec // the numbers of a fight are small: rating points and a percent
func toProtoPetFight(f user.PetFight) *userv1.PetFight {
	return &userv1.PetFight{
		Id:         f.ID.String(),
		AttackerId: f.AttackerID.String(),
		DefenderId: f.DefenderID.String(),
		FoughtAt:   timestamppb.New(f.At),
		Attacker:   toProtoPetFighter(f.Bout.Attacker),
		Defender:   toProtoPetFighter(f.Bout.Defender),
		Hits:       toProtoPetHits(f.Bout.Hits),
		Won:        f.Bout.Won,
		Rating:     int32(f.Bout.Rating),
		Ratio:      int32(f.Bout.Ratio),
		Reward:     f.Bout.Reward,
	}
}

//nolint:gosec // the numbers of a fight are small: rounds, health, rating points and a percent
func toProtoPetBattle(b *user.PetBattle) *userv1.PetBattle {
	if b == nil {
		return nil
	}

	a, d := b.Attacker(), b.Defender()
	win, loss := b.Rating()

	return &userv1.PetBattle{
		PetId:          b.PetID().String(),
		DefenderId:     b.DefenderOwner().String(),
		Attacker:       toProtoPetFighter(a.Fighter),
		Defender:       toProtoPetFighter(d.Fighter),
		Hits:           toProtoPetHits(b.Hits()),
		Round:          int32(b.Round()),
		Rounds:         user.MaxPetRounds,
		AttackerHealth: int32(a.Health),
		DefenderHealth: int32(d.Health),
		Ratio:          int32(b.Ratio()),
		Win:            int32(win),
		Loss:           int32(loss),
	}
}

//nolint:gosec // shards and cases are small
func toProtoPetKeeper(k *user.PetKeeper, now time.Time) *userv1.PetKeeper {
	return &userv1.PetKeeper{
		Shards: int32(k.Shards()), Pity: int32(k.Pity()), PityLimit: user.PetPity,
		Cases: int32(k.CasesAt(now)), CasesLimit: user.MaxPetCasesADay,
		DropKind: k.Drop().String(), DropCase: k.DropCase().String(),
	}
}

func (h *UserHandler) GetPet(ctx context.Context, _ *userv1.GetPetRequest) (*userv1.PetState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	shop, err := h.pets.Shop(ctx)
	if err != nil {
		return nil, err
	}

	pets, err := h.pets.Pets(ctx, userID)
	if err != nil {
		return nil, err
	}

	keeper, err := h.pets.Keeper(ctx, userID)
	if err != nil {
		return nil, err
	}

	battles, err := h.pets.Battles(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	state := &userv1.PetState{Shop: toProtoPetShop(shop), Pets: toProtoPets(pets, now), Keeper: toProtoPetKeeper(keeper, now)}
	for _, b := range battles {
		state.Battles = append(state.Battles, toProtoPetBattle(b))
	}

	return petCatalogue(state), nil
}

func (h *UserHandler) GetUserPet(ctx context.Context, req *userv1.GetUserPetRequest) (*userv1.GetUserPetResponse, error) {
	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	pets, ownerID, err := h.pets.UserPets(ctx, nickname, now)
	if err != nil {
		return nil, err
	}

	return &userv1.GetUserPetResponse{Pets: toProtoPets(pets, now), OwnerId: ownerID.String()}, nil
}

func (h *UserHandler) GetNamedPet(ctx context.Context, _ *userv1.GetNamedPetRequest) (*userv1.GetNamedPetResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	named, err := h.pets.Big2Z(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetNamedPetResponse{Kind: named.Kind.String(), OpensAt: timestamppb.New(named.OpensAt), Claimed: named.Claimed}, nil
}

func (h *UserHandler) ClaimNamedPet(ctx context.Context, _ *userv1.ClaimNamedPetRequest) (*userv1.PetCareResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	pet, err := h.pets.ClaimBig2Z(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(pet, now)}, nil
}

func (h *UserHandler) AdoptPet(ctx context.Context, req *userv1.AdoptPetRequest) (*userv1.PetCareResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := user.NewPetKind(req.GetKind())
	if err != nil {
		return nil, err
	}
	name, err := user.NewPetName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	now := time.Now()

	care, err := h.pets.Adopt(ctx, userID, kind, name, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(care.Pet, now), Balance: care.Balance}, nil
}

func (h *UserHandler) FeedPet(ctx context.Context, req *userv1.FeedPetRequest) (*userv1.PetCareResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	care, err := h.pets.Feed(ctx, userID, petID, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(care.Pet, now), Balance: care.Balance}, nil
}

func (h *UserHandler) PlayPet(ctx context.Context, req *userv1.PlayPetRequest) (*userv1.PetCareResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	p, err := h.pets.Play(ctx, userID, petID, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, now)}, nil
}

func (h *UserHandler) RenamePet(ctx context.Context, req *userv1.RenamePetRequest) (*userv1.PetCareResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	name, err := user.NewPetName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.pets.Rename(ctx, userID, petID, name)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, time.Now())}, nil
}

func (h *UserHandler) ReleasePet(ctx context.Context, req *userv1.ReleasePetRequest) (*emptypb.Empty, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	if _, err = h.pets.Release(ctx, userID, petID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) SetPetStance(ctx context.Context, req *userv1.SetPetStanceRequest) (*userv1.PetCareResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	stance, err := user.NewPetStance(req.GetStance())
	if err != nil {
		return nil, err
	}

	p, err := h.pets.SetStance(ctx, userID, petID, stance)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, time.Now())}, nil
}

// fightRequest is what a fight is asked with: the opponent, the nickname
// of whose best pet or empty for any, and the stance, empty for the one
// the pet defends in.
func fightRequest(nickname, stance string) (user.Nickname, user.PetStance, error) {
	var (
		n   user.Nickname
		st  user.PetStance
		err error
	)

	if nickname != "" {
		if n, err = pageOwner(nickname); err != nil {
			return "", "", err
		}
	}
	if stance != "" {
		if st, err = user.NewPetStance(stance); err != nil {
			return "", "", err
		}
	}

	return n, st, nil
}

func (h *UserHandler) StartPetBattle(ctx context.Context, req *userv1.StartPetBattleRequest) (*userv1.PetBattleResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	nickname, stance, err := fightRequest(req.GetNickname(), req.GetStance())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	turn, err := h.pets.StartBattle(ctx, userID, petID, nickname, stance, now)
	if err != nil {
		return nil, err
	}

	return toProtoPetBattleResponse(turn, now), nil
}

func (h *UserHandler) MovePetBattle(ctx context.Context, req *userv1.MovePetBattleRequest) (*userv1.PetBattleResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	move, err := user.NewPetMove(req.GetMove())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	turn, err := h.pets.MoveBattle(ctx, userID, petID, move, now)
	if err != nil {
		return nil, err
	}

	return toProtoPetBattleResponse(turn, now), nil
}

func toProtoPetBattleResponse(turn service.PetBattleTurn, now time.Time) *userv1.PetBattleResponse {
	resp := &userv1.PetBattleResponse{Battle: toProtoPetBattle(turn.Battle), Pet: toProtoPet(turn.Pet, now)}
	if turn.Fought != nil {
		resp.Fight, resp.Balance = toProtoPetFight(turn.Fought.Fight), turn.Fought.Balance
	}

	return resp
}

func (h *UserHandler) FightPet(ctx context.Context, req *userv1.FightPetRequest) (*userv1.FightPetResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	nickname, stance, err := fightRequest(req.GetNickname(), req.GetStance())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	fought, err := h.pets.Fight(ctx, userID, petID, nickname, stance, now)
	if err != nil {
		return nil, err
	}

	return &userv1.FightPetResponse{Fight: toProtoPetFight(fought.Fight), Pet: toProtoPet(fought.Pet, now), Balance: fought.Balance}, nil
}

func (h *UserHandler) OpenPetCase(ctx context.Context, req *userv1.OpenPetCaseRequest) (*userv1.OpenPetCaseResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	c, err := user.NewPetCase(req.GetCaseId())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	drop, err := h.pets.OpenCase(ctx, userID, c, req.GetShards(), now)
	if err != nil {
		return nil, err
	}

	return &userv1.OpenPetCaseResponse{Keeper: toProtoPetKeeper(drop.Keeper, now), Balance: drop.Balance}, nil
}

func (h *UserHandler) TakePetDrop(ctx context.Context, req *userv1.TakePetDropRequest) (*userv1.PetCareResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	name, err := user.NewPetName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	now := time.Now()

	p, err := h.pets.TakeDrop(ctx, userID, name, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, now)}, nil
}

func (h *UserHandler) LeavePetDrop(ctx context.Context, _ *userv1.LeavePetDropRequest) (*userv1.PetKeeperResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	k, err := h.pets.LeaveDrop(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.PetKeeperResponse{Keeper: toProtoPetKeeper(k, time.Now())}, nil
}

func (h *UserHandler) BuyPetItem(ctx context.Context, req *userv1.BuyPetItemRequest) (*userv1.PetCareResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	item, err := user.NewPetItem(req.GetItem())
	if err != nil {
		return nil, err
	}

	care, err := h.pets.BuyItem(ctx, userID, petID, item)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(care.Pet, time.Now()), Balance: care.Balance}, nil
}

func (h *UserHandler) DressPet(ctx context.Context, req *userv1.DressPetRequest) (*userv1.PetCareResponse, error) {
	userID, petID, err := petCaller(ctx, req.GetPetId())
	if err != nil {
		return nil, err
	}

	item, err := user.NewPetItem(req.GetItem())
	if err != nil {
		return nil, err
	}

	p, err := h.pets.Dress(ctx, userID, petID, item, req.GetWear())
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, time.Now())}, nil
}

func (h *UserHandler) ListPetFights(ctx context.Context, _ *userv1.ListPetFightsRequest) (*userv1.ListPetFightsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	fights, err := h.pets.Fights(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListPetFightsResponse{}
	for _, f := range fights {
		resp.Fights = append(resp.Fights, toProtoPetFight(f))
	}

	return resp, nil
}

func (h *UserHandler) GetPetSettings(ctx context.Context, _ *userv1.GetPetSettingsRequest) (*userv1.GetPetSettingsResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := h.pets.Settings(ctx, adminID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetPetSettingsResponse{
		Shop: toProtoPetShop(s.Shop), Pets: s.Stats.Pets, Fights: s.Stats.Fights, Spent: s.Stats.Spent, Won: s.Stats.Won,
	}, nil
}

func (h *UserHandler) SetPetSettings(ctx context.Context, req *userv1.SetPetSettingsRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	shop, err := fromProtoPetShop(req.GetShop())
	if err != nil {
		return nil, err
	}

	if err = h.pets.SetShop(ctx, adminID, shop); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
