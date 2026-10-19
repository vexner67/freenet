package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// NotificationRepository keeps one notification per like, comment and
// follow, as the unique indexes do in PostgreSQL.
type NotificationRepository struct {
	mu            sync.RWMutex
	notifications map[uuid.UUID]*notification.Notification
}

func NewNotificationRepository() *NotificationRepository {
	return &NotificationRepository{
		notifications: make(map[uuid.UUID]*notification.Notification),
	}
}

// sameEvent tells whether two events are the same like, comment, follow,
// wall post, repost, reply, comment like, accepted or suggested post,
// appointment or mention.
func sameEvent(a, b notification.Event) bool {
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case notification.KindLike, notification.KindPhotoLike, notification.KindStoryLike:
		return a.ActorID == b.ActorID && a.PostID == b.PostID
	case notification.KindComment, notification.KindCommentReply, notification.KindTopicMessage, notification.KindPhotoComment,
		notification.KindStoryComment:
		return a.CommentID == b.CommentID
	case notification.KindCommentLike, notification.KindTopicMessageLike:
		return a.ActorID == b.ActorID && a.CommentID == b.CommentID
	case notification.KindWallPost, notification.KindRepost, notification.KindPostAccepted, notification.KindGift,
		notification.KindVotes:
		return a.PostID == b.PostID
	case notification.KindSuggestedPost, notification.KindPostMention:
		return a.RecipientID == b.RecipientID && a.PostID == b.PostID
	case notification.KindCommentMention, notification.KindTopicMention, notification.KindPhotoMention:
		return a.RecipientID == b.RecipientID && a.CommentID == b.CommentID
	case notification.KindVerified:
		return a.RecipientID == b.RecipientID
	case notification.KindClubVerified:
		return a.ActorID == b.ActorID
	default:
		return a.ActorID == b.ActorID && a.RecipientID == b.RecipientID
	}
}

// Notifications are immutable, so sharing pointers with callers is safe.
func (r *NotificationRepository) Save(ctx context.Context, n *notification.Notification) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.notifications {
		if sameEvent(existing.Event(), n.Event()) {
			return false, nil
		}
	}

	r.notifications[n.ID()] = n

	return true, nil
}

func (r *NotificationRepository) Get(ctx context.Context, id uuid.UUID) (*notification.Notification, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.notifications[id]
	if !ok {
		return nil, notification.ErrNotFound
	}

	return n, nil
}

func (r *NotificationRepository) Delete(ctx context.Context, e notification.Event) error {
	return r.deleteWhere(ctx, func(n *notification.Notification) bool { return sameEvent(n.Event(), e) })
}

func (r *NotificationRepository) DeleteByPost(ctx context.Context, postID uuid.UUID) error {
	return r.deleteWhere(ctx, func(n *notification.Notification) bool { return n.Event().PostID == postID })
}

func (r *NotificationRepository) DeleteByComment(ctx context.Context, commentID uuid.UUID) error {
	return r.deleteWhere(ctx, func(n *notification.Notification) bool { return n.Event().CommentID == commentID })
}

func (r *NotificationRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	return r.deleteWhere(ctx, func(n *notification.Notification) bool {
		e := n.Event()
		return e.RecipientID == userID || e.ActorID == userID
	})
}

// DeleteOlder deletes the oldest notifications with IDs below before.
func (r *NotificationRepository) DeleteOlder(ctx context.Context, before uuid.UUID, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var old []uuid.UUID
	for id := range r.notifications {
		if bytes.Compare(id[:], before[:]) < 0 {
			old = append(old, id)
		}
	}

	slices.SortFunc(old, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	old = old[:min(len(old), limit)]

	for _, id := range old {
		delete(r.notifications, id)
	}

	return len(old), nil
}

func (r *NotificationRepository) deleteWhere(ctx context.Context, match func(*notification.Notification) bool) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, n := range r.notifications {
		if match(n) {
			delete(r.notifications, id)
		}
	}

	return nil
}

func (r *NotificationRepository) List(ctx context.Context, q service.Query) ([]*notification.Notification, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*notification.Notification
	for id, n := range r.notifications {
		read := bytes.Compare(id[:], q.ReadUpTo[:]) <= 0
		beforeCursor := q.Before == uuid.Nil || bytes.Compare(id[:], q.Before[:]) < 0

		if n.Event().RecipientID == q.RecipientID && read != q.Unread && beforeCursor {
			found = append(found, n)
		}
	}

	// Newest first: UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(a, b *notification.Notification) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(bi[:], ai[:])
	})

	if len(found) > q.Limit {
		found = found[:q.Limit]
	}

	return found, nil
}

func (r *NotificationRepository) CountNewer(ctx context.Context, recipientID, after uuid.UUID, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for id, n := range r.notifications {
		if n.Event().RecipientID == recipientID && bytes.Compare(id[:], after[:]) > 0 {
			count++
		}
	}

	return min(count, limit), nil
}

type ReadMarkRepository struct {
	mu    sync.RWMutex
	marks map[uuid.UUID]uuid.UUID
}

func NewReadMarkRepository() *ReadMarkRepository {
	return &ReadMarkRepository{
		marks: make(map[uuid.UUID]uuid.UUID),
	}
}

func (r *ReadMarkRepository) ReadUpTo(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.marks[userID], nil
}

func (r *ReadMarkRepository) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if mark := r.marks[userID]; bytes.Compare(notificationID[:], mark[:]) > 0 {
		r.marks[userID] = notificationID
	}

	return nil
}

func (r *ReadMarkRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.marks, userID)

	return nil
}
