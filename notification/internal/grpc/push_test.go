package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/notification/internal/push"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// keyOnly has a key but pushes nothing: the queue is not run here.
type keyOnly struct{}

func (keyOnly) Send(context.Context, *push.Subscription, push.Payload) error { return nil }
func (keyOnly) PublicKey() string                                            { return "BKey" }

type nobody struct{}

func (nobody) Actors(context.Context, []uuid.UUID) (map[uuid.UUID]push.Actor, error) {
	return nil, nil
}

const (
	p256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	auth   = "BTBZMqHH6r4Tts7J_aSIgg"
)

func TestPush(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ctx := context.Background()
	ann := uuid.New()

	key, err := s.client.GetPushKey(ctx, &notificationv1.GetPushKeyRequest{})
	if err != nil || key.GetPublicKey() != "BKey" {
		t.Fatalf("GetPushKey = %v, %v", key, err)
	}

	sub := &notificationv1.SubscribePushRequest{Endpoint: "https://push.example/ann", P256Dh: p256dh, Auth: auth}

	if _, err = s.client.SubscribePush(ctx, sub); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("SubscribePush without a token: %v", err)
	}

	if _, err = s.client.SubscribePush(s.as(t, ann), sub); err != nil {
		t.Fatalf("SubscribePush: %v", err)
	}

	bad := []*notificationv1.SubscribePushRequest{
		{Endpoint: "http://push.example/ann", P256Dh: p256dh, Auth: auth},
		{Endpoint: "https://push.example/ann", P256Dh: auth, Auth: auth},
		{Endpoint: "https://push.example/ann", P256Dh: p256dh, Auth: "x"},
	}
	for _, req := range bad {
		if _, err = s.client.SubscribePush(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("SubscribePush(%v): %v, want InvalidArgument", req, err)
		}
	}

	if _, err = s.client.UnsubscribePush(s.as(t, ann), &notificationv1.UnsubscribePushRequest{Endpoint: sub.GetEndpoint()}); err != nil {
		t.Fatalf("UnsubscribePush: %v", err)
	}

	// message calls PushMessage without a user token.
	msg := &notificationv1.PushMessageRequest{MessageId: uuid.NewString(), SenderId: uuid.NewString(), RecipientIds: []string{ann.String()}}
	if _, err = s.client.PushMessage(ctx, msg); err != nil {
		t.Fatalf("PushMessage: %v", err)
	}

	msg.RecipientIds = nil
	if _, err = s.client.PushMessage(ctx, msg); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PushMessage without recipients: %v", err)
	}

	msg.RecipientIds, msg.ChatId = []string{ann.String()}, "chat"
	if _, err = s.client.PushMessage(ctx, msg); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PushMessage with a bad chat: %v", err)
	}
}
