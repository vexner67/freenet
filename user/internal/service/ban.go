package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// BanRepository blocks a page or lets it be with the zero user.Ban;
// user.ErrNotFound for an unknown one.
type BanRepository interface {
	SetBan(ctx context.Context, userID uuid.UUID, ban user.Ban) error
}

// BanService is openvk's "Заблокировать пользователя": the administrators
// of the site block a page for a while or for good. A blocked page shows
// others only its name, photo and the block, and its owner, once their
// access token is renewed, only why it was blocked.
type BanService struct {
	userRepo UserRepository
	bans     BanRepository
	admins   user.Admins
	now      func() time.Time
}

func NewBanService(users UserRepository, bans BanRepository, admins user.Admins) *BanService {
	return &BanService{userRepo: users, bans: bans, admins: admins, now: time.Now}
}

// Ban blocks userID until a time, or for good with the zero time:
// user.ErrNotAdmin unless adminID runs the site, user.ErrBanSelf,
// user.ErrBanAdmin for an administrator, user.ErrBanUntilPast for a time
// gone. A blocked page is blocked anew.
func (s *BanService) Ban(ctx context.Context, adminID, userID uuid.UUID, reason user.BanReason, until time.Time) error {
	target, err := s.target(ctx, adminID, userID)
	if err != nil {
		return err
	}

	if err = target.CheckBanBy(adminID, s.admins.Has(target)); err != nil {
		return errs.Wrap("target.CheckBanBy", err)
	}

	ban, err := user.NewBan(reason, s.now(), until)
	if err != nil {
		return errs.Wrap("user.NewBan", err)
	}

	return errs.Wrap("s.bans.SetBan", s.bans.SetBan(ctx, userID, ban))
}

// Unban lets userID be; nothing happens to a page that is not blocked.
func (s *BanService) Unban(ctx context.Context, adminID, userID uuid.UUID) error {
	if _, err := s.target(ctx, adminID, userID); err != nil {
		return err
	}

	return errs.Wrap("s.bans.SetBan", s.bans.SetBan(ctx, userID, user.Ban{}))
}

// target is the page userID once adminID is known to run the site.
func (s *BanService) target(ctx context.Context, adminID, userID uuid.UUID) (*user.User, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, errs.Wrap("checkAdmin", err)
	}

	target, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, errs.Wrap("s.userRepo.GetByID", err)
	}

	return target, nil
}
