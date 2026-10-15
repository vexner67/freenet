package grpc

import (
	"context"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/notification/internal/push"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PushService interface {
	PublicKey() (string, error)
	Subscribe(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint, keys push.Keys) error
	Unsubscribe(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint) error
	Messaged(ctx context.Context, m push.Message) error
}

func (h *NotificationHandler) GetPushKey(_ context.Context, _ *notificationv1.GetPushKeyRequest) (*notificationv1.GetPushKeyResponse, error) {
	key, err := h.pushService.PublicKey()
	if err != nil {
		// Off: no key, as the answer says.
		return &notificationv1.GetPushKeyResponse{}, nil //nolint:nilerr // pushes off is a key of nothing
	}

	return &notificationv1.GetPushKeyResponse{PublicKey: key}, nil
}

func (h *NotificationHandler) SubscribePush(ctx context.Context, req *notificationv1.SubscribePushRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	endpoint, err := push.NewEndpoint(req.GetEndpoint())
	if err != nil {
		return nil, err
	}

	keys, err := push.NewKeys(req.GetP256Dh(), req.GetAuth())
	if err != nil {
		return nil, err
	}

	if err = h.pushService.Subscribe(ctx, userID, endpoint, keys); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) UnsubscribePush(ctx context.Context, req *notificationv1.UnsubscribePushRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	endpoint, err := push.NewEndpoint(req.GetEndpoint())
	if err != nil {
		return nil, err
	}

	if err = h.pushService.Unsubscribe(ctx, userID, endpoint); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) PushMessage(ctx context.Context, req *notificationv1.PushMessageRequest) (*emptypb.Empty, error) {
	m, err := fromProtoPushMessage(req)
	if err != nil {
		return nil, err
	}

	if err = h.pushService.Messaged(ctx, m); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func fromProtoPushMessage(req *notificationv1.PushMessageRequest) (push.Message, error) {
	var m push.Message

	ids := []struct {
		s   string
		dst *uuid.UUID
		opt bool
	}{
		{req.GetMessageId(), &m.ID, false},
		{req.GetSenderId(), &m.SenderID, false},
		{req.GetChatId(), &m.ChatID, true},
	}

	for _, id := range ids {
		if id.opt && id.s == "" {
			continue
		}

		parsed, err := uuid.Parse(id.s)
		if err != nil {
			return push.Message{}, status.Error(codes.InvalidArgument, "invalid id")
		}
		*id.dst = parsed
	}

	for _, s := range req.GetRecipientIds() {
		id, err := uuid.Parse(s)
		if err != nil {
			return push.Message{}, status.Error(codes.InvalidArgument, "invalid recipient id")
		}
		m.RecipientIDs = append(m.RecipientIDs, id)
	}

	m.ChatTitle = req.GetChatTitle()

	return m, nil
}
