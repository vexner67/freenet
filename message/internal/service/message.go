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

type MessageService struct {
	messageRepo MessageRepository
	dialogRepo  DialogRepository
	now         func() time.Time
}

func NewMessageService(messageRepo MessageRepository, dialogRepo DialogRepository) *MessageService {
	return &MessageService{
		messageRepo: messageRepo,
		dialogRepo:  dialogRepo,
		now:         time.Now,
	}
}

// Send writes from senderID to recipientID; the caller has made sure the
// recipient exists.
func (s *MessageService) Send(ctx context.Context, senderID, recipientID uuid.UUID, text message.Text) (*message.Message, error) {
	m, err := message.New(senderID, recipientID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("message.New", err)
	}

	if err = s.messageRepo.Save(ctx, m); err != nil {
		return nil, errs.Wrap("s.messageRepo.Save", err)
	}

	return m, nil
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

// MarkRead reads the dialog up to upTo, a message from the peer to the
// user: an ID made up "in the future" would mark every message still to
// come as read.
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

	return nil
}

func pageSize(size int) int {
	if size <= 0 || size > MaxPageSize {
		return DefaultPageSize
	}

	return size
}
