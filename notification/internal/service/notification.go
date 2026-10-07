package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxUnreadCount bounds counting: the menu shows "99+" beyond it, and
	// nobody needs the exact number of a thousand unread notifications.
	MaxUnreadCount = 100
)

type NotificationRepository interface {
	// Save is idempotent: an event already notified (the same like, comment
	// or follow) is not saved twice.
	Save(ctx context.Context, n *notification.Notification) error
	// Get returns notification.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*notification.Notification, error)
	// Delete removes the notification of an event, if there is one.
	Delete(ctx context.Context, e notification.Event) error
	// DeleteByPost removes every notification about a post.
	DeleteByPost(ctx context.Context, postID uuid.UUID) error
	// List returns up to q.Limit notifications, newest first.
	List(ctx context.Context, q Query) ([]*notification.Notification, error)
	// CountNewer counts, up to limit, the notifications of a recipient with
	// IDs above after.
	CountNewer(ctx context.Context, recipientID, after uuid.UUID, limit int) (int, error)
}

// ReadMarkRepository keeps, per user, the newest notification read: all
// newer ones are unread, it and older ones are in the archive.
type ReadMarkRepository interface {
	// ReadUpTo returns uuid.Nil for a user who has read nothing.
	ReadUpTo(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	// MarkRead moves the mark to notificationID, never back.
	MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error
}

// Query selects the unread notifications of a recipient (IDs above
// ReadUpTo) or the archive (ReadUpTo and below), with IDs below Before.
type Query struct {
	RecipientID uuid.UUID
	Unread      bool
	ReadUpTo    uuid.UUID
	// Before is the keyset cursor; uuid.Nil starts from the newest.
	Before uuid.UUID
	Limit  int
}

// Page is one screen of notifications; NextCursor is uuid.Nil on the last.
type Page struct {
	Notifications []*notification.Notification
	NextCursor    uuid.UUID
}

// Change is what happened to a user's notifications, for live updates:
// the page refreshes its counter and, for a new one, can show it.
type Change string

const (
	ChangeNew  Change = "new"
	ChangeGone Change = "gone"
	ChangeRead Change = "read"
)

// Events delivers live updates to the subscribers on every replica
// (pubsub.Redis) or on this one (pubsub.Hub): best effort, as a subscriber
// catches up from storage after any gap.
type Events interface {
	Publish(ctx context.Context, userID uuid.UUID, c Change)
	Subscribe(userID uuid.UUID) (<-chan Change, func())
}

type NotificationService struct {
	notificationRepo NotificationRepository
	readMarkRepo     ReadMarkRepository
	events           Events
	now              func() time.Time
}

func NewNotificationService(notificationRepo NotificationRepository, readMarkRepo ReadMarkRepository, events Events) *NotificationService {
	return &NotificationService{
		notificationRepo: notificationRepo,
		readMarkRepo:     readMarkRepo,
		events:           events,
		now:              time.Now,
	}
}

// Notify records an event; a repeated event is notified once. The
// recipient hears of it at once.
func (s *NotificationService) Notify(ctx context.Context, e notification.Event) error {
	n, err := notification.New(e, s.now())
	if err != nil {
		return errs.Wrap("notification.New", err)
	}

	if err = s.notificationRepo.Save(ctx, n); err != nil {
		return errs.Wrap("s.notificationRepo.Save", err)
	}

	s.events.Publish(ctx, e.RecipientID, ChangeNew)

	return nil
}

// Retract takes back the notification of an event that was undone: a like
// taken back, a comment deleted, an unfollow. Nothing to take back is fine.
func (s *NotificationService) Retract(ctx context.Context, e notification.Event) error {
	if err := e.Validate(); err != nil {
		return errs.Wrap("e.Validate", err)
	}

	if err := s.notificationRepo.Delete(ctx, e); err != nil {
		return errs.Wrap("s.notificationRepo.Delete", err)
	}

	s.events.Publish(ctx, e.RecipientID, ChangeGone)

	return nil
}

// ForgetPost drops the notifications about a deleted post. Whose they were
// is not looked up, so nobody hears of it live: counters catch up on the
// next change or page.
func (s *NotificationService) ForgetPost(ctx context.Context, postID uuid.UUID) error {
	if err := s.notificationRepo.DeleteByPost(ctx, postID); err != nil {
		return errs.Wrap("s.notificationRepo.DeleteByPost", err)
	}

	return nil
}

// List pages the unread notifications of a user or the archive, newest
// first; cursor is the ID of the last notification of the previous page.
func (s *NotificationService) List(ctx context.Context, userID uuid.UUID, unread bool, cursor uuid.UUID, size int) (Page, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	readUpTo, err := s.readMarkRepo.ReadUpTo(ctx, userID)
	if err != nil {
		return Page{}, errs.Wrap("s.readMarkRepo.ReadUpTo", err)
	}

	// One extra notification tells whether there is a next page.
	found, err := s.notificationRepo.List(ctx, Query{
		RecipientID: userID,
		Unread:      unread,
		ReadUpTo:    readUpTo,
		Before:      cursor,
		Limit:       size + 1,
	})
	if err != nil {
		return Page{}, errs.Wrap("s.notificationRepo.List", err)
	}

	page := Page{Notifications: found}

	if len(found) > size {
		page.Notifications = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
}

// CountUnread counts up to MaxUnreadCount unread notifications.
func (s *NotificationService) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	readUpTo, err := s.readMarkRepo.ReadUpTo(ctx, userID)
	if err != nil {
		return 0, errs.Wrap("s.readMarkRepo.ReadUpTo", err)
	}

	count, err := s.notificationRepo.CountNewer(ctx, userID, readUpTo, MaxUnreadCount)
	if err != nil {
		return 0, errs.Wrap("s.notificationRepo.CountNewer", err)
	}

	return count, nil
}

// MarkRead moves everything up to the user's notification notificationID
// to the archive. The ID must be the user's own: a made-up ID far in the
// future would otherwise hide every notification still to come.
func (s *NotificationService) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	n, err := s.notificationRepo.Get(ctx, notificationID)
	if err != nil {
		return errs.Wrap("s.notificationRepo.Get", err)
	}

	if n.Event().RecipientID != userID {
		return notification.ErrNotFound
	}

	if err = s.readMarkRepo.MarkRead(ctx, userID, notificationID); err != nil {
		return errs.Wrap("s.readMarkRepo.MarkRead", err)
	}

	// The user's other pages clear their counters.
	s.events.Publish(ctx, userID, ChangeRead)

	return nil
}

// Subscribe streams what happens to userID's notifications from now on
// until cancel, which must be called; the channel also closes when the
// subscriber falls behind.
func (s *NotificationService) Subscribe(userID uuid.UUID) (<-chan Change, func()) {
	return s.events.Subscribe(userID)
}
