package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// TitleRepository keeps the titles of the pages.
type TitleRepository interface {
	// AddTitle keeps a title just given; user.ErrNotFound when its page is
	// gone.
	AddTitle(ctx context.Context, t *user.Title) error
	// Titles are the titles of userID, the newest first.
	Titles(ctx context.Context, userID uuid.UUID) ([]*user.Title, error)
	// DeleteTitle takes a title away; user.ErrTitleNotFound.
	DeleteTitle(ctx context.Context, id uuid.UUID) error
}

// TitleService is the titles: awards with a name and a picture the
// administrators give pages, as many as they like, and the profile shows.
type TitleService struct {
	userRepo UserRepository
	titles   TitleRepository
	admins   user.Admins
}

func NewTitleService(users UserRepository, titles TitleRepository, admins user.Admins) *TitleService {
	return &TitleService{userRepo: users, titles: titles, admins: admins}
}

// Grant gives the page nickname the title name with icon at now;
// user.ErrNotAdmin, user.ErrNotFound.
func (s *TitleService) Grant(ctx context.Context, adminID uuid.UUID, nickname user.Nickname, name user.TitleName, icon user.TitleIcon, now time.Time) (*user.Title, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, errs.Wrap("checkAdmin", err)
	}

	holder, err := s.userRepo.GetByNickname(ctx, nickname)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByNickname", err)
	}

	title, err := user.NewTitle(holder.ID(), name, icon, now)
	if err != nil {
		return nil, errs.Wrap("user.NewTitle", err)
	}

	if err = s.titles.AddTitle(ctx, title); err != nil {
		return nil, errs.Wrap("s.titles.AddTitle", err)
	}

	return title, nil
}

// Revoke takes the title id away; user.ErrNotAdmin, user.ErrTitleNotFound.
func (s *TitleService) Revoke(ctx context.Context, adminID, id uuid.UUID) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.titles.DeleteTitle", s.titles.DeleteTitle(ctx, id))
}

// UserTitles are the titles of the page nickname, the newest first;
// user.ErrNotFound for a page deleted or banned at now.
func (s *TitleService) UserTitles(ctx context.Context, nickname user.Nickname, now time.Time) ([]*user.Title, error) {
	holder, err := s.userRepo.GetByNickname(ctx, nickname)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByNickname", err)
	}

	if holder.IsDeactivated() || holder.IsBannedAt(now) {
		return nil, user.ErrNotFound
	}

	titles, err := s.titles.Titles(ctx, holder.ID())

	return titles, errs.Wrap("s.titles.Titles", err)
}
