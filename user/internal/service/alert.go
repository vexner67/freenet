package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// AlertRepository puts and takes the label of a page; user.ErrNotFound for
// an unknown one.
type AlertRepository interface {
	SetAlert(ctx context.Context, userID uuid.UUID, alert user.Alert) error
}

// AlertService is the orange label of openvk ("user-alert") the
// administrators of the site put on a page with words of their own, as
// openvk's administrators write it into the table: the marking of a
// foreign agent or anything else. Nobody hears of it, as on openvk.
type AlertService struct {
	userRepo UserRepository
	alerts   AlertRepository
	admins   user.Admins
}

func NewAlertService(users UserRepository, alerts AlertRepository, admins user.Admins) *AlertService {
	return &AlertService{userRepo: users, alerts: alerts, admins: admins}
}

// Set puts the label on userID or, with "", takes it; user.ErrNotAdmin
// unless adminID runs the site, user.ErrNotFound for an unknown page.
func (s *AlertService) Set(ctx context.Context, adminID, userID uuid.UUID, alert user.Alert) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.alerts.SetAlert", s.alerts.SetAlert(ctx, userID, alert))
}
