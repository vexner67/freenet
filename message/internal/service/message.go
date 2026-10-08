package service

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 50

	// MaxUnreadCount bounds counting: the menu shows "99+" beyond it.
	MaxUnreadCount = 100
)

type MessageRepository interface {
	// Save stores the message and brings both sides' dialogs up to it.
	Save(ctx context.Context, m *message.Message) error
	// Get returns message.ErrNotFound.
	Get(ctx context.Context, id uuid.UUID) (*message.Message, error)
	// ListBefore returns up to limit messages between a and b with IDs
	// below before (uuid.Nil: from the newest), newest first.
	ListBefore(ctx context.Context, a, b, before uuid.UUID, limit int) ([]*message.Message, error)
	// ListAfter returns up to limit messages between a and b with IDs
	// above after, oldest first.
	ListAfter(ctx context.Context, a, b, after uuid.UUID, limit int) ([]*message.Message, error)
	// Delete removes the message and brings both sides' dialogs back to
	// the messages left; message.ErrNotFound if it is gone already.
	Delete(ctx context.Context, m *message.Message) error
	// DeleteByUser removes the dialogs of a user, both sides of each, with
	// every message in them, and returns the deleted messages that had
	// photos.
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.Message, error)
}

// PhotoKeeper records what the media service has to do with the photos of
// messages, inside the transaction of the change: media keeps the photos
// of a sent message for good and deletes those of an erased one. Photos
// never sent it deletes after a while by itself.
type PhotoKeeper interface {
	Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
}

// TxManager runs fn in one transaction: everything it saves is kept if it
// returns nil and undone otherwise.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type DialogRepository interface {
	// List returns up to limit dialogs of a user whose last message is
	// below before (uuid.Nil: from the newest), most recent first.
	List(ctx context.Context, userID, before uuid.UUID, limit int) ([]message.Dialog, error)
	// CountUnread counts, up to limit, the dialogs with unread messages.
	CountUnread(ctx context.Context, userID uuid.UUID, limit int) (int, error)
	// ReadMarks are zero for two people who have not written yet.
	ReadMarks(ctx context.Context, userID, peerID uuid.UUID) (message.ReadMarks, error)
	// MarkRead moves the user's mark in the dialog to upTo, never back.
	MarkRead(ctx context.Context, userID, peerID, upTo uuid.UUID) error
	// HasIncoming tells whether peerID has ever written to userID.
	HasIncoming(ctx context.Context, userID, peerID uuid.UUID) (bool, error)
}

// DialogPage is one screen of dialogs; NextCursor is uuid.Nil on the last.
type DialogPage struct {
	Dialogs    []message.Dialog
	NextCursor uuid.UUID
}

// HistoryPage is a run of messages of one dialog, oldest first, with the
// read marks to tell which are read. NextCursor pages to older messages
// and is uuid.Nil when there are none.
type HistoryPage struct {
	Messages   []*message.Message
	Marks      message.ReadMarks
	NextCursor uuid.UUID
}

// Event is what happened in a user's dialogs, for live updates: a new
// message, sent or received, a read mark that moved or a message deleted.
type Event struct {
	Message *message.Message
	Read    *ReadMark
	Deleted *Deleted
}

// Deleted: the sender took message ID back from both sides.
type Deleted struct {
	ID          uuid.UUID
	SenderID    uuid.UUID
	RecipientID uuid.UUID
}

// ReadMark: ReaderID has read the dialog with PeerID up to UpTo.
type ReadMark struct {
	ReaderID uuid.UUID
	PeerID   uuid.UUID
	UpTo     uuid.UUID
}

// Events delivers live updates to the subscribers on every replica
// (pubsub.Redis) or on this one (pubsub.Hub): best effort, as a subscriber
// catches up from storage after any gap.
type Events interface {
	Publish(ctx context.Context, userID uuid.UUID, e Event)
	Subscribe(userID uuid.UUID) (<-chan Event, func())
}

type MessageService struct {
	messageRepo MessageRepository
	dialogRepo  DialogRepository
	events      Events
	tx          TxManager
	photos      PhotoKeeper
	now         func() time.Time
}

// Deps are what MessageService works with.
type Deps struct {
	Messages MessageRepository
	Dialogs  DialogRepository
	Events   Events
	Tx       TxManager
	Photos   PhotoKeeper
}

func NewMessageService(d Deps) *MessageService {
	return &MessageService{
		messageRepo: d.Messages,
		dialogRepo:  d.Dialogs,
		events:      d.Events,
		tx:          d.Tx,
		photos:      d.Photos,
		now:         time.Now,
	}
}

// Send writes from senderID to recipientID; the caller has made sure the
// recipient exists and the photos are the sender's. Media keeps the
// photos. Both hear of it: the sender's other pages too.
func (s *MessageService) Send(ctx context.Context, senderID, recipientID uuid.UUID, content message.Content) (*message.Message, error) {
	m, err := message.New(senderID, recipientID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("message.New", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.messageRepo.Save(ctx, m); err != nil {
			return errs.Wrap("s.messageRepo.Save", err)
		}

		if photos := m.Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Publish", s.photos.Publish(ctx, senderID, photos))
		}

		return nil
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	s.events.Publish(ctx, senderID, Event{Message: m})
	s.events.Publish(ctx, recipientID, Event{Message: m})

	return m, nil
}

// Delete takes a message back from both sides of the dialog: only its
// sender may, for a day after sending (message.CheckDelete). Media deletes
// its photos. Both hear of it: the peer's open dialog drops the message
// and the counters follow.
func (s *MessageService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	m, err := s.messageRepo.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.messageRepo.Get", err)
	}

	// Others' dialogs are none of the user's business, not even that a
	// message exists.
	if m.SenderID() != userID && m.RecipientID() != userID {
		return message.ErrNotFound
	}

	if err = m.CheckDelete(userID, s.now()); err != nil {
		return errs.Wrap("m.CheckDelete", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.messageRepo.Delete(ctx, m); err != nil {
			return errs.Wrap("s.messageRepo.Delete", err)
		}

		if photos := m.Photos(); len(photos) > 0 {
			return errs.Wrap("s.photos.Delete", s.photos.Delete(ctx, m.SenderID(), photos))
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	deleted := Event{Deleted: &Deleted{ID: m.ID(), SenderID: m.SenderID(), RecipientID: m.RecipientID()}}
	s.events.Publish(ctx, m.SenderID(), deleted)
	s.events.Publish(ctx, m.RecipientID(), deleted)

	return nil
}

// Dialogs pages a user's dialogs, the most recently written first; cursor
// is the last message of the previous page's last dialog.
func (s *MessageService) Dialogs(ctx context.Context, userID, cursor uuid.UUID, size int) (DialogPage, error) {
	size = pageSize(size)

	// One extra dialog tells whether there is a next page.
	dialogs, err := s.dialogRepo.List(ctx, userID, cursor, size+1)
	if err != nil {
		return DialogPage{}, errs.Wrap("s.dialogRepo.List", err)
	}

	page := DialogPage{Dialogs: dialogs}

	if len(dialogs) > size {
		page.Dialogs = dialogs[:size]
		page.NextCursor = dialogs[size-1].LastMessage.ID()
	}

	return page, nil
}

// CountUnread counts up to MaxUnreadCount dialogs with unread messages.
func (s *MessageService) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := s.dialogRepo.CountUnread(ctx, userID, MaxUnreadCount)
	if err != nil {
		return 0, errs.Wrap("s.dialogRepo.CountUnread", err)
	}

	return count, nil
}

// History pages the dialog of userID with peerID back in time: cursor is
// the oldest message of the previous page (uuid.Nil: from the newest).
func (s *MessageService) History(ctx context.Context, userID, peerID, cursor uuid.UUID, size int) (HistoryPage, error) {
	size = pageSize(size)

	newestFirst, err := s.messageRepo.ListBefore(ctx, userID, peerID, cursor, size+1)
	if err != nil {
		return HistoryPage{}, errs.Wrap("s.messageRepo.ListBefore", err)
	}

	page := HistoryPage{Messages: newestFirst}

	if len(newestFirst) > size {
		page.Messages = newestFirst[:size]
		page.NextCursor = newestFirst[size-1].ID()
	}

	slices.Reverse(page.Messages)

	if page.Marks, err = s.dialogRepo.ReadMarks(ctx, userID, peerID); err != nil {
		return HistoryPage{}, errs.Wrap("s.dialogRepo.ReadMarks", err)
	}

	return page, nil
}

// Newer returns up to size messages of the dialog after the given one,
// oldest first: what came since the page last looked. NextCursor stays
// uuid.Nil; the marks tell whether the peer has read the older ones too.
func (s *MessageService) Newer(ctx context.Context, userID, peerID, after uuid.UUID, size int) (HistoryPage, error) {
	messages, err := s.messageRepo.ListAfter(ctx, userID, peerID, after, pageSize(size))
	if err != nil {
		return HistoryPage{}, errs.Wrap("s.messageRepo.ListAfter", err)
	}

	marks, err := s.dialogRepo.ReadMarks(ctx, userID, peerID)
	if err != nil {
		return HistoryPage{}, errs.Wrap("s.dialogRepo.ReadMarks", err)
	}

	return HistoryPage{Messages: messages, Marks: marks}, nil
}

// HasMessagesFrom tells whether peerID has ever written to userID: whom
// the user wrote to may answer whatever their privacy, which the user
// service decides.
func (s *MessageService) HasMessagesFrom(ctx context.Context, userID, peerID uuid.UUID) (bool, error) {
	ok, err := s.dialogRepo.HasIncoming(ctx, userID, peerID)
	if err != nil {
		return false, errs.Wrap("s.dialogRepo.HasIncoming", err)
	}

	return ok, nil
}

// MarkRead reads the dialog up to upTo, a message from the peer to the
// user: an ID made up "in the future" would mark every message still to
// come as read. Both sides hear of it: the peer's messages show as read,
// the user's other pages update their counters.
func (s *MessageService) MarkRead(ctx context.Context, userID, peerID, upTo uuid.UUID) error {
	m, err := s.messageRepo.Get(ctx, upTo)
	if err != nil {
		return errs.Wrap("s.messageRepo.Get", err)
	}

	if m.SenderID() != peerID || m.RecipientID() != userID {
		return message.ErrNotFound
	}

	if err = s.dialogRepo.MarkRead(ctx, userID, peerID, upTo); err != nil {
		return errs.Wrap("s.dialogRepo.MarkRead", err)
	}

	read := Event{Read: &ReadMark{ReaderID: userID, PeerID: peerID, UpTo: upTo}}
	s.events.Publish(ctx, userID, read)
	s.events.Publish(ctx, peerID, read)

	return nil
}

// Subscribe streams userID's events from now on until cancel, which must
// be called; the channel also closes when the subscriber falls behind and
// has to catch up from storage.
func (s *MessageService) Subscribe(userID uuid.UUID) (<-chan Event, func()) {
	return s.events.Subscribe(userID)
}

func pageSize(size int) int {
	if size <= 0 || size > MaxPageSize {
		return DefaultPageSize
	}

	return size
}

// PurgeUser erases the dialogs of a user whose page was deleted for good,
// from both sides: the peers would only keep a dialog with nobody. The
// photos the peers sent there go too; the user's own media erases with the
// rest of the user.
func (s *MessageService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		withPhotos, err := s.messageRepo.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.messageRepo.DeleteByUser", err)
		}

		for _, m := range withPhotos {
			if m.SenderID() == userID {
				continue
			}

			if err = s.photos.Delete(ctx, m.SenderID(), m.Photos()); err != nil {
				return errs.Wrap("s.photos.Delete", err)
			}
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}
