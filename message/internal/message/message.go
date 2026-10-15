// Package message is the domain of private messages: what two people
// write to each other and how far each of them has read.
package message

import (
	"bytes"
	"errors"
	"slices"
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
	// ErrEditConflict: the message says something else than the edit
	// started from; another edit was first.
	ErrEditConflict = errors.New("the message was changed")
	ErrQueryEmpty   = errors.New("search query is empty")
	ErrQueryTooLong = errors.New("search query is too long")
	// ErrForwardNotFound: a message to forward is not in the dialogs of
	// whoever forwards it, or was taken back.
	ErrForwardNotFound  = errors.New("forwarded message not found")
	ErrTooManyForwarded = errors.New("a message forwards at most 20 messages")
	// ErrForwardsMisplaced: the depths of forwarded messages do not make
	// a tree.
	ErrForwardsMisplaced = errors.New("forwarded messages are out of place")
	// ErrNoMessages and ErrTooManyMessages: how many messages are deleted
	// at once.
	ErrNoMessages      = errors.New("no messages to delete")
	ErrTooManyMessages = errors.New("at most 100 messages are deleted at once")
)

// MaxBatch is how many messages are deleted at once.
const MaxBatch = 100

// Batch are the messages to delete at once: some, MaxBatch at most, each
// one once.
func Batch(ids []uuid.UUID) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]bool, len(ids))
	batch := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			batch = append(batch, id)
		}
	}

	if len(batch) == 0 {
		return nil, ErrNoMessages
	}

	if len(batch) > MaxBatch {
		return nil, ErrTooManyMessages
	}

	return batch, nil
}

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

// MatchesChat tells whether a message a person wrote in a chat says q,
// itself or in a message it carries on: what happened there and a message
// taken back say nothing to search. Messages of dialogs are sealed, and
// only the browsers of their people can search them.
func (q Query) MatchesChat(m *ChatMessage) bool {
	return !m.IsAction() && !m.IsDeleted() && q.said(m.content)
}

func (q Query) said(c Content) bool {
	if q.saidIn(c.text) {
		return true
	}

	for _, f := range c.forwarded {
		if q.saidIn(f.Text) {
			return true
		}
	}

	return false
}

func (q Query) saidIn(text Text) bool {
	return strings.Contains(strings.ToLower(text.String()), strings.ToLower(string(q)))
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
// characters. It ends after a whole word and not inside an address, when
// it can. Without a limit the text is whole.
func (t Text) Excerpt(limit int) (string, bool) {
	if limit <= 0 || len(t) <= limit {
		return string(t), false
	}

	runes := []rune(string(t))
	if len(runes) <= limit {
		return string(t), false
	}

	return string(runes[:wholeEnd(runes, limit, 0)]), true
}

// excerptSlack is how far an excerpt gives way to end well: back to the
// space before the word it would cut, or to the start of an address
// ("[ann|Анна]") it would end inside. A longer word is cut where it is.
const excerptSlack = 100

// wholeEnd is where an excerpt of runes, at most limit of them and fewer
// than there are, ends: after a whole word and outside an address, when
// one is near enough, and never before keep.
func wholeEnd(runes []rune, limit, keep int) int {
	end := limit
	floor := max(limit-excerptSlack, keep)

	if !unicode.IsSpace(runes[end]) {
		for end > floor && !unicode.IsSpace(runes[end-1]) {
			end--
		}

		if end == 0 || !unicode.IsSpace(runes[end-1]) {
			end = limit
		}
	}

	if open := addressStart(runes, end); open >= keep {
		end = open
	}

	for end > keep && unicode.IsSpace(runes[end-1]) {
		end--
	}

	if end == 0 {
		return limit
	}

	return end
}

// addressStart is where the address that end falls inside of begins: its
// bracket was opened shortly before and is closed shortly after, on the
// same line. It is -1 when end is inside none.
func addressStart(runes []rune, end int) int {
	open := -1

	for i := end - 1; i >= max(end-excerptSlack, 0) && open < 0; i-- {
		switch runes[i] {
		case ']', '\n':
			return -1
		case '[':
			open = i
		}
	}

	if open < 0 {
		return -1
	}

	for i := end; i < min(end+excerptSlack, len(runes)); i++ {
		switch runes[i] {
		case ']':
			return open
		case '[', '\n':
			return -1
		}
	}

	return -1
}

// ExcerptAround is the part of the text, at most limit characters, where
// it first says query, whatever the case, and whether it was cut: a found
// message shows why it was found. It is the beginning when the words are
// there, or are not said at all. Like Excerpt it begins and ends with a
// whole word when it can, never losing the words it was found by.
func (t Text) ExcerptAround(query string, limit int) (string, bool) {
	runes := []rune(string(t))
	if limit <= 0 || len(runes) <= limit {
		return string(t), false
	}

	words := []rune(query)

	at := indexFold(runes, words)
	if at < 0 {
		return string(runes[:wholeEnd(runes, limit, 0)]), true
	}

	if at+len(words) <= limit {
		return string(runes[:wholeEnd(runes, limit, at+len(words))]), true
	}

	// A quarter of the excerpt leads to the words, less when they would
	// not fit after it.
	lead := max(min(limit/4, limit-len(words)), 0)
	start := min(at-lead, len(runes)-limit)
	rest := runes[start:]
	end := len(rest)

	if end > limit {
		end = wholeEnd(rest, limit, min(at-start+len(words), limit))
	}

	return string(runes[wholeStart(runes, start, at) : start+end]), true
}

// wholeStart is where an excerpt that would begin at start, amid a word,
// begins instead: at the next word, when one starts shortly after and
// not after keep.
func wholeStart(runes []rune, start, keep int) int {
	if start == 0 || unicode.IsSpace(runes[start-1]) {
		return start
	}

	next := start
	ceil := min(start+excerptSlack, keep)

	for next < ceil && !unicode.IsSpace(runes[next]) {
		next++
	}

	if !unicode.IsSpace(runes[next]) {
		return start
	}

	for next < keep && unicode.IsSpace(runes[next]) {
		next++
	}

	return next
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
	// Sticker is the sticker it was, "" for none.
	Sticker Sticker
	// Documents are the documents it attached.
	Documents Documents
	CreatedAt time.Time
	// Depth is how deep it is carried: 0 for a message forwarded itself,
	// 1 for one that message carried on, and so on, as VK nests them.
	Depth int
}

// Forwards are the messages a message carries on, oldest first, as a tree
// written parent first: what a forwarded message carried on itself
// follows it, one level deeper.
type Forwards []Forwarded

// NewForwards: none is nil. No message is deeper than one level under
// the one before it, and the first is at the top.
func NewForwards(carried []Forwarded) (Forwards, error) {
	if len(carried) > MaxForwarded {
		return nil, ErrTooManyForwarded
	}

	if len(carried) == 0 {
		return nil, nil
	}

	above := -1
	for _, f := range carried {
		if f.Depth < 0 || f.Depth > above+1 {
			return nil, ErrForwardsMisplaced
		}

		above = f.Depth
	}

	return Forwards(carried), nil
}

// Content is what a message shows: text, photos, a post shared into the
// dialog (openvk's "Поделиться" "в переписку"), messages carried on from
// other dialogs, documents or some of them, or a photo of an album shared
// into the dialog with text, never nothing. The post and the photo of an album
// live in the post service: the message keeps the ID, and a deleted one
// stays "Запись удалена." or "Фотография удалена.". A message of a dialog
// says it all sealed instead: its words and those of what it carries on.
type Content struct {
	text      Text
	sealed    Sealed
	photos    Photos
	post      uuid.UUID
	forwarded Forwards
	// sharedPhoto is a photo of an album shared ("Поделиться" under a
	// photo), uuid.Nil for none; only with text.
	sharedPhoto uuid.UUID
	// sticker is the sticker the message is, "" for none; nothing goes
	// with it.
	sticker Sticker
	// documents are files of "Мои Документы" the message attaches.
	documents Documents
	// replyTo is the message it answers, uuid.Nil for none.
	replyTo uuid.UUID
}

// NewContent: post is uuid.Nil for none; whether it exists the caller has
// checked with the post service.
func NewContent(text Text, photos Photos, post uuid.UUID, forwarded Forwards) (Content, error) {
	return newContent(text, "", photos, post, forwarded)
}

// NewSealedContent is the content of a message of a dialog: what it says
// is sealed, and the words of the messages it carries on are sealed with
// it, so their copies here say nothing.
func NewSealedContent(sealed Sealed, photos Photos, post uuid.UUID, forwarded Forwards) (Content, error) {
	return newContent("", sealed, photos, post, forwarded.wordless())
}

func newContent(text Text, sealed Sealed, photos Photos, post uuid.UUID, forwarded Forwards) (Content, error) {
	if text == "" && sealed == "" && len(photos) == 0 && post == uuid.Nil && len(forwarded) == 0 {
		return Content{}, ErrTextEmpty
	}

	return Content{text: text, sealed: sealed, photos: photos, post: post, forwarded: forwarded}, nil
}

// wordless are the forwarded messages without their words.
func (f Forwards) wordless() Forwards {
	if len(f) == 0 {
		return nil
	}

	out := slices.Clone(f)
	for i := range out {
		out[i].Text = ""
	}

	return out
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

// NewSealedPhotoShare is NewPhotoShare in a dialog: what the sender says
// about the photo is sealed.
func NewSealedPhotoShare(sealed Sealed, photoID uuid.UUID) (Content, error) {
	if photoID == uuid.Nil {
		return Content{}, ErrInvalidPhoto
	}

	return Content{sealed: sealed, sharedPhoto: photoID}, nil
}

// WithSealed is c saying sealed, restored from storage.
func (c Content) WithSealed(sealed Sealed) Content {
	c.sealed = sealed
	return c
}

// Sealed is what a message of a dialog says, "" for nothing.
func (c Content) Sealed() Sealed {
	return c.sealed
}

// plain tells whether c says something the server could read: its own
// words or those of what it carries on.
func (c Content) plain() bool {
	if c.text != "" {
		return true
	}

	for _, f := range c.forwarded {
		if f.Text != "" {
			return true
		}
	}

	return false
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

// revised is c as rev makes it: the words and the photos change, what it
// shares and carries on stays.
func (c Content) revised(rev Revision) (Content, error) {
	if c.sticker != "" {
		return Content{}, ErrStickerAlone
	}

	photos, err := rev.photos(c.photos)
	if err != nil {
		return Content{}, err
	}

	revised, err := c.revisedWith(rev, photos)

	return revised.Answering(c.replyTo), err
}

// revisedWith is c saying what rev says, with photos.
func (c Content) revisedWith(rev Revision, photos Photos) (Content, error) {
	switch {
	case c.sharedPhoto == uuid.Nil:
		revised, err := newContent(rev.Text, rev.Sealed, photos, c.post, c.forwarded)
		if errors.Is(err, ErrTextEmpty) && len(c.documents) > 0 {
			// The documents stay: they are enough.
			revised, err = Content{text: rev.Text, sealed: rev.Sealed, photos: photos}, nil
		}
		revised.documents = c.documents

		return revised, err
	case len(photos) > 0:
		return Content{}, ErrSharedPhotoAlone
	default:
		// It shares the photo still: the words are what changes.
		return Content{text: rev.Text, sealed: rev.Sealed, sharedPhoto: c.sharedPhoto, documents: c.documents}, nil
	}
}

// carried is c as another message carries it on: what senderID said at
// createdAt, then the messages it carried on, one level deeper.
func (c Content) carried(senderID uuid.UUID, createdAt time.Time) []Forwarded {
	carried := make([]Forwarded, 0, 1+len(c.forwarded))
	carried = append(carried, Forwarded{
		SenderID:    senderID,
		Text:        c.text,
		Photos:      c.photos,
		Post:        c.post,
		SharedPhoto: c.sharedPhoto,
		Sticker:     c.sticker,
		Documents:   c.documents,
		CreatedAt:   createdAt,
	})

	for _, f := range c.forwarded {
		f.Depth++
		carried = append(carried, f)
	}

	return carried
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
	// quote and reactions are read with the message, not kept in it: what
	// the message it answers says now and how people reacted to it.
	quote     *Quote
	reactions Reactions
}

// New creates a message. Its UUIDv7 ID grows with time, so a dialog is
// ordered and paged by ID, and "read up to" is one ID. It says nothing in
// plain text: see NewSealedContent.
func New(senderID, recipientID uuid.UUID, content Content, now time.Time) (*Message, error) {
	if senderID == recipientID {
		return nil, ErrToYourself
	}

	if content.plain() {
		return nil, ErrPlainText
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

// Revision is what an edit makes of a message: what it says, the photos
// it shows no more and those it shows from now on, after the ones it
// keeps. It names the changes, not the photos left: two edits made at
// once, each knowing the message as it was, both take effect.
//
// The words are not changes but the whole text, so Base, when set, is the
// text the edit started from: an edit of a message that says something
// else by now is refused rather than left to undo the edit made first. A
// message of a dialog says Sealed instead, and BaseSealed is what it said
// sealed when the edit started.
type Revision struct {
	Text       Text
	Base       *Text
	Sealed     Sealed
	BaseSealed *Sealed
	Removed    Photos
	Added      Photos
}

// outdated tells whether the edit started from another text than now.
func (r Revision) outdated(now Text) bool {
	return r.Base != nil && *r.Base != now
}

// outdatedSealed tells the same of what a message of a dialog says.
func (r Revision) outdatedSealed(now Sealed) bool {
	return r.BaseSealed != nil && *r.BaseSealed != now
}

// photos are those of a message that showed old before the edit. A photo
// to remove that is gone already and one to add that is there are fine:
// another edit was first.
func (r Revision) photos(old Photos) (Photos, error) {
	ids := make([]uuid.UUID, 0, len(old)+len(r.Added))

	for _, id := range old {
		if !slices.Contains(r.Removed, id) {
			ids = append(ids, id)
		}
	}

	for _, id := range r.Added {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	return NewPhotos(ids)
}

// Edited returns m as rev makes it, edited by requesterID at now: only
// its sender, for editWindow after sending. The post or the photo of an
// album it shares and the messages it carries on stay; without them and
// without photos it may not say nothing. A message sharing a photo of an
// album takes no photos of its own. What it says is sealed, as when it
// was sent.
func (m *Message) Edited(requesterID uuid.UUID, rev Revision, now time.Time) (*Message, error) {
	if requesterID != m.senderID {
		return nil, ErrCannotEdit
	}

	if rev.Text != "" {
		return nil, ErrPlainText
	}

	if m.IsDeleted() {
		return nil, ErrNotFound
	}

	if now.After(m.EditableUntil()) {
		return nil, ErrEditExpired
	}

	if rev.outdatedSealed(m.content.sealed) {
		return nil, ErrEditConflict
	}

	content, err := m.content.revised(rev)
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

// Sealed is what the message says, sealed; "" for nothing.
func (m *Message) Sealed() Sealed {
	return m.content.sealed
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

// Sticker is the sticker m is, "" for none.
func (m *Message) Sticker() Sticker {
	return m.content.sticker
}

// Documents are the documents m attaches, nil for none.
func (m *Message) Documents() Documents {
	return m.content.documents
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
	gone.quote = nil
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

// Carried is m as another message carries it on: m itself, with what it
// says, if anything, then the messages it carried on, one level deeper —
// forwarding a forward nests, as on VK, and tells who forwarded what. A
// message taken back carries nothing. The photos are still those of m:
// whoever forwards copies them.
func (m *Message) Carried() []Forwarded {
	if m.IsDeleted() {
		return nil
	}

	return m.content.carried(m.senderID, m.createdAt)
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
