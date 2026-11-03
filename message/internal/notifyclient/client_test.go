package notifyclient_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/notifyclient"
	"github.com/vexner67/freenet/message/internal/service"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	notificationv1.NotificationServiceClient
	got      chan *notificationv1.PushMessageRequest
	notified chan *notificationv1.Event
}

func (f fakeAPI) Notify(_ context.Context, req *notificationv1.NotifyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.notified <- req.GetEvent()
	return &emptypb.Empty{}, nil
}

func (f fakeAPI) PushMessage(_ context.Context, req *notificationv1.PushMessageRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.got <- req
	return &emptypb.Empty{}, nil
}

func TestPusher(t *testing.T) {
	t.Parallel()

	api := fakeAPI{got: make(chan *notificationv1.PushMessageRequest, 4)}
	p := notifyclient.New(api, time.Second, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ann, bob, chat := uuid.New(), uuid.New(), uuid.New()
	m := service.Pushed{ID: uuid.New(), SenderID: ann, RecipientIDs: []uuid.UUID{bob}, ChatID: chat, ChatTitle: "Поход"}

	p.Push(context.Background(), m)
	// The queue holds one: this one is dropped, not waited for.
	p.Push(context.Background(), service.Pushed{ID: uuid.New(), SenderID: ann, RecipientIDs: []uuid.UUID{bob}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		p.Run(ctx)
		close(done)
	}()

	select {
	case req := <-api.got:
		if req.GetMessageId() != m.ID.String() || req.GetSenderId() != ann.String() || req.GetChatId() != chat.String() ||
			req.GetChatTitle() != "Поход" || len(req.GetRecipientIds()) != 1 || req.GetRecipientIds()[0] != bob.String() {
			t.Fatalf("told %v", req)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing told")
	}

	cancel()
	<-done

	select {
	case req := <-api.got:
		t.Fatalf("the dropped push was told: %v", req)
	default:
	}
}

func TestPusher_Called(t *testing.T) {
	t.Parallel()

	api := fakeAPI{notified: make(chan *notificationv1.Event, 4)}
	p := notifyclient.New(api, time.Second, 4, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ann, bob := uuid.New(), uuid.New()

	p.Called(context.Background(), service.Call{CallerID: ann, PeerID: bob})
	p.Called(context.Background(), service.Call{CallerID: ann, PeerID: bob, Seconds: 95})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	want := []string{
		"KIND_MISSED_CALL " + bob.String() + " " + ann.String() + " 0",
		"KIND_CALL " + bob.String() + " " + ann.String() + " 95",
		"KIND_CALL " + ann.String() + " " + bob.String() + " 95",
	}
	for _, w := range want {
		select {
		case e := <-api.notified:
			if got := fmt.Sprintf("%s %s %s %d", e.GetKind(), e.GetRecipientId(), e.GetActorId(), e.GetSeconds()); got != w {
				t.Fatalf("told %s, want %s", got, w)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("not told %s", w)
		}
	}
}
