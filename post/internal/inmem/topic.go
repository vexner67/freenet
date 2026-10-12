package inmem

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// TopicRepository keeps topics and their messages, as the two tables do
// in PostgreSQL: a message counts in its topic and dates it, a deleted
// topic takes its messages. Topics are stored as StoredTopic and handed
// out anew, so callers never share them; messages are immutable.
type TopicRepository struct {
	mu       sync.RWMutex
	topics   map[uuid.UUID]post.StoredTopic
	messages map[uuid.UUID]*post.TopicMessage
	// likes are who put a heart on each message, and when.
	likes map[uuid.UUID]map[uuid.UUID]time.Time
	// heldLikes are the hearts of deleted pages.
	heldLikes heldLikes
	// hidden are the deleted pages themselves.
	hidden hiddenUsers
}

func NewTopicRepository() *TopicRepository {
	return &TopicRepository{
		topics:    make(map[uuid.UUID]post.StoredTopic),
		messages:  make(map[uuid.UUID]*post.TopicMessage),
		likes:     make(map[uuid.UUID]map[uuid.UUID]time.Time),
		heldLikes: make(heldLikes),
		hidden:    make(hiddenUsers),
	}
}

// Messages is the same repository as service.TopicMessageRepository.
func (r *TopicRepository) Messages() *TopicMessageRepository {
	return &TopicMessageRepository{r: r}
}

func (r *TopicRepository) Save(ctx context.Context, t *post.Topic) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.topics[t.ID()] = t.Stored()

	return nil
}

func (r *TopicRepository) Get(ctx context.Context, id uuid.UUID) (*post.Topic, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.topics[id]
	if !ok {
		return nil, post.ErrTopicNotFound
	}

	return r.seenTopic(s), nil
}

func (r *TopicRepository) Update(ctx context.Context, t *post.Topic) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.topics[t.ID()]
	if !ok {
		return post.ErrTopicNotFound
	}

	changed := t.Stored()
	s.Title, s.Closed, s.Pinned, s.Restricted = changed.Title, changed.Closed, changed.Pinned, changed.Restricted
	r.topics[t.ID()] = s

	return nil
}

func (r *TopicRepository) Delete(ctx context.Context, id uuid.UUID) ([]*post.TopicMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.topics[id]; !ok {
		return nil, post.ErrTopicNotFound
	}

	return r.deleteTopic(id, uuid.Nil), nil
}

// deleteTopic removes a topic with its messages, returning those not by
// keepOut; the lock is held.
func (r *TopicRepository) deleteTopic(id, keepOut uuid.UUID) []*post.TopicMessage {
	delete(r.topics, id)

	var gone []*post.TopicMessage
	for mid, m := range r.messages {
		if m.TopicID() == id {
			delete(r.messages, mid)
			delete(r.likes, mid)
			if m.AuthorID() != keepOut {
				gone = append(gone, m)
			}
		}
	}

	return gone
}

func (r *TopicRepository) List(ctx context.Context, clubID uuid.UUID, query string, after *service.TopicCursor, limit int) ([]*post.Topic, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := r.matching(clubID, query)
	slices.SortFunc(found, func(a, b post.StoredTopic) int { return -compareTopics(a, b) })

	var out []*post.Topic
	for _, s := range found {
		if after != nil && compareTopics(s, post.StoredTopic{Pinned: after.Pinned, UpdatedAt: after.UpdatedAt, ID: after.ID}) >= 0 {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, r.seenTopic(s))
	}

	return out, nil
}

func (r *TopicRepository) Count(ctx context.Context, clubID uuid.UUID, query string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.matching(clubID, query)), nil
}

// matching are the topics of a club whose titles contain query; the lock
// is held.
func (r *TopicRepository) matching(clubID uuid.UUID, query string) []post.StoredTopic {
	var found []post.StoredTopic
	for _, s := range r.topics {
		if s.ClubID == clubID && strings.Contains(strings.ToLower(s.Title.String()), strings.ToLower(query)) && !r.hidden.writes(s.AuthorID, s.FromWall) {
			found = append(found, s)
		}
	}

	return found
}

// compareTopics orders topics as (pinned, updated_at, id) in PostgreSQL.
func compareTopics(a, b post.StoredTopic) int {
	switch {
	case a.Pinned != b.Pinned:
		if a.Pinned {
			return 1
		}
		return -1
	case !a.UpdatedAt.Equal(b.UpdatedAt):
		return a.UpdatedAt.Compare(b.UpdatedAt)
	default:
		return compareIDs(a.ID, b.ID)
	}
}

func (r *TopicRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*post.TopicMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, users := range r.likes {
		delete(users, userID)
	}

	for id, m := range r.messages {
		if m.AuthorID() == userID {
			delete(r.messages, id)
			delete(r.likes, id)
			r.recount(m.TopicID(), -1)
		}
	}

	var gone []*post.TopicMessage
	for id, s := range r.topics {
		if s.AuthorID == userID {
			gone = append(gone, r.deleteTopic(id, userID)...)
		}
	}

	return gone, nil
}

// recount changes the count of a topic by delta; the lock is held.
func (r *TopicRepository) recount(topicID uuid.UUID, delta int) {
	if s, ok := r.topics[topicID]; ok {
		s.Messages += delta
		r.topics[topicID] = s
	}
}

// TopicMessageRepository is the service.TopicMessageRepository side of
// TopicRepository.
type TopicMessageRepository struct {
	r *TopicRepository
}

func (m *TopicMessageRepository) Save(ctx context.Context, msg *post.TopicMessage) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	m.r.mu.Lock()
	defer m.r.mu.Unlock()

	s, ok := m.r.topics[msg.TopicID()]
	if !ok {
		return post.ErrTopicNotFound
	}

	s.Messages++
	s.UpdatedAt = msg.CreatedAt()
	m.r.topics[msg.TopicID()] = s
	m.r.messages[msg.ID()] = msg

	return nil
}

func (m *TopicMessageRepository) Get(ctx context.Context, id uuid.UUID) (*post.TopicMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.RLock()
	defer m.r.mu.RUnlock()

	msg, ok := m.r.messages[id]
	if !ok {
		return nil, post.ErrMessageNotFound
	}

	return m.r.counted(msg), nil
}

func (m *TopicMessageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	m.r.mu.Lock()
	defer m.r.mu.Unlock()

	msg, ok := m.r.messages[id]
	if !ok {
		return post.ErrMessageNotFound
	}

	delete(m.r.messages, id)
	delete(m.r.likes, id)
	m.r.recount(msg.TopicID(), -1)

	return nil
}

// counted is msg with its hearts as they are now; the lock is held.
func (r *TopicRepository) counted(msg *post.TopicMessage) *post.TopicMessage {
	return msg.WithLikes(len(r.likes[msg.ID()]))
}

func (m *TopicMessageRepository) Like(ctx context.Context, messageID, userID uuid.UUID) (int, error) {
	return m.setLike(ctx, messageID, userID, true)
}

func (m *TopicMessageRepository) Unlike(ctx context.Context, messageID, userID uuid.UUID) (int, error) {
	return m.setLike(ctx, messageID, userID, false)
}

func (m *TopicMessageRepository) setLike(ctx context.Context, messageID, userID uuid.UUID, like bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.Lock()
	defer m.r.mu.Unlock()

	if _, ok := m.r.messages[messageID]; !ok {
		return 0, post.ErrMessageNotFound
	}

	if m.r.likes[messageID] == nil {
		m.r.likes[messageID] = make(map[uuid.UUID]time.Time)
	}

	if _, liked := m.r.likes[messageID][userID]; like && !liked {
		m.r.likes[messageID][userID] = time.Now()
	} else if !like {
		delete(m.r.likes[messageID], userID)
	}

	return len(m.r.likes[messageID]), nil
}

func (m *TopicMessageRepository) Likers(ctx context.Context, messageID uuid.UUID, after *service.LikerCursor, limit int) ([]service.Liker, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.RLock()
	defer m.r.mu.RUnlock()

	return likers(m.r.likes[messageID], after, limit), nil
}

func (m *TopicMessageRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.TopicMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.RLock()
	defer m.r.mu.RUnlock()

	var found []*post.TopicMessage
	for _, id := range ids {
		if msg, ok := m.r.messages[id]; ok {
			found = append(found, m.r.counted(msg))
		}
	}

	return found, nil
}

func (m *TopicMessageRepository) Liked(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.RLock()
	defer m.r.mu.RUnlock()

	liked := make(map[uuid.UUID]bool)
	for _, id := range messageIDs {
		if _, ok := m.r.likes[id][userID]; ok {
			liked[id] = true
		}
	}

	return liked, nil
}

func (m *TopicMessageRepository) List(ctx context.Context, topicID, after uuid.UUID, limit int) ([]*post.TopicMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.RLock()
	defer m.r.mu.RUnlock()

	var found []*post.TopicMessage
	for _, msg := range m.r.messages {
		if msg.TopicID() == topicID && compareIDs(msg.ID(), after) > 0 && !m.r.hidden.writes(msg.AuthorID(), msg.FromWall()) {
			found = append(found, m.r.counted(msg))
		}
	}

	slices.SortFunc(found, func(a, b *post.TopicMessage) int { return compareIDs(a.ID(), b.ID()) })

	return found[:min(limit, len(found))], nil
}

func (m *TopicMessageRepository) Last(ctx context.Context, topicIDs []uuid.UUID) ([]*post.TopicMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	m.r.mu.RLock()
	defer m.r.mu.RUnlock()

	last := make(map[uuid.UUID]*post.TopicMessage)
	for _, msg := range m.r.messages {
		if !slices.Contains(topicIDs, msg.TopicID()) {
			continue
		}
		if prev, ok := last[msg.TopicID()]; !ok || compareIDs(msg.ID(), prev.ID()) > 0 {
			last[msg.TopicID()] = msg
		}
	}

	out := make([]*post.TopicMessage, 0, len(last))
	for _, msg := range last {
		out = append(out, msg)
	}

	return out, nil
}

// compareIDs orders UUIDs as PostgreSQL does.
func compareIDs(a, b uuid.UUID) int {
	return bytes.Compare(a[:], b[:])
}
