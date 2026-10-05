package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
)

type Notifications struct {
	api notificationv1.NotificationServiceClient
}

func NewNotifications(api notificationv1.NotificationServiceClient) *Notifications {
	return &Notifications{
		api: api,
	}
}

// List returns the caller's unread notifications or the archive.
func (n *Notifications) List(ctx context.Context, accessToken string, unread bool, cursor string, limit int) (model.NotificationPage, error) {
	box := notificationv1.Box_BOX_ARCHIVE
	if unread {
		box = notificationv1.Box_BOX_UNREAD
	}

	resp, err := n.api.ListNotifications(withToken(ctx, accessToken), &notificationv1.ListNotificationsRequest{
		Box:    box,
		Cursor: cursor,
		Limit:  toLimit(limit),
	})
	if err != nil {
		return model.NotificationPage{}, errs.Wrap("n.api.ListNotifications", fromStatus(err))
	}

	page := model.NotificationPage{
		Notifications: make([]model.Notification, 0, len(resp.GetNotifications())),
		NextCursor:    resp.GetNextCursor(),
	}

	for _, pn := range resp.GetNotifications() {
		page.Notifications = append(page.Notifications, model.Notification{
			ID:        pn.GetId(),
			Kind:      enumName(pn.GetKind().String(), kindPrefix),
			ActorID:   pn.GetActorId(),
			PostID:    pn.GetPostId(),
			CommentID: pn.GetCommentId(),
			CreatedAt: pn.GetCreatedAt().AsTime(),
		})
	}

	return page, nil
}

func (n *Notifications) CountUnread(ctx context.Context, accessToken string) (int, error) {
	resp, err := n.api.CountUnread(withToken(ctx, accessToken), &notificationv1.CountUnreadRequest{})
	if err != nil {
		return 0, errs.Wrap("n.api.CountUnread", fromStatus(err))
	}

	return int(resp.GetCount()), nil
}

// MarkRead moves the caller's notifications up to id to the archive.
func (n *Notifications) MarkRead(ctx context.Context, accessToken, id string) error {
	if _, err := n.api.MarkRead(withToken(ctx, accessToken), &notificationv1.MarkReadRequest{NotificationId: id}); err != nil {
		return errs.Wrap("n.api.MarkRead", fromStatus(err))
	}

	return nil
}
