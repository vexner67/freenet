package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// GiftRepository keeps the gifts sent to pages. A gift goes with the page
// it was sent to and outlives the page of its sender.
type GiftRepository interface {
	// Save returns user.ErrNotFound if the recipient or the sender is gone.
	Save(ctx context.Context, g *user.Gift) error
	// Get returns user.ErrGiftNotFound.
	Get(ctx context.Context, id uuid.UUID) (*user.Gift, error)
	// Delete is idempotent.
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit gifts sent to recipientID, newest first,
	// with IDs below before (uuid.Nil: from the newest).
	List(ctx context.Context, recipientID, before uuid.UUID, limit int) ([]*user.Gift, error)
	Count(ctx context.Context, recipientID uuid.UUID) (int, error)
}

// GiftNotifier records what the notification service has to hear about
// gifts, in the transaction of the change, as Notifier does.
type GiftNotifier interface {
	GiftSent(ctx context.Context, giftID, senderID, recipientID uuid.UUID) error
	// GiftDeleted takes back the notification of the gift.
	GiftDeleted(ctx context.Context, giftID, senderID, recipientID uuid.UUID) error
}

// GiftDraft is a gift to send.
type GiftDraft struct {
	Kind      user.GiftKind
	Caption   user.GiftCaption
	Anonymous bool
}

// SentGift is a gift as a viewer sees it. Sender is nil when the viewer
// is not told who sent it: it was sent anonymously, or the sender's page
// is deleted or erased. The words of an anonymous gift are not told
// either.
type SentGift struct {
	ID        uuid.UUID
	Kind      user.GiftKind
	Caption   user.GiftCaption
	Anonymous bool
	SentAt    time.Time
	Sender    *user.PublicProfile
}

// GiftPage is one screen of the gifts of a page, newest first; NextCursor
// is uuid.Nil on the last.
type GiftPage struct {
	Gifts      []SentGift
	NextCursor uuid.UUID
	Total      int
}

// GiftService is openvk's "Подарки": people send each other gifts of the
// catalogue, and a page shows the gifts it was sent.
type GiftService struct {
	userRepo    UserRepository
	giftRepo    GiftRepository
	permissions *PermissionService
	tx          TxManager
	notifier    GiftNotifier
	now         func() time.Time
}

// GiftDeps are what GiftService works with.
type GiftDeps struct {
	Users       UserRepository
	Gifts       GiftRepository
	Permissions *PermissionService
	Tx          TxManager
	Notifier    GiftNotifier
}

func NewGiftService(d GiftDeps) *GiftService {
	return &GiftService{
		userRepo:    d.Users,
		giftRepo:    d.Gifts,
		permissions: d.Permissions,
		tx:          d.Tx,
		notifier:    d.Notifier,
		now:         time.Now,
	}
}

// Send sends recipientID a gift from senderID; the recipient hears of it.
// user.ErrNotFound for an unknown recipient, user.ErrDeactivated and
// user.ErrGiftRefused as user.GiftFrom decides.
func (s *GiftService) Send(ctx context.Context, senderID, recipientID uuid.UUID, d GiftDraft) (SentGift, error) {
	recipient, ties, err := s.permissions.ties(ctx, senderID, recipientID)
	if err != nil {
		return SentGift{}, errs.Wrap("s.permissions.ties", err)
	}

	gift, err := recipient.GiftFrom(senderID, ties, d.Kind, d.Caption, d.Anonymous)
	if err != nil {
		return SentGift{}, errs.Wrap("recipient.GiftFrom", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.giftRepo.Save(ctx, gift); err != nil {
			return errs.Wrap("s.giftRepo.Save", err)
		}

		if !gift.Notifies() {
			return nil
		}

		return errs.Wrap("s.notifier.GiftSent", s.notifier.GiftSent(ctx, gift.ID(), senderID, recipientID))
	})
	if err != nil {
		return SentGift{}, errs.Wrap("s.tx.WithinTx", err)
	}

	shown, err := s.shown(ctx, senderID, []*user.Gift{gift})
	if err != nil {
		return SentGift{}, errs.Wrap("s.shown", err)
	}

	return shown[0], nil
}

// Delete removes a gift from the page of userID, its recipient; the
// notification of it goes too. user.ErrGiftNotFound and
// user.ErrCannotDeleteGift.
func (s *GiftService) Delete(ctx context.Context, userID, giftID uuid.UUID) error {
	gift, err := s.giftRepo.Get(ctx, giftID)
	if err != nil {
		return errs.Wrap("s.giftRepo.Get", err)
	}

	if !gift.CanBeDeletedBy(userID) {
		return user.ErrCannotDeleteGift
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.giftRepo.Delete(ctx, giftID); err != nil {
			return errs.Wrap("s.giftRepo.Delete", err)
		}

		if !gift.Notifies() {
			return nil
		}

		return errs.Wrap("s.notifier.GiftDeleted", s.notifier.GiftDeleted(ctx, giftID, gift.SenderID(), gift.RecipientID()))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// List pages the gifts sent to ownerID as viewerID (uuid.Nil: anonymous)
// sees them, newest first. user.ErrNotFound for an unknown owner and
// user.ErrGiftsHidden when the page or its gifts are not for the viewer.
func (s *GiftService) List(ctx context.Context, viewerID, ownerID, cursor uuid.UUID, size int) (GiftPage, error) {
	owner, ties, err := s.permissions.ties(ctx, viewerID, ownerID)
	if err != nil {
		return GiftPage{}, errs.Wrap("s.permissions.ties", err)
	}

	if access := owner.AccessFor(viewerID, ties); !access.Page || !access.Gifts {
		return GiftPage{}, user.ErrGiftsHidden
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra row tells whether there is a next page.
	found, err := s.giftRepo.List(ctx, ownerID, cursor, size+1)
	if err != nil {
		return GiftPage{}, errs.Wrap("s.giftRepo.List", err)
	}

	total, err := s.giftRepo.Count(ctx, ownerID)
	if err != nil {
		return GiftPage{}, errs.Wrap("s.giftRepo.Count", err)
	}

	page := GiftPage{Total: total}

	if len(found) > size {
		found = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Gifts, err = s.shown(ctx, viewerID, found); err != nil {
		return GiftPage{}, errs.Wrap("s.shown", err)
	}

	return page, nil
}

// shown is gifts as viewerID sees them, in their order, with the senders
// the viewer is told read in one go.
func (s *GiftService) shown(ctx context.Context, viewerID uuid.UUID, gifts []*user.Gift) ([]SentGift, error) {
	var senderIDs []uuid.UUID

	for _, g := range gifts {
		if g.SenderID() != uuid.Nil && g.SignedFor(viewerID) {
			senderIDs = append(senderIDs, g.SenderID())
		}
	}

	senders := make(map[uuid.UUID]*user.User, len(senderIDs))

	if len(senderIDs) > 0 {
		found, err := s.userRepo.GetByIDs(ctx, senderIDs)
		if err != nil {
			return nil, errs.Wrap("s.userRepo.GetByIDs", err)
		}

		for _, u := range found {
			senders[u.ID()] = u
		}
	}

	now := s.now()
	shown := make([]SentGift, 0, len(gifts))

	for _, g := range gifts {
		sent := SentGift{ID: g.ID(), Kind: g.Kind(), SentAt: g.SentAt()}

		if g.SignedFor(viewerID) {
			sent.Caption, sent.Anonymous = g.Caption(), g.Anonymous()

			// Who wrote a deleted page is not shown anywhere.
			if sender, ok := senders[g.SenderID()]; ok && !sender.IsDeactivated() {
				profile := sender.ListedProfile(now)
				sent.Sender = &profile
			}
		}

		shown = append(shown, sent)
	}

	return shown, nil
}
