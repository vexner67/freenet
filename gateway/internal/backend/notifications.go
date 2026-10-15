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

// Subscribe streams what happens to the caller's notifications until ctx
// ends, the token expires (Unauthenticated) or the caller falls behind.
func (n *Notifications) Subscribe(ctx context.Context, accessToken string) (model.NotificationChanges, error) {
	stream, err := n.api.Subscribe(withToken(ctx, accessToken), &notificationv1.SubscribeRequest{})
	if err != nil {
		return nil, errs.Wrap("n.api.Subscribe", fromStatus(err))
	}

	return func() (string, error) {
		resp, err := stream.Recv()
		if err != nil {
			return "", errs.Wrap("stream.Recv", fromStatus(err))
		}

		return enumName(resp.GetChange().String(), changePrefix), nil
	}, nil
}

// PushKey is the key browsers subscribe to pushes with; "" when pushes are
// off.
func (n *Notifications) PushKey(ctx context.Context) (string, error) {
	resp, err := n.api.GetPushKey(ctx, &notificationv1.GetPushKeyRequest{})
	if err != nil {
		return "", errs.Wrap("n.api.GetPushKey", fromStatus(err))
	}

	return resp.GetPublicKey(), nil
}

// SubscribePush lets the browser of s take the caller's pushes.
func (n *Notifications) SubscribePush(ctx context.Context, accessToken string, s model.PushSubscription) error {
	_, err := n.api.SubscribePush(withToken(ctx, accessToken), &notificationv1.SubscribePushRequest{
		Endpoint: s.Endpoint,
		P256Dh:   s.P256DH,
		Auth:     s.Auth,
	})
	if err != nil {
		return errs.Wrap("n.api.SubscribePush", fromStatus(err))
	}

	return nil
}

// UnsubscribePush stops pushing to the caller's browser at endpoint.
func (n *Notifications) UnsubscribePush(ctx context.Context, accessToken, endpoint string) error {
	if _, err := n.api.UnsubscribePush(withToken(ctx, accessToken), &notificationv1.UnsubscribePushRequest{Endpoint: endpoint}); err != nil {
		return errs.Wrap("n.api.UnsubscribePush", fromStatus(err))
	}

	return nil
}
