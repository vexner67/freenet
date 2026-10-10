package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxTitleLen is openvk's: it cuts a topic title at 127 characters.
const maxTitleLen = 127

var (
	ErrTitleEmpty          = errors.New("topic title is empty")
	ErrTitleTooLong        = errors.New("topic title is too long")
	ErrTopicNotFound       = errors.New("topic not found")
	ErrCannotChangeTopic   = errors.New("only the author or the admins can change the topic")
	ErrCannotPinTopic      = errors.New("only the admins can pin the topic or restrict comments")
	ErrRestrictOwnTopic    = errors.New("only a topic of the community can be restricted")
	ErrTopicClosed         = errors.New("the topic is closed")
	ErrTopicRestricted     = errors.New("only the admins comment on this topic")
	ErrCannotDeleteMessage = errors.New("only the author or the admins can delete the message")
	ErrMessageNotFound     = errors.New("message not found")
)

// TopicTitle is what a topic is called: one line, trimmed.
type TopicTitle string

func NewTopicTitle(s string) (TopicTitle, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrTitleEmpty
	}

	if utf8.RuneCountInString(s) > maxTitleLen {
		return "", ErrTitleTooLong
	}

	return TopicTitle(s), nil
}

func (t TopicTitle) String() string {
	return string(t)
}

// Topic is a discussion of a community, openvk's "Обсуждения": a title,
// started by its author or, fromWall, by the community, and the messages
// people leave in it (TopicMessage). Its admins pin it on top of the list
// and let only themselves write in a topic of the community (restricted);
// its author and the admins close it, and then nobody writes.
type Topic struct {
	id         uuid.UUID
	clubID     uuid.UUID
	authorID   uuid.UUID
	title      TopicTitle
	fromWall   bool
	closed     bool
	pinned     bool
	restricted bool
	createdAt  time.Time
	// updatedAt is the time of the newest message, or of the topic
	// itself while it has none: the list shows the active topics first.
	updatedAt time.Time
	messages  int
}

// NewTopic starts a topic in the discussions of clubID. Whether authorID
// may start one there, and speak for the community, is the community's,
// which the caller checks.
func NewTopic(clubID, authorID uuid.UUID, title TopicTitle, fromWall bool, now time.Time) (*Topic, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	// PostgreSQL keeps microseconds; trim now so a topic reads back equal.
	now = now.UTC().Truncate(time.Microsecond)

	return &Topic{
		id:        id,
		clubID:    clubID,
		authorID:  authorID,
		title:     title,
		fromWall:  fromWall,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// StoredTopic is a topic as storage keeps it.
type StoredTopic struct {
	ID         uuid.UUID
	ClubID     uuid.UUID
	AuthorID   uuid.UUID
	Title      TopicTitle
	FromWall   bool
	Closed     bool
	Pinned     bool
	Restricted bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Messages   int
}

// RestoreTopic rebuilds a topic from storage without validation or new IDs.
func RestoreTopic(s StoredTopic) *Topic {
	return &Topic{
		id: s.ID, clubID: s.ClubID, authorID: s.AuthorID, title: s.Title, fromWall: s.FromWall,
		closed: s.Closed, pinned: s.Pinned, restricted: s.Restricted,
		createdAt: s.CreatedAt, updatedAt: s.UpdatedAt, messages: s.Messages,
	}
}

func (t *Topic) Stored() StoredTopic {
	return StoredTopic{
		ID: t.id, ClubID: t.clubID, AuthorID: t.authorID, Title: t.title, FromWall: t.fromWall,
		Closed: t.closed, Pinned: t.pinned, Restricted: t.restricted,
		CreatedAt: t.createdAt, UpdatedAt: t.updatedAt, Messages: t.messages,
	}
}

// TopicChanges are openvk's "Редактирование темы": the title and the
// settings, all at once, as the form sends them.
type TopicChanges struct {
	Title      TopicTitle
	Closed     bool
	Pinned     bool
	Restricted bool
}

// Changed returns t with changes made by requesterID, admin if they run
// the community. Its author and the admins rename and close it; only the
// admins pin it and restrict it, and only a topic of the community is
// restricted, as in openvk.
func (t *Topic) Changed(requesterID uuid.UUID, admin bool, changes TopicChanges) (*Topic, error) {
	if !t.CanBeChangedBy(requesterID, admin) {
		return nil, ErrCannotChangeTopic
	}

	if !admin && (changes.Pinned != t.pinned || changes.Restricted != t.restricted) {
		return nil, ErrCannotPinTopic
	}

	if changes.Restricted && !t.fromWall {
		return nil, ErrRestrictOwnTopic
	}

	changed := *t
	changed.title = changes.Title
	changed.closed = changes.Closed
	changed.pinned = changes.Pinned
	changed.restricted = changes.Restricted

	return &changed, nil
}

// CanBeChangedBy: the author, even of a topic of the community, or,
// admin, whoever runs the community renames, closes and deletes a topic,
// as in openvk.
func (t *Topic) CanBeChangedBy(userID uuid.UUID, admin bool) bool {
	return admin || userID == t.authorID
}

// CheckWrite tells whether someone, admin if they run the community, may
// leave a message: nobody in a closed topic, only the admins in a
// restricted one.
func (t *Topic) CheckWrite(admin bool) error {
	if t.closed {
		return ErrTopicClosed
	}

	if t.restricted && !admin {
		return ErrTopicRestricted
	}

	return nil
}

func (t *Topic) ID() uuid.UUID {
	return t.id
}

func (t *Topic) ClubID() uuid.UUID {
	return t.clubID
}

func (t *Topic) AuthorID() uuid.UUID {
	return t.authorID
}

func (t *Topic) Title() TopicTitle {
	return t.title
}

// FromWall: the topic was started by the community, which readers see
// instead of its author.
func (t *Topic) FromWall() bool {
	return t.fromWall
}

func (t *Topic) Closed() bool {
	return t.closed
}

func (t *Topic) Pinned() bool {
	return t.pinned
}

func (t *Topic) Restricted() bool {
	return t.restricted
}

func (t *Topic) CreatedAt() time.Time {
	return t.createdAt
}

func (t *Topic) UpdatedAt() time.Time {
	return t.updatedAt
}

// Messages counts the messages in the topic.
func (t *Topic) Messages() int {
	return t.messages
}

// TopicMessage is what someone writes in a topic: text, photos or both,
// as a comment says, for themselves or, fromWall, for the community.
type TopicMessage struct {
	id        uuid.UUID
	topicID   uuid.UUID
	authorID  uuid.UUID
	content   CommentContent
	fromWall  bool
	createdAt time.Time
	// likes counts the hearts of the message, as of a comment.
	likes int
}

// NewTopicMessage writes content in topic t. Its UUIDv7 ID grows with
// time, so the messages of a topic are ordered and paged by ID. Whether
// authorID may speak for the community is the community's, which the
// caller checks.
func NewTopicMessage(t *Topic, authorID uuid.UUID, content CommentContent, fromWall bool, now time.Time) (*TopicMessage, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &TopicMessage{
		id:       id,
		topicID:  t.id,
		authorID: authorID,
		content:  content,
		fromWall: fromWall,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredTopicMessage is a message as storage keeps it.
type StoredTopicMessage struct {
	ID        uuid.UUID
	TopicID   uuid.UUID
	AuthorID  uuid.UUID
	Content   CommentContent
	FromWall  bool
	CreatedAt time.Time
	Likes     int
}

// RestoreTopicMessage rebuilds a message from storage without validation.
func RestoreTopicMessage(s StoredTopicMessage) *TopicMessage {
	return &TopicMessage{
		id: s.ID, topicID: s.TopicID, authorID: s.AuthorID, content: s.Content, fromWall: s.FromWall, createdAt: s.CreatedAt,
		likes: s.Likes,
	}
}

// CanBeDeletedBy: its author or, admin, whoever runs the community.
func (m *TopicMessage) CanBeDeletedBy(userID uuid.UUID, admin bool) bool {
	return admin || userID == m.authorID
}

// BelongsTo tells whether userID wrote the message, for "mine".
func (m *TopicMessage) BelongsTo(userID uuid.UUID) bool {
	return userID == m.authorID
}

func (m *TopicMessage) ID() uuid.UUID {
	return m.id
}

func (m *TopicMessage) TopicID() uuid.UUID {
	return m.topicID
}

func (m *TopicMessage) AuthorID() uuid.UUID {
	return m.authorID
}

func (m *TopicMessage) Content() CommentContent {
	return m.content
}

// FromWall: the message speaks for the community.
// Notified is who hears about the likes of the message: its author, or
// nobody for a message that speaks for the community, whose author
// readers are not told.
func (m *TopicMessage) Notified() uuid.UUID {
	if m.fromWall {
		return uuid.Nil
	}

	return m.authorID
}

func (m *TopicMessage) FromWall() bool {
	return m.fromWall
}

func (m *TopicMessage) CreatedAt() time.Time {
	return m.createdAt
}

func (m *TopicMessage) Likes() int {
	return m.likes
}

// WithLikes returns m with its hearts counted anew; m does not change.
func (m *TopicMessage) WithLikes(n int) *TopicMessage {
	counted := *m
	counted.likes = n

	return &counted
}

// Hears tells who is notified about m written in the topic and on whose
// behalf: whoever started the topic hears of what others write in it, as
// openvk tells the owner of a topic of its comments, from the writer or,
// for a message that speaks for the community, from the community. Nobody
// (uuid.Nil) hears of their own message, nor of one in a topic started
// for the community, whose author readers are not told.
func (t *Topic) Hears(m *TopicMessage) (recipient, actor uuid.UUID) {
	if t.fromWall || m.authorID == t.authorID {
		return uuid.Nil, uuid.Nil
	}

	if m.fromWall {
		return t.authorID, t.clubID
	}

	return t.authorID, m.authorID
}
