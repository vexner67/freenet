package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// TopicCursor is where a page of topics ends: they go pinned first, then
// the most recently active, newest first among equals.
type TopicCursor struct {
	Pinned    bool
	UpdatedAt time.Time
	ID        uuid.UUID
}

// TopicRepository keeps the topics of communities with their messages.
type TopicRepository interface {
	Save(ctx context.Context, t *post.Topic) error
	// Get and Update return post.ErrTopicNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Topic, error)
	// Update saves the title and the settings.
	Update(ctx context.Context, t *post.Topic) error
	// Delete deletes a topic with its messages, returning the messages;
	// post.ErrTopicNotFound.
	Delete(ctx context.Context, id uuid.UUID) ([]*post.TopicMessage, error)
	// List returns up to limit topics of a club after cursor (nil: from
	// the first) whose titles contain query, whatever the case ("" for
	// all); Count counts them all.
	List(ctx context.Context, clubID uuid.UUID, query string, after *TopicCursor, limit int) ([]*post.Topic, error)
	Count(ctx context.Context, clubID uuid.UUID, query string) (int, error)
}

// TopicMessageRepository keeps what is written in topics.
type TopicMessageRepository interface {
	// Save adds a message, counting it in its topic and making the topic
	// the most recently active, in one statement; post.ErrTopicNotFound
	// when the topic is gone.
	Save(ctx context.Context, m *post.TopicMessage) error
	// Get returns post.ErrMessageNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.TopicMessage, error)
	// Delete removes a message and uncounts it; post.ErrMessageNotFound.
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit messages of a topic, oldest first, with
	// IDs above after (uuid.Nil: from the first).
	List(ctx context.Context, topicID, after uuid.UUID, limit int) ([]*post.TopicMessage, error)
	// Last returns the newest message of each topic that has any.
	Last(ctx context.Context, topicIDs []uuid.UUID) ([]*post.TopicMessage, error)
	// GetMany returns the messages that exist, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.TopicMessage, error)
}

// TopicMessageLikeRepository keeps the hearts of messages as
// CommentLikeRepository keeps those of comments.
type TopicMessageLikeRepository interface {
	// Like and Unlike are idempotent and return how many likes the
	// message has now; post.ErrMessageNotFound for a missing message.
	Like(ctx context.Context, messageID, userID uuid.UUID) (int, error)
	Unlike(ctx context.Context, messageID, userID uuid.UUID) (int, error)
	// Liked returns which of messageIDs the user likes.
	Liked(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	// Likers returns up to limit users who like the message, newest
	// first, after the cursor (nil: from the newest).
	Likers(ctx context.Context, messageID uuid.UUID, after *LikerCursor, limit int) ([]Liker, error)
}

// TopicPage is one screen of topics with the newest message of each, by
// topic, for "ответил"; NextCursor is nil on the last.
type TopicPage struct {
	Topics     []*post.Topic
	Last       map[uuid.UUID]*post.TopicMessage
	NextCursor *TopicCursor
	Total      int
}

// TopicMessagePage is one screen of a topic, oldest first; NextCursor is
// uuid.Nil on the last, Total counts the messages of the topic. Liked
// are those of them the viewer likes.
type TopicMessagePage struct {
	Messages   []*post.TopicMessage
	Liked      map[uuid.UUID]bool
	NextCursor uuid.UUID
	Total      int
}

// StartTopicInput is a topic to start: by AuthorID in the discussions of
// ClubID, for the community if FromWall, with a first message if Content
// is set ("Текст" of openvk's "Новая тема").
type StartTopicInput struct {
	ClubID   uuid.UUID
	AuthorID uuid.UUID
	Title    post.TopicTitle
	FromWall bool
	Content  *post.CommentContent
	// Mentions are the people the first message names.
	Mentions post.Mentions
}

// TopicMessageInput is a message to write in TopicID, for the community
// if FromWall.
type TopicMessageInput struct {
	TopicID  uuid.UUID
	AuthorID uuid.UUID
	Content  post.CommentContent
	FromWall bool
	// Mentions are the people the text names, who hear of it.
	Mentions post.Mentions
}

// TopicService keeps the discussions of communities ("Обсуждения" of
// openvk). Whether someone may start a topic and speak for the community
// is the community's, which the caller checks, as for posts; who runs it,
// for the rest, the service asks itself (WallModerators).
type TopicService struct {
	topics     TopicRepository
	messages   TopicMessageRepository
	likes      TopicMessageLikeRepository
	tx         TxManager
	notifier   Notifier
	photos     PhotoKeeper
	moderators WallModerators
	pages      Pages
	now        func() time.Time
}

// TopicDeps are what TopicService works with.
type TopicDeps struct {
	// Pages knows the deleted pages, whose messages are not read; nil: none.
	Pages      Pages
	Topics     TopicRepository
	Messages   TopicMessageRepository
	Likes      TopicMessageLikeRepository
	Tx         TxManager
	Notifier   Notifier
	Photos     PhotoKeeper
	Moderators WallModerators
}

func NewTopicService(d TopicDeps) *TopicService {
	return &TopicService{
		topics:     d.Topics,
		messages:   d.Messages,
		likes:      d.Likes,
		tx:         d.Tx,
		notifier:   d.Notifier,
		photos:     d.Photos,
		moderators: d.Moderators,
		pages:      d.Pages,
		now:        time.Now,
	}
}

// Start creates a topic and its first message, if any, at once; media
// keeps the message's photos.
func (s *TopicService) Start(ctx context.Context, in StartTopicInput) (*post.Topic, error) {
	t, err := post.NewTopic(in.ClubID, in.AuthorID, in.Title, in.FromWall, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewTopic", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.topics.Save(ctx, t); err != nil {
			return errs.Wrap("s.topics.Save", err)
		}

		if in.Content == nil {
			return nil
		}

		_, err := s.newMessage(ctx, t, TopicMessageInput{
			TopicID: t.ID(), AuthorID: in.AuthorID, Content: *in.Content, FromWall: in.FromWall, Mentions: in.Mentions,
		})

		return err
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	// Read back: the first message counted and dated it.
	return s.Get(ctx, t.ID())
}

func (s *TopicService) Get(ctx context.Context, id uuid.UUID) (*post.Topic, error) {
	t, err := s.topics.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.topics.Get", err)
	}

	return t, nil
}

// List pages the topics of a club whose titles contain query ("" for
// all), pinned first, then the most recently active.
func (s *TopicService) List(ctx context.Context, clubID uuid.UUID, query string, after *TopicCursor, size int) (TopicPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra tells whether there is a next page.
	found, err := s.topics.List(ctx, clubID, query, after, size+1)
	if err != nil {
		return TopicPage{}, errs.Wrap("s.topics.List", err)
	}

	page := TopicPage{Topics: found, Last: make(map[uuid.UUID]*post.TopicMessage)}
	if len(found) > size {
		page.Topics = found[:size]
		last := page.Topics[size-1]
		page.NextCursor = &TopicCursor{Pinned: last.Pinned(), UpdatedAt: last.UpdatedAt(), ID: last.ID()}
	}

	if page.Total, err = s.topics.Count(ctx, clubID, query); err != nil {
		return TopicPage{}, errs.Wrap("s.topics.Count", err)
	}

	ids := make([]uuid.UUID, 0, len(page.Topics))
	for _, t := range page.Topics {
		ids = append(ids, t.ID())
	}

	last, err := s.messages.Last(ctx, ids)
	if err != nil {
		return TopicPage{}, errs.Wrap("s.messages.Last", err)
	}

	for _, m := range last {
		page.Last[m.TopicID()] = m
	}

	return page, nil
}

// Change renames a topic and changes its settings ("Редактирование
// темы"): see post.Topic.Changed.
func (s *TopicService) Change(ctx context.Context, requesterID, id uuid.UUID, changes post.TopicChanges) (*post.Topic, error) {
	t, err := s.topics.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.topics.Get", err)
	}

	admin, err := s.moderators.CanModerate(ctx, t.ClubID(), requesterID)
	if err != nil {
		return nil, errs.Wrap("s.moderators.CanModerate", err)
	}

	changed, err := t.Changed(requesterID, admin, changes)
	if err != nil {
		return nil, errs.Wrap("t.Changed", err)
	}

	if err = s.topics.Update(ctx, changed); err != nil {
		return nil, errs.Wrap("s.topics.Update", err)
	}

	return changed, nil
}

// Delete deletes a topic with its messages and their photos ("Удалить
// тему"): its author or the admins.
func (s *TopicService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	t, err := s.topics.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.topics.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, t.ClubID(), requesterID, t.CanBeChangedBy(requesterID, false))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotChangeTopic
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		messages, err := s.topics.Delete(ctx, id)
		if err != nil {
			return errs.Wrap("s.topics.Delete", err)
		}

		// The notifications about its messages name the topic.
		if err = s.notifier.ForgetPost(ctx, id); err != nil {
			return errs.Wrap("s.notifier.ForgetPost", err)
		}

		return errs.Wrap("deleteMessagePhotos", deleteMessagePhotos(ctx, s.photos, messages))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// Write leaves a message in a topic: nobody in a closed topic, only the
// admins in a restricted one.
func (s *TopicService) Write(ctx context.Context, in TopicMessageInput) (*post.TopicMessage, error) {
	t, err := s.topics.Get(ctx, in.TopicID)
	if err != nil {
		return nil, errs.Wrap("s.topics.Get", err)
	}

	// Only a restricted topic asks who runs the community.
	admin, err := moderates(ctx, s.moderators, t.ClubID(), in.AuthorID, !t.Restricted())
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if err = t.CheckWrite(admin); err != nil {
		return nil, errs.Wrap("t.CheckWrite", err)
	}

	var m *post.TopicMessage

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		m, err = s.newMessage(ctx, t, in)

		return err
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return m, nil
}

// newMessage saves a message in t with its photos, in the transaction of
// ctx, and tells whoever started the topic and the people it names.
func (s *TopicService) newMessage(ctx context.Context, t *post.Topic, in TopicMessageInput) (*post.TopicMessage, error) {
	authorID, content := in.AuthorID, in.Content

	m, err := post.NewTopicMessage(t, authorID, content, in.FromWall, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewTopicMessage", err)
	}

	if err = s.messages.Save(ctx, m); err != nil {
		return nil, errs.Wrap("s.messages.Save", err)
	}

	if photos := content.Photos(); len(photos) > 0 {
		if err = s.photos.Publish(ctx, authorID, photos); err != nil {
			return nil, errs.Wrap("s.photos.Publish", err)
		}
	}

	for _, e := range topicEvents(t, m, in.Mentions) {
		if err = s.notifier.Notify(ctx, e); err != nil {
			return nil, errs.Wrap("s.notifier.Notify", err)
		}
	}

	return m, nil
}

// topicEvents are the notifications of message m in t: whoever started
// the topic hears of it (Topic.Hears), and the people it names of that,
// unless they heard as the starter; from the community when the message
// speaks for it.
func topicEvents(t *post.Topic, m *post.TopicMessage, mentions post.Mentions) []Event {
	var events []Event

	starter, actor := t.Hears(m)
	if starter != uuid.Nil {
		events = append(events, Event{Kind: EventTopicMessage, RecipientID: starter, ActorID: actor, PostID: t.ID(), CommentID: m.ID()})
	}

	actor = m.AuthorID()
	if m.FromWall() {
		actor = t.ClubID()
	}

	for _, id := range mentions.Heard(m.AuthorID(), starter) {
		events = append(events, Event{Kind: EventTopicMention, RecipientID: id, ActorID: actor, PostID: t.ID(), CommentID: m.ID()})
	}

	return events
}

// LikeMessage and UnlikeMessage put a heart on a message and take it
// off, as on a comment; they return how many it has now. Nobody is
// notified.
func (s *TopicService) LikeMessage(ctx context.Context, userID, id uuid.UUID) (int, error) {
	return s.setMessageLike(ctx, userID, id, true)
}

func (s *TopicService) UnlikeMessage(ctx context.Context, userID, id uuid.UUID) (int, error) {
	return s.setMessageLike(ctx, userID, id, false)
}

// setMessageLike tells the author of the message about the like, as the
// author of a comment hears about its own, and takes that back with it.
func (s *TopicService) setMessageLike(ctx context.Context, userID, id uuid.UUID, like bool) (int, error) {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return 0, errs.Wrap("s.messages.Get", err)
	}

	set, tell := s.likes.Unlike, s.notifier.Retract
	if like {
		set, tell = s.likes.Like, s.notifier.Notify
	}

	var likes int

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if likes, err = set(ctx, id, userID); err != nil {
			return errs.Wrap("set", err)
		}

		// Nobody is notified about liking their own message, nor about a
		// message that speaks for a community (post.TopicMessage.Notified).
		recipient := m.Notified()
		if recipient == uuid.Nil || recipient == userID {
			return nil
		}

		e := Event{Kind: EventTopicMessageLike, RecipientID: recipient, ActorID: userID, PostID: m.TopicID(), CommentID: id}

		return errs.Wrap("tell", tell(ctx, e))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return likes, nil
}

// Messages pages what is written in a topic, oldest first, as openvk
// shows it.
func (s *TopicService) Messages(ctx context.Context, viewerID, topicID, after uuid.UUID, size int) (TopicMessagePage, error) {
	t, err := s.topics.Get(ctx, topicID)
	if err != nil {
		return TopicMessagePage{}, errs.Wrap("s.topics.Get", err)
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	found, err := s.messages.List(ctx, topicID, after, size+1)
	if err != nil {
		return TopicMessagePage{}, errs.Wrap("s.messages.List", err)
	}

	page := TopicMessagePage{Messages: found, Total: t.Messages()}
	if len(found) > size {
		page.Messages = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if viewerID == uuid.Nil || len(page.Messages) == 0 {
		return page, nil
	}

	ids := make([]uuid.UUID, 0, len(page.Messages))
	for _, m := range page.Messages {
		ids = append(ids, m.ID())
	}

	if page.Liked, err = s.likes.Liked(ctx, viewerID, ids); err != nil {
		return TopicMessagePage{}, errs.Wrap("s.likes.Liked", err)
	}

	return page, nil
}

// MessageLikers pages who put a heart on a message, newest first
// ("Понравилось N людям"); post.ErrMessageNotFound for a missing message.
func (s *TopicService) MessageLikers(ctx context.Context, id uuid.UUID, cursor *LikerCursor, size int) (LikerPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.messages.Get", err)
	}

	found, err := s.likes.Likers(ctx, id, cursor, size+1)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.likes.Likers", err)
	}

	return likerPage(found, size, m.Likes()), nil
}

// MessagesByID returns the messages with these IDs that exist, at most
// MaxPageSize, in no particular order; those a deleted page wrote are
// left out, as the list of a topic leaves them.
func (s *TopicService) MessagesByID(ctx context.Context, ids []uuid.UUID) ([]*post.TopicMessage, error) {
	if len(ids) > MaxPageSize {
		return nil, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return nil, nil
	}

	found, err := s.messages.GetMany(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.messages.GetMany", err)
	}

	found, err = written(ctx, s.pages, found, messageWriter)

	return found, errs.Wrap("written", err)
}

// messageWriter is whose words a message is: no one's when it speaks for
// the community.
func messageWriter(m *post.TopicMessage) uuid.UUID {
	return own(m.AuthorID(), m.FromWall())
}

// Message is one message of a topic, whole; post.ErrMessageNotFound for
// a missing one and for one a deleted page wrote.
func (s *TopicService) Message(ctx context.Context, id uuid.UUID) (*post.TopicMessage, error) {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.messages.Get", err)
	}

	gone, err := deletedPages(ctx, s.pages, messageWriter(m))
	if err != nil {
		return nil, errs.Wrap("deletedPages", err)
	}

	if len(gone) > 0 {
		return nil, post.ErrMessageNotFound
	}

	return m, nil
}

// DeleteMessage deletes a message with its photos: its author or the
// admins.
func (s *TopicService) DeleteMessage(ctx context.Context, requesterID, id uuid.UUID) error {
	m, err := s.messages.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.messages.Get", err)
	}

	t, err := s.topics.Get(ctx, m.TopicID())
	if err != nil {
		return errs.Wrap("s.topics.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, t.ClubID(), requesterID, m.CanBeDeletedBy(requesterID, false))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotDeleteMessage
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.messages.Delete(ctx, id); err != nil {
			return errs.Wrap("s.messages.Delete", err)
		}

		if err := s.notifier.ForgetComment(ctx, id); err != nil {
			return errs.Wrap("s.notifier.ForgetComment", err)
		}

		return errs.Wrap("deleteMessagePhotos", deleteMessagePhotos(ctx, s.photos, []*post.TopicMessage{m}))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// deleteMessagePhotos tells media to delete the photos of messages, each
// under its author.
func deleteMessagePhotos(ctx context.Context, photos PhotoKeeper, messages []*post.TopicMessage) error {
	for _, m := range messages {
		if ids := m.Content().Photos(); len(ids) > 0 {
			if err := photos.Delete(ctx, m.AuthorID(), ids); err != nil {
				return errs.Wrap("photos.Delete", err)
			}
		}
	}

	return nil
}
