package grpc

import (
	"context"
	"errors"
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
	Pet(ctx context.Context, userID uuid.UUID) (*user.Pet, error)
	UserPet(ctx context.Context, nickname user.Nickname, now time.Time) (*user.Pet, uuid.UUID, error)
	Adopt(ctx context.Context, userID uuid.UUID, kind user.PetKind, name user.PetName, now time.Time) (service.PetCare, error)
	Feed(ctx context.Context, userID uuid.UUID, now time.Time) (service.PetCare, error)
	Play(ctx context.Context, userID uuid.UUID, now time.Time) (*user.Pet, error)
	Rename(ctx context.Context, userID uuid.UUID, name user.PetName) (*user.Pet, error)
	Release(ctx context.Context, userID uuid.UUID) error
	Fight(ctx context.Context, userID uuid.UUID, nickname user.Nickname, now time.Time) (service.PetFought, error)
	Fights(ctx context.Context, userID uuid.UUID) ([]user.PetFight, error)
	Settings(ctx context.Context, adminID uuid.UUID) (service.PetSettings, error)
	SetShop(ctx context.Context, adminID uuid.UUID, shop user.PetShop) error
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

	return &userv1.PetShop{Prices: prices, Meal: s.Meal(), Win: s.Win(), Fights: int32(s.Fights())} //nolint:gosec // at most user.MaxPetFightsADay
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

	return user.NewPetShop(prices, s.GetMeal(), s.GetWin(), int(s.GetFights()))
}

//nolint:gosec // the numbers of a pet are small: feelings to 100, levels to 30, stats and fights of a day
func toProtoPet(p *user.Pet, now time.Time) *userv1.Pet {
	if p == nil {
		return nil
	}

	stats, level := p.Stats(), p.Level()
	pet := &userv1.Pet{
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
	}
	if level < user.MaxPetLevel {
		pet.NextLevelExperience = user.PetLevelExperience(level + 1)
	}
	if at := p.PlayAt(); at.After(now) {
		pet.PlayAt = timestamppb.New(at)
	}

	return pet
}

//nolint:gosec // the numbers of a fight are small: health, damage, levels and rating points
func toProtoPetFight(f user.PetFight) *userv1.PetFight {
	fighter := func(pf user.PetFighter) *userv1.PetFighter {
		return &userv1.PetFighter{Kind: pf.Kind.String(), Name: pf.Name.String(), Level: int32(pf.Level), Health: int32(pf.Health)}
	}

	fight := &userv1.PetFight{
		Id:         f.ID.String(),
		AttackerId: f.AttackerID.String(),
		DefenderId: f.DefenderID.String(),
		FoughtAt:   timestamppb.New(f.At),
		Attacker:   fighter(f.Bout.Attacker),
		Defender:   fighter(f.Bout.Defender),
		Won:        f.Bout.Won,
		Rating:     int32(f.Bout.Rating),
	}
	for _, h := range f.Bout.Hits {
		fight.Hits = append(fight.Hits, &userv1.PetHit{
			ByAttacker: h.ByAttacker, Dodged: h.Dodged, Crit: h.Crit, Damage: int32(h.Damage),
			AttackerHealth: int32(h.AttackerHealth), DefenderHealth: int32(h.DefenderHealth),
		})
	}

	return fight
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

	p, err := h.pets.Pet(ctx, userID)
	switch {
	case err == nil:
		return &userv1.PetState{Shop: toProtoPetShop(shop), Pet: toProtoPet(p, time.Now())}, nil
	case errors.Is(err, user.ErrPetNotFound):
		return &userv1.PetState{Shop: toProtoPetShop(shop)}, nil
	default:
		return nil, err
	}
}

func (h *UserHandler) GetUserPet(ctx context.Context, req *userv1.GetUserPetRequest) (*userv1.GetUserPetResponse, error) {
	nickname, err := pageOwner(req.GetNickname())
	if err != nil {
		return nil, err
	}

	now := time.Now()

	p, ownerID, err := h.pets.UserPet(ctx, nickname, now)
	if err != nil {
		return nil, err
	}

	return &userv1.GetUserPetResponse{Pet: toProtoPet(p, now), OwnerId: ownerID.String()}, nil
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

func (h *UserHandler) FeedPet(ctx context.Context, _ *userv1.FeedPetRequest) (*userv1.PetCareResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	care, err := h.pets.Feed(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(care.Pet, now), Balance: care.Balance}, nil
}

func (h *UserHandler) PlayPet(ctx context.Context, _ *userv1.PlayPetRequest) (*userv1.PetCareResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	p, err := h.pets.Play(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, now)}, nil
}

func (h *UserHandler) RenamePet(ctx context.Context, req *userv1.RenamePetRequest) (*userv1.PetCareResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	name, err := user.NewPetName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.pets.Rename(ctx, userID, name)
	if err != nil {
		return nil, err
	}

	return &userv1.PetCareResponse{Pet: toProtoPet(p, time.Now())}, nil
}

func (h *UserHandler) ReleasePet(ctx context.Context, _ *userv1.ReleasePetRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err = h.pets.Release(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) FightPet(ctx context.Context, req *userv1.FightPetRequest) (*userv1.FightPetResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var nickname user.Nickname
	if req.GetNickname() != "" {
		if nickname, err = pageOwner(req.GetNickname()); err != nil {
			return nil, err
		}
	}

	now := time.Now()

	fought, err := h.pets.Fight(ctx, userID, nickname, now)
	if err != nil {
		return nil, err
	}

	return &userv1.FightPetResponse{Fight: toProtoPetFight(fought.Fight), Pet: toProtoPet(fought.Pet, now), Balance: fought.Balance}, nil
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
