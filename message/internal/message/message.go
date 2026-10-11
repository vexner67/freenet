// Package message is the domain of private messages: what two people
// write to each other and how far each of them has read.
package message

import (
	"bytes"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxTextLen is as long as a post of the wall may be, openvk's 60000;
	// openvk itself does not limit a message at all.
	maxTextLen = 60000
	maxPhotos  = 10

	// deleteWindow is how long the sender may take a message back from
	// both sides, as "delete for everyone" in VK: long enough to take back
	// a message sent in haste, short enough that the peer can rely on what
	// was said before.
	deleteWindow = 24 * time.Hour

	// editWindow is how long the sender may change what a message says,
	// as in VK: a day, as with taking it back.
	editWindow = 24 * time.Hour

	// maxQueryLen bounds what messages are searched for.
	maxQueryLen = 100

	// MaxForwarded is how many messages one message may carry on, counting
	// those the forwarded messages carried themselves.
	MaxForwarded = 20
)

var (
	// ErrTextEmpty: neither text nor photos. The message keeps the words
	// it had before messages took photos: clients know it.
	ErrTextEmpty      = errors.New("message text is empty")
	ErrTextTooLong    = errors.New("message text is too long")
	ErrToYourself     = errors.New("you cannot write to yourself")
	ErrNotFound       = errors.New("message not found")
	ErrTooManyPhotos  = errors.New("a message has at most 10 photos")
	ErrDuplicatePhoto = errors.New("the same photo is attached twice")
	ErrInvalidPhoto   = errors.New("invalid photo id")
	// ErrSharedPhotoAlone: a photo of an album is shared with words only.
	ErrSharedPhotoAlone = errors.New("a shared photo goes with text alone")
	ErrCannotDelete     = errors.New("only the sender can delete the message")
	ErrDeleteExpired    = errors.New("the message can no longer be deleted")
	ErrCannotEdit       = errors.New("only the sender can edit the message")
	ErrEditExpired      = errors.New("the message can no longer be edited")
	ErrQueryEmpty       = errors.New("search query is empty")
	ErrQueryTooLong     = errors.New("search query is too long")
	// ErrForwardNotFound: a message to forward is not in the dialogs of
	// whoever forwards it, or was taken back.
	ErrForwardNotFound  = errors.New("forwarded message not found")
	ErrTooManyForwarded = errors.New("a message forwards at most 20 messages")
)

// Query is what messages are searched for: trimmed text they say, in any
// case, never empty.
type Query string

func NewQuery(s string) (Query, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrQueryEmpty
	}

	if utf8.RuneCountInString(s) > maxQueryLen {
		return "", ErrQueryTooLong
	}

	return Query(s), nil
}

func (q Query) String() string {
	return string(q)
}

// Matches tells whether m says q.
func (q Query) Matches(m *Message) bool {
	return strings.Contains(strings.ToLower(m.content.text.String()), strings.ToLower(string(q)))
}

// Text is what a message says: trimmed, with "\n" line breaks. It may be
// empty when the message has photos (see NewContent).
type Text string

func NewText(s string) (Text, error) {
	// Normalise line breaks so length and storage do not depend on the client.
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if utf8.RuneCountInString(s) > maxTextLen {
		return "", ErrTextTooLong
	}

	return Text(s), nil
}

func (t Text) String() string {
	return string(t)
}

// Excerpt is the beginning of the text, at most limit characters, and
// whether it was cut: lists carry it instead of a text of thousands of
// characters. Without a limit the text is whole.
func (t Text) Excerpt(limit int) (string, bool) {
	if limit <= 0 || len(t) <= limit {
		return string(t), false
	}

	n := 0
	for i := range string(t) {
		if n == limit {
			return string(t[:i]), true
		}
		n++
	}

	return string(t), false
}

// ExcerptAround is the part of the text, at most limit characters, where
// it first says query, whatever the case, and whether it was cut: a found
// message shows why it was found. It is the beginning when the words are
// there, or are not said at all.
func (t Text) ExcerptAround(query string, limit int) (string, bool) {
	runes := []rune(string(t))
	if limit <= 0 || len(runes) <= limit {
		return string(t), false
	}

	words := []rune(query)

	at := indexFold(runes, words)
	if at < 0 || at+len(words) <= limit {
		return string(runes[:limit]), true
	}

	// A quarter of the excerpt leads to the words, less when they would
	// not fit after it.
	lead := max(min(limit/4, limit-len(words)), 0)
	start := min(at-lead, len(runes)-limit)

	return string(runes[start : start+limit]), true
}

// indexFold is where text first says words, whatever the case; -1 when
// it does not.
func indexFold(text, words []rune) int {
	if len(words) == 0 {
		return -1
	}

	for i := 0; i+len(words) <= len(text); i++ {
		if equalFold(text[i:i+len(words)], words) {
			return i
		}
	}

	return -1
}

func equalFold(a, b []rune) bool {
	for i := range a {
		if unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}

	return true
}

// Photos are the IDs of a message's photos in the media service, in the
// order they are shown. Whose photos they are the gateway checks with the
// media service when the message is sent.
type Photos []uuid.UUID

func NewPhotos(ids []uuid.UUID) (Photos, error) {
	if len(ids) > maxPhotos {
		return nil, ErrTooManyPhotos
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidPhoto
		}
		if seen[id] {
			return nil, ErrDuplicatePhoto
		}
		seen[id] = true
	}

	if len(ids) == 0 {
		return nil, nil
	}

	return Photos(ids), nil
}

// Forwarded is a message carried on by another ("Переслать" of VK): who
// wrote it, when, and what it showed then. It is a copy: the message it
// was made of may change or go, and whoever reads it need not be a side
// of that dialog. Its photos are copies of the sender's in the media
// service.
type Forwarded struct {
	SenderID uuid.UUID
	Text     Text
	Photos   Photos
	Post     uuid.UUID
	// SharedPhoto is a photo of an album it shared, uuid.Nil for none.
	SharedPhoto uuid.UUID
	CreatedAt   time.Time
}

// Forwards are the messages a message carries on, oldest first.
type Forwards []Forwarded

// NewForwards: none is nil.
func NewForwards(carried []Forwarded) (Forwards, error) {
	if len(carried) > MaxForwarded {
		return nil, ErrTooManyForwarded
	}

	if len(carried) == 0 {
		return nil, nil
	}

	return Forwards(carried), nil
}

// Content is what a message shows: text, photos, a post shared into the
// dialog (openvk's "Поделиться" "в переписку"), messages carried on from
// other dialogs or some of them, or a photo of an album shared into the
// dialog with text, never nothing. The post and the photo of an album
// live in the post service: the message keeps the ID, and a deleted one
// stays "Запись удалена." or "Фотография удалена.".
type Content struct {
	text      Text
	photos    Photos
	post      uuid.UUID
	forwarded Forwards
	// sharedPhoto is a photo of an album shared ("Поделиться" under a
	// photo), uuid.Nil for none; only with text.
	sharedPhoto uuid.UUID
}

// NewContent: post is uuid.Nil for none; whether it exists the caller has
// checked with the post service.
func NewContent(text Text, photos Photos, post uuid.UUID, forwarded Forwards) (Content, error) {
	if text == "" && len(photos) == 0 && post == uuid.Nil && len(forwarded) == 0 {
		return Content{}, ErrTextEmpty
	}

	return Content{text: text, photos: photos, post: post, forwarded: forwarded}, nil
}

// NewPhotoShare is the content of a message sharing a photo of an album,
// with what the sender says about it, if anything; whether the photo
// exists the caller has checked with the post service.
func NewPhotoShare(text Text, photoID uuid.UUID) (Content, error) {
	if photoID == uuid.Nil {
		return Content{}, ErrInvalidPhoto
	}

	return Content{text: text, sharedPhoto: photoID}, nil
}

// WithSharedPhoto is c sharing photoID, restored from storage.
func (c Content) WithSharedPhoto(photoID uuid.UUID) Content {
	c.sharedPhoto = photoID
	return c
}

// SharedPhoto is the photo of an album the message shares, uuid.Nil for
// none.
func (c Content) SharedPhoto() uuid.UUID {
	return c.sharedPhoto
}

// RestoreContent rebuilds content from storage without validation; a
// message taken back has none.
func RestoreContent(text Text, photos Photos, post uuid.UUID, forwarded Forwards) Content {
	return Content{text: text, photos: photos, post: post, forwarded: forwarded}
}

// Forwarded are the messages carried on, nil for none.
func (c Content) Forwarded() Forwards {
	return c.forwarded
}

// Post is the post shared into the dialog, uuid.Nil for none.
func (c Content) Post() uuid.UUID {
	return c.post
}

func (c Content) Text() Text {
	return c.text
}

func (c Content) Photos() Photos {
	return c.photos
}

// Message does not change once made: Edited and HiddenFrom return a new
// one, so readers may keep the pointers they got.
type Message struct {
	id          uuid.UUID
	senderID    uuid.UUID
	recipientID uuid.UUID
	content     Content
	createdAt   time.Time
	// editedAt is zero for a message never edited.
	editedAt time.Time
	// deletedAt is when the sender took the message back from both sides
	// ("Удалить для всех"), zero if never: it stays in the dialog saying
	// nothing, "Сообщение удалено".
	deletedAt time.Time
	// hiddenFromSender and hiddenFromRecipient: that side deleted the
	// message for themselves ("Удалить у себя"); the other still has it.
	hiddenFromSender    bool
	hiddenFromRecipient bool
}

// New creates a message. Its UUIDv7 ID grows with time, so a dialog is
// ordered and paged by ID, and "read up to" is one ID.
func New(senderID, recipientID uuid.UUID, content Content, now time.Time) (*Message, error) {
	if senderID == recipientID {
		return nil, ErrToYourself
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Message{
		id:          id,
		senderID:    senderID,
		recipientID: recipientID,
		content:     content,
		// PostgreSQL keeps microseconds; trim now so a message reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Stored is a message as storage keeps it.
type Stored struct {
	ID          uuid.UUID
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	Content     Content
	CreatedAt   time.Time
	// EditedAt is zero for a message never edited.
	EditedAt time.Time
	// DeletedAt is zero for a message never taken back.
	DeletedAt           time.Time
	HiddenFromSender    bool
	HiddenFromRecipient bool
}

// Restore rebuilds a message from storage without validation or new IDs.
func Restore(s Stored) *Message {
	return &Message{
		id:                  s.ID,
		senderID:            s.SenderID,
		recipientID:         s.RecipientID,
		content:             s.Content,
		createdAt:           s.CreatedAt,
		editedAt:            s.EditedAt,
		deletedAt:           s.DeletedAt,
		hiddenFromSender:    s.HiddenFromSender,
		hiddenFromRecipient: s.HiddenFromRecipient,
	}
}

// Between tells whether userID is one side of the message.
func (m *Message) Between(userID uuid.UUID) bool {
	return userID == m.senderID || userID == m.recipientID
}

// VisibleTo tells whether userID still has the message in their dialog:
// one of its sides who has not deleted it for themselves.
func (m *Message) VisibleTo(userID uuid.UUID) bool {
	switch userID {
	case m.senderID:
		return !m.hiddenFromSender
	case m.recipientID:
		return !m.hiddenFromRecipient
	default:
		return false
	}
}

// HiddenFrom returns m deleted for userID alone ("Удалить у себя"): the
// other side keeps it. ErrNotFound for whoever does not have it.
func (m *Message) HiddenFrom(userID uuid.UUID) (*Message, error) {
	if !m.VisibleTo(userID) {
		return nil, ErrNotFound
	}

	hidden := *m
	if userID == m.senderID {
		hidden.hiddenFromSender = true
	} else {
		hidden.hiddenFromRecipient = true
	}

	return &hidden, nil
}

// Gone tells whether both sides have deleted the message for themselves:
// nobody has it anymore.
func (m *Message) Gone() bool {
	return m.hiddenFromSender && m.hiddenFromRecipient
}

// EditedAt is zero for a message never edited.
func (m *Message) EditedAt() time.Time {
	return m.editedAt
}

// EditableUntil is when the sender can no longer edit the message.
func (m *Message) EditableUntil() time.Time {
	return m.createdAt.Add(editWindow)
}

// Edited returns m saying text instead, edited by requesterID at now:
// only its sender, for editWindow after sending. Its photos, the post or
// the photo of an album it shares and the messages it carries on stay; without them the text may
// not be empty.
func (m *Message) Edited(requesterID uuid.UUID, text Text, now time.Time) (*Message, error) {
	if requesterID != m.senderID {
		return nil, ErrCannotEdit
	}

	if m.IsDeleted() {
		return nil, ErrNotFound
	}

	if now.After(m.EditableUntil()) {
		return nil, ErrEditExpired
	}

	var (
		content Content
		err     error
	)
	if m.content.sharedPhoto != uuid.Nil {
		// It shares the photo still: the words are what changes.
		content, err = NewPhotoShare(text, m.content.sharedPhoto)
	} else {
		content, err = NewContent(text, m.content.photos, m.content.post, m.content.forwarded)
	}
	if err != nil {
		return nil, err
	}

	edited := *m
	edited.content = content
	edited.editedAt = now.UTC().Truncate(time.Microsecond)

	return &edited, nil
}

func (m *Message) ID() uuid.UUID {
	return m.id
}

func (m *Message) SenderID() uuid.UUID {
	return m.senderID
}

func (m *Message) RecipientID() uuid.UUID {
	return m.recipientID
}

func (m *Message) Text() Text {
	return m.content.text
}

func (m *Message) Photos() Photos {
	return m.content.photos
}

// Post is the post shared in the message, uuid.Nil for none.
func (m *Message) Post() uuid.UUID {
	return m.content.post
}

// SharedPhoto is the photo of an album shared in the message, uuid.Nil
// for none.
func (m *Message) SharedPhoto() uuid.UUID {
	return m.content.sharedPhoto
}

func (m *Message) CreatedAt() time.Time {
	return m.createdAt
}

// DeletableUntil is when the sender can no longer delete the message.
func (m *Message) DeletableUntil() time.Time {
	return m.createdAt.Add(deleteWindow)
}

// CheckDelete tells whether requesterID may delete the message, from both
// sides, at now: only its sender, for deleteWindow after sending.
func (m *Message) CheckDelete(requesterID uuid.UUID, now time.Time) error {
	if requesterID != m.senderID {
		return ErrCannotDelete
	}

	if now.After(m.DeletableUntil()) {
		return ErrDeleteExpired
	}

	return nil
}

// TakenBack returns m taken back from both sides by requesterID at now
// ("Удалить для всех"), if they may (CheckDelete): it stays where it was
// in the dialog and says nothing anymore, so the peer sees that
// something was there. ErrNotFound for one taken back already.
func (m *Message) TakenBack(requesterID uuid.UUID, now time.Time) (*Message, error) {
	if err := m.CheckDelete(requesterID, now); err != nil {
		return nil, err
	}

	if m.IsDeleted() {
		return nil, ErrNotFound
	}

	gone := *m
	gone.content = Content{}
	gone.editedAt = time.Time{}
	gone.deletedAt = now.UTC().Truncate(time.Microsecond)

	return &gone, nil
}

// IsDeleted tells whether the sender took the message back.
func (m *Message) IsDeleted() bool {
	return !m.deletedAt.IsZero()
}

// DeletedAt is zero for a message never taken back.
func (m *Message) DeletedAt() time.Time {
	return m.deletedAt
}

// Forwarded are the messages m carries on, nil for none.
func (m *Message) Forwarded() Forwards {
	return m.content.forwarded
}

// Carried is m as another message carries it on: what it says itself,
// if anything, then the messages it carries on, so that forwarding a
// forward does not nest. A message taken back carries nothing. The photos
// are still those of m: whoever forwards copies them.
func (m *Message) Carried() []Forwarded {
	var carried []Forwarded

	if m.content.text != "" || len(m.content.photos) > 0 || m.content.post != uuid.Nil || m.content.sharedPhoto != uuid.Nil {
		carried = append(carried, Forwarded{
			SenderID:    m.senderID,
			Text:        m.content.text,
			Photos:      m.content.photos,
			Post:        m.content.post,
			SharedPhoto: m.content.sharedPhoto,
			CreatedAt:   m.createdAt,
		})
	}

	return append(carried, m.content.forwarded...)
}

// ReadMarks are how far each side of a dialog has read: Mine is the newest
// message from the peer I have read, Theirs the newest of mine the peer
// has read; uuid.Nil is nothing read.
type ReadMarks struct {
	Mine   uuid.UUID
	Theirs uuid.UUID
}

// IsRead tells whether m has been read by its recipient; me is the user
// whose dialog this is.
func (r ReadMarks) IsRead(m *Message, me uuid.UUID) bool {
	mark := r.Mine
	if m.SenderID() == me {
		mark = r.Theirs
	}

	id := m.ID()

	return bytes.Compare(id[:], mark[:]) <= 0
}

// Dialog is one user's view of their conversation with PeerID.
type Dialog struct {
	PeerID      uuid.UUID
	LastMessage *Message
	// LastIncomingID is the newest message from the peer; uuid.Nil if the
	// peer has not written yet.
	LastIncomingID uuid.UUID
	Marks          ReadMarks
}

// HasUnread tells whether the peer wrote something the user has not read.
func (d Dialog) HasUnread() bool {
	return bytes.Compare(d.LastIncomingID[:], d.Marks.Mine[:]) > 0
}
