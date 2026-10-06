package grpc

import (
	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/notification/internal/notification"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	kindToProto = map[notification.Kind]notificationv1.Kind{
		notification.KindLike:     notificationv1.Kind_KIND_LIKE,
		notification.KindComment:  notificationv1.Kind_KIND_COMMENT,
		notification.KindFollow:   notificationv1.Kind_KIND_FOLLOW,
		notification.KindWallPost: notificationv1.Kind_KIND_WALL_POST,
		notification.KindRepost:   notificationv1.Kind_KIND_REPOST,
	}
	kindFromProto = map[notificationv1.Kind]notification.Kind{
		notificationv1.Kind_KIND_LIKE:      notification.KindLike,
		notificationv1.Kind_KIND_COMMENT:   notification.KindComment,
		notificationv1.Kind_KIND_FOLLOW:    notification.KindFollow,
		notificationv1.Kind_KIND_WALL_POST: notification.KindWallPost,
		notificationv1.Kind_KIND_REPOST:    notification.KindRepost,
	}
)

// fromProtoEvent parses the IDs; whether they fit the kind is the domain's
// call (notification.Event.Validate).
func fromProtoEvent(e *notificationv1.Event) (notification.Event, error) {
	kind, ok := kindFromProto[e.GetKind()]
	if !ok {
		return notification.Event{}, status.Error(codes.InvalidArgument, "unknown notification kind")
	}

	event := notification.Event{Kind: kind}

	ids := []struct {
		raw      string
		dst      *uuid.UUID
		optional bool
	}{
		{e.GetRecipientId(), &event.RecipientID, false},
		{e.GetActorId(), &event.ActorID, false},
		{e.GetPostId(), &event.PostID, true},
		{e.GetCommentId(), &event.CommentID, true},
	}

	for _, id := range ids {
		if id.raw == "" && id.optional {
			continue
		}

		parsed, err := uuid.Parse(id.raw)
		if err != nil {
			return notification.Event{}, status.Error(codes.InvalidArgument, "invalid id in the event")
		}
		*id.dst = parsed
	}

	return event, nil
}

func toProtoNotification(n *notification.Notification) *notificationv1.Notification {
	e := n.Event()

	return &notificationv1.Notification{
		Id:        n.ID().String(),
		Kind:      kindToProto[e.Kind],
		ActorId:   e.ActorID.String(),
		PostId:    optionalID(e.PostID),
		CommentId: optionalID(e.CommentID),
		CreatedAt: timestamppb.New(n.CreatedAt()),
	}
}

// optionalID is "" for uuid.Nil.
func optionalID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}
