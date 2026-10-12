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
	// ListBefore returns up to limit messages between a and b that a has
	// not deleted for themselves, with IDs below before (uuid.Nil: from
	// the newest), newest first.
	ListBefore(ctx context.Context, a, b, before uuid.UUID, limit int) ([]*message.Message, error)
	// ListAfter returns up to limit such messages with IDs above after,
	// oldest first.
	ListAfter(ctx context.Context, a, b, after uuid.UUID, limit int) ([]*message.Message, error)
	// Delete removes the message and brings both sides' dialogs back to
	// the messages they have left; message.ErrNotFound if it is gone
	// already.
	Delete(ctx context.Context, m *message.Message) error
	// Retract saves a message taken back: it says nothing from now on;
	// message.ErrNotFound if it is gone or was taken back already.
	Retract(ctx context.Context, m *message.Message) error
	// Lock is Get inside a transaction, holding the message until its
	// end: what is read is what the transaction changes.
	Lock(ctx context.Context, id uuid.UUID) (*message.Message, error)
	// Update saves what an edited message says and shows;
	// message.ErrNotFound.
	Update(ctx context.Context, m *message.Message) error
	// Hide deletes the message for userID alone and brings their side of
	// the dialog back to the messages they have left; message.ErrNotFound.
	Hide(ctx context.Context, m *message.Message, userID uuid.UUID) error
	// Search returns up to limit messages userID has that say q, in the
	// dialog with peerID or, for uuid.Nil, in all of theirs, newest first,
	// with IDs below before (uuid.Nil: from the newest).
	Search(ctx context.Context, userID, peerID uuid.UUID, q message.Query, before uuid.UUID, limit int) ([]*message.Message, error)
	// DeleteByUser removes the dialogs of a user, both sides of each, with
	// every message in them, and returns the deleted messages that had
	// photos, their own or of the messages they carried on.
	DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.Message, error)
}

// PhotoKeeper records what the media service has to do with the photos of
// messages, inside the transaction of the change: media keeps the photos
// of a sent message for good, keeps those of a forwarded one once more
// and deletes those of an erased one. Photos never sent it deletes after
// a while by itself.
type PhotoKeeper interface {
	Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	// Copy keeps the owner's published photos once more under new IDs.
	Copy(ctx context.Context, ownerID uuid.UUID, copies []PhotoCopy) error
}

// PhotoCopy: photo From is kept once more as To.
type PhotoCopy struct {
	From uuid.UUID
	To   uuid.UUID
}

// Draft is a message to send, into a dialog or a chat. Forward are the
// messages of the sender's dialogs and chats it carries on ("Переслать"); the rest is what it says itself,
// and one of them is there. SharedPhoto, a photo of an album to share,
// goes with text alone.
type Draft struct {
	Text        message.Text
	Photos      message.Photos
	Post        uuid.UUID
	SharedPhoto uuid.UUID
	Forward     []uuid.UUID
}

// content is what the draft says itself, with the messages it carries on.
func (d Draft) content(forwarded message.Forwards) (message.Content, error) {
	if d.SharedPhoto == uuid.Nil {
		content, err := message.NewContent(d.Text, d.Photos, d.Post, forwarded)
		return content, errs.Wrap("message.NewContent", err)
	}

	if len(d.Photos) > 0 || d.Post != uuid.Nil || len(forwarded) > 0 {
		return message.Content{}, message.ErrSharedPhotoAlone
	}

	content, err := message.NewPhotoShare(d.Text, d.SharedPhoto)

	return content, errs.Wrap("message.NewPhotoShare", err)
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
// message, sent or received, a read mark that moved, a message deleted or
// one that says something else now — edited, or taken back by its sender
// and saying nothing. Or what happened in a chat of theirs (Chat).
type Event struct {
	Message *message.Message
	Read    *ReadMark
	Deleted *Deleted
	Edited  *message.Message
	Chat    *ChatEvent
}

// SearchPage is one screen of found messages, newest first; NextCursor is
// uuid.Nil on the last.
type SearchPage struct {
	Messages   []*message.Message
	NextCursor uuid.UUID
}

// Deleted: message ID is gone from the dialog of whoever hears it, who
// deleted it for themselves.
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
	forwards    forwarder
	now         func() time.Time
}

// Deps are what MessageService works with. ChatMessages and ChatMembers
// are read when a message of a chat is carried on into a dialog.
type Deps struct {
	Messages     MessageRepository
	Dialogs      DialogRepository
	ChatMessages ChatMessageRepository
	ChatMembers  ChatMemberRepository
	Events       Events
	Tx           TxManager
	Photos       PhotoKeeper
}

func NewMessageService(d Deps) *MessageService {
	return &MessageService{
		messageRepo: d.Messages,
		dialogRepo:  d.Dialogs,
		events:      d.Events,
		tx:          d.Tx,
		photos:      d.Photos,
		forwards:    forwarder{messages: d.Messages, chatMessages: d.ChatMessages, members: d.ChatMembers},
		now:         time.Now,
	}
}

// Send writes from senderID to recipientID; the caller has made sure the
// recipient exists and the photos are the sender's. Media keeps the
// photos, and those of the forwarded messages once more. Both hear of
// it: the sender's other pages too.
func (s *MessageService) Send(ctx context.Context, senderID, recipientID uuid.UUID, d Draft) (*message.Message, error) {
	forwarded, copies, err := s.forwards.forwarded(ctx, senderID, d.Forward)
	if err != nil {
		return nil, errs.Wrap("s.forwards.forwarded", err)
	}

	content, err := d.content(forwarded)
	if err != nil {
		return nil, errs.Wrap("d.content", err)
	}

	m, err := message.New(senderID, recipientID, content, s.now())
	if err != nil {
		return nil, errs.Wrap("message.New", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.messageRepo.Save(ctx, m); err != nil {
			return errs.Wrap("s.messageRepo.Save", err)
		}

		return errs.Wrap("keepPhotos", keepPhotos(ctx, s.photos, senderID, m.Photos(), copies))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	s.events.Publish(ctx, senderID, Event{Message: m})
	s.events.Publish(ctx, recipientID, Event{Message: m})

	return m, nil
}

// Delete takes a message back from both sides of the dialog: only its
// sender may, for a day after sending (message.TakenBack). It stays in
// the dialog saying nothing, "Сообщение удалено", until each side deletes
// it for themselves; media deletes its photos. Both hear what it is now.
func (s *MessageService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	return s.DeleteMany(ctx, userID, []uuid.UUID{id}, true)
}

// Hide deletes a message for userID alone ("Удалить у себя"), their own
// or the peer's: the other side keeps it. Once both have, the message
// goes for good with its photos. The user's other pages hear of it.
func (s *MessageService) Hide(ctx context.Context, userID, id uuid.UUID) error {
	return s.DeleteMany(ctx, userID, []uuid.UUID{id}, false)
}

// removal is one message on its way out: what it was and what it is to
// be, taken back or hidden from whoever deletes it.
type removal struct {
	was *message.Message
	now *message.Message
}

// DeleteMany deletes the messages userID selected in their dialogs, all
// or none of them: for everyone, as Delete does, or for themselves, as
// Hide does. One the user does not have, or may not take back, fails
// them all.
func (s *MessageService) DeleteMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, forEveryone bool) error {
	ids, err := message.Batch(ids)
	if err != nil {
		return errs.Wrap("message.Batch", err)
	}

	removals := make([]removal, 0, len(ids))

	for _, id := range ids {
		r, err := s.removal(ctx, userID, id, forEveryone)
		if err != nil {
			return errs.Wrap("s.removal", err)
		}

		removals = append(removals, r)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		for _, r := range removals {
			if err := s.remove(ctx, userID, r); err != nil {
				return errs.Wrap("s.remove", err)
			}
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	for _, r := range removals {
		s.announce(ctx, userID, r)
	}

	return nil
}

// removal reads message id and tells what deleting it makes of it.
func (s *MessageService) removal(ctx context.Context, userID, id uuid.UUID, forEveryone bool) (removal, error) {
	m, err := s.messageRepo.Get(ctx, id)
	if err != nil {
		return removal{}, errs.Wrap("s.messageRepo.Get", err)
	}

	// Others' dialogs are none of the user's business, not even that a
	// message exists; nor is a message the user deleted for themselves.
	if !m.VisibleTo(userID) {
		return removal{}, message.ErrNotFound
	}

	if forEveryone {
		gone, err := m.TakenBack(userID, s.now())

		return removal{was: m, now: gone}, errs.Wrap("m.TakenBack", err)
	}

	hidden, err := m.HiddenFrom(userID)

	return removal{was: m, now: hidden}, errs.Wrap("m.HiddenFrom", err)
}

// remove saves r: a message taken back says nothing and loses its photos,
// one hidden by both sides goes for good with them, one hidden by userID
// alone stays for the other side.
func (s *MessageService) remove(ctx context.Context, userID uuid.UUID, r removal) error {
	switch {
	case r.now.IsDeleted() && !r.was.IsDeleted():
		if err := s.messageRepo.Retract(ctx, r.now); err != nil {
			return errs.Wrap("s.messageRepo.Retract", err)
		}
	case r.now.Gone():
		if err := s.messageRepo.Delete(ctx, r.was); err != nil {
			return errs.Wrap("s.messageRepo.Delete", err)
		}
	default:
		return errs.Wrap("s.messageRepo.Hide", s.messageRepo.Hide(ctx, r.was, userID))
	}

	return errs.Wrap("dropPhotos", dropPhotos(ctx, s.photos, r.was.SenderID(), r.was.Photos(), r.was.Forwarded(), uuid.Nil))
}

// announce tells of r: both sides hear what a message taken back is now,
// the pages of userID that a message they hid is gone.
func (s *MessageService) announce(ctx context.Context, userID uuid.UUID, r removal) {
	if !r.now.IsDeleted() || r.was.IsDeleted() {
		s.events.Publish(ctx, userID, Event{Deleted: &Deleted{ID: r.was.ID(), SenderID: r.was.SenderID(), RecipientID: r.was.RecipientID()}})
		return
	}

	for _, side := range []uuid.UUID{r.now.SenderID(), r.now.RecipientID()} {
		if r.now.VisibleTo(side) {
			s.events.Publish(ctx, side, Event{Edited: r.now})
		}
	}
}

// Edit changes what a message says and, if rev says so, the photos it
// shows: only its sender, for a day after sending (message.Edited). The
// caller has made sure new photos are the sender's; media keeps them and
// deletes those the edit took away. Both sides that still have the
// message hear of it. The message is read and saved in one transaction that holds it, so two edits made at
// once go one after the other, the second changing what the first left.
func (s *MessageService) Edit(ctx context.Context, userID, id uuid.UUID, rev message.Revision) (*message.Message, error) {
	var edited *message.Message

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		m, err := s.messageRepo.Lock(ctx, id)
		if err != nil {
			return errs.Wrap("s.messageRepo.Lock", err)
		}

		if !m.VisibleTo(userID) {
			return message.ErrNotFound
		}

		if edited, err = m.Edited(userID, rev, s.now()); err != nil {
			return errs.Wrap("m.Edited", err)
		}

		if err = s.messageRepo.Update(ctx, edited); err != nil {
			return errs.Wrap("s.messageRepo.Update", err)
		}

		return errs.Wrap("movePhotos", movePhotos(ctx, s.photos, userID, without(edited.Photos(), m.Photos()), without(m.Photos(), edited.Photos())))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	for _, side := range []uuid.UUID{edited.SenderID(), edited.RecipientID()} {
		if edited.VisibleTo(side) {
			s.events.Publish(ctx, side, Event{Edited: edited})
		}
	}

	return edited, nil
}

// Search pages the messages of userID that say q, newest first: in the
// dialog with peerID, or in all of theirs for uuid.Nil.
func (s *MessageService) Search(ctx context.Context, userID, peerID uuid.UUID, q message.Query, cursor uuid.UUID, size int) (SearchPage, error) {
	size = pageSize(size)

	found, err := s.messageRepo.Search(ctx, userID, peerID, q, cursor, size+1)
	if err != nil {
		return SearchPage{}, errs.Wrap("s.messageRepo.Search", err)
	}

	page := SearchPage{Messages: found}
	if len(found) > size {
		page.Messages = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
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

// Around returns the part of the dialog of userID with peerID around
// message id, which is in it: about half of size before it and half
// after, oldest first, so that a found message opens where it was said.
// NextCursor pages to older messages; newer ones come with Newer.
func (s *MessageService) Around(ctx context.Context, userID, peerID, id uuid.UUID, size int) (HistoryPage, error) {
	m, err := s.messageRepo.Get(ctx, id)
	if err != nil {
		return HistoryPage{}, errs.Wrap("s.messageRepo.Get", err)
	}

	if !m.VisibleTo(userID) || !m.Between(peerID) || userID == peerID {
		return HistoryPage{}, message.ErrNotFound
	}

	half := max(pageSize(size)/2, 1)

	older, err := s.messageRepo.ListBefore(ctx, userID, peerID, id, half+1)
	if err != nil {
		return HistoryPage{}, errs.Wrap("s.messageRepo.ListBefore", err)
	}

	newer, err := s.messageRepo.ListAfter(ctx, userID, peerID, id, half)
	if err != nil {
		return HistoryPage{}, errs.Wrap("s.messageRepo.ListAfter", err)
	}

	var page HistoryPage

	if len(older) > half {
		older = older[:half]
		page.NextCursor = older[half-1].ID()
	}

	slices.Reverse(older)
	page.Messages = append(append(older, m), newer...)

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

	if m.SenderID() != peerID || m.RecipientID() != userID || !m.VisibleTo(userID) {
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
// photos the peers sent or forwarded there go too; the user's own media
// erases with the rest of the user.
func (s *MessageService) PurgeUser(ctx context.Context, userID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		withPhotos, err := s.messageRepo.DeleteByUser(ctx, userID)
		if err != nil {
			return errs.Wrap("s.messageRepo.DeleteByUser", err)
		}

		for _, m := range withPhotos {
			if err = dropPhotos(ctx, s.photos, m.SenderID(), m.Photos(), m.Forwarded(), userID); err != nil {
				return errs.Wrap("dropPhotos", err)
			}
		}

		return nil
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}
