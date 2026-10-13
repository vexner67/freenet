package purgeclient

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/outbox"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakePost struct {
	postv1.PostServiceClient
	purged   string
	held     string
	released string
	sources  []string
	deadline bool
	err      error
}

func (f *fakePost) PurgeUser(ctx context.Context, req *postv1.PurgeUserRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.purged = req.GetUserId()
	_, f.deadline = ctx.Deadline()
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) HoldUserReactions(_ context.Context, req *postv1.HoldUserReactionsRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.held = req.GetUserId()
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) ReleaseUserReactions(_ context.Context, req *postv1.ReleaseUserReactionsRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.released = req.GetUserId()
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) SyncSources(ctx context.Context, req *postv1.SyncSourcesRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	told := "=" + req.GetOwnerId() + " above " + req.GetAfterId() + ": " + strings.Join(req.GetIds(), ",")
	if req.GetLast() {
		told += " last"
	}
	if req.GetClub() {
		told += " a club"
	}

	f.sources = append(f.sources, told)
	_, f.deadline = ctx.Deadline()

	return &emptypb.Empty{}, f.err
}

// fakeLock records whose follows were locked.
type fakeLock struct {
	readers []uuid.UUID
}

func (f *fakeLock) LockReader(_ context.Context, readerID uuid.UUID) error {
	f.readers = append(f.readers, readerID)
	return nil
}

func (f *fakePost) FollowSource(ctx context.Context, req *postv1.FollowSourceRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.sources = append(f.sources, "+"+req.GetReaderId()+">"+req.GetSourceId())
	_, f.deadline = ctx.Deadline()
	if req.GetClub() {
		f.sources = append(f.sources, "a club")
	}
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) UnfollowSource(_ context.Context, req *postv1.UnfollowSourceRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.sources = append(f.sources, "-"+req.GetReaderId()+">"+req.GetSourceId())
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) CloseWall(_ context.Context, req *postv1.CloseWallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.sources = append(f.sources, "closed "+req.GetWallId())
	if req.GetClub() {
		f.sources = append(f.sources, "a club")
	}
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) OpenWall(_ context.Context, req *postv1.OpenWallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.sources = append(f.sources, "opened "+req.GetWallId())
	return &emptypb.Empty{}, f.err
}

type fakeNotification struct {
	notificationv1.NotificationServiceClient
	purged string
}

func (f *fakeNotification) PurgeUser(_ context.Context, req *notificationv1.PurgeUserRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.purged = req.GetUserId()
	return &emptypb.Empty{}, nil
}

func TestOutboxAndDelivery(t *testing.T) {
	t.Parallel()

	post, media := outbox.NewMemory(), outbox.NewMemory()
	ann := uuid.New()

	if err := NewOutbox(post, media).UserErased(context.Background(), ann); err != nil {
		t.Fatal(err)
	}

	for _, q := range []*outbox.Memory{post, media} {
		if head, _ := q.Head(context.Background(), 10); len(head) != 1 || head[0].Kind != Kind {
			t.Fatalf("recorded %+v, want one erasure in each stream", head)
		}
	}

	head, _ := post.Head(context.Background(), 1)
	api := &fakePost{}
	if err := Post(api, time.Second)(context.Background(), head[0]); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if api.purged != ann.String() || !api.deadline {
		t.Fatalf("purged %q (deadline %v), want ann with a timeout", api.purged, api.deadline)
	}
}

// A deleted and a restored page go to post in the order they happened.
func TestReactions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	ann := uuid.New()
	reactions := NewReactions(q)

	if err := reactions.UserDeactivated(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err := reactions.UserReactivated(ctx, ann); err != nil {
		t.Fatal(err)
	}

	head, _ := q.Head(ctx, 10)
	if len(head) != 2 || head[0].Kind != HoldKind || head[1].Kind != ReleaseKind {
		t.Fatalf("recorded %+v, want a hold and a release", head)
	}

	api := &fakePost{}
	deliver := Post(api, time.Second)
	if err := deliver(ctx, head[0]); err != nil || api.held != ann.String() || api.released != "" {
		t.Fatalf("hold: %v, held %q, released %q", err, api.held, api.released)
	}
	if err := deliver(ctx, head[1]); err != nil || api.released != ann.String() || api.purged != "" {
		t.Fatalf("release: %v, released %q, purged %q", err, api.released, api.purged)
	}

	// Only post keeps reactions: another stream drops the kind.
	if err := Media(nil, time.Second)(ctx, head[0]); !outbox.IsPermanent(err) {
		t.Fatalf("media delivers a hold: %v", err)
	}

	if err := (NoReactions{}).UserDeactivated(ctx, ann); err != nil {
		t.Fatal(err)
	}
}

// A follow and its end go to post, who reads whom, in the order they
// happened.
func TestFollows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	ann, bob := uuid.New(), uuid.New()
	lock := &fakeLock{}
	follows := NewFollows(q, lock)

	if err := follows.Followed(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}
	if err := follows.Unfollowed(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}

	cat, dan := uuid.MustParse("00000000-0000-7000-8000-000000000001"), uuid.MustParse("00000000-0000-7000-8000-000000000002")
	if err := follows.Following(ctx, ann, cat, []uuid.UUID{dan}, true); err != nil {
		t.Fatal(err)
	}
	if err := follows.Following(ctx, bob, uuid.Nil, nil, false); err != nil {
		t.Fatal(err)
	}

	// Each is recorded under the lock of the one who follows.
	if want := []uuid.UUID{ann, ann, ann, bob}; !slices.Equal(lock.readers, want) {
		t.Fatalf("locked %v, want %v", lock.readers, want)
	}

	head, _ := q.Head(ctx, 10)
	if len(head) != 4 || head[0].Kind != FollowKind || head[1].Kind != UnfollowKind || head[2].Kind != SyncKind {
		t.Fatalf("recorded %+v, want a follow, its end and two stretches", head)
	}

	// A stretch cut short is dropped, not retried for ever.
	if err := Post(&fakePost{}, time.Second)(ctx, outbox.Message{Kind: SyncKind, Payload: head[2].Payload[:40]}); !outbox.IsPermanent(err) {
		t.Fatalf("a stretch cut short: %v", err)
	}

	api := &fakePost{}
	deliver := Post(api, time.Second)
	for _, m := range head {
		if err := deliver(ctx, m); err != nil {
			t.Fatalf("deliver %s: %v", m.Kind, err)
		}
	}

	want := []string{
		"+" + ann.String() + ">" + bob.String(),
		"-" + ann.String() + ">" + bob.String(),
		"=" + ann.String() + " above " + cat.String() + ": " + dan.String() + " last",
		"=" + bob.String() + " above " + uuid.Nil.String() + ": ",
	}
	if !slices.Equal(api.sources, want) || !api.deadline {
		t.Fatalf("post heard %v (deadline %v), want %v with a timeout", api.sources, api.deadline, want)
	}
}

// A closed and an opened page go to post in the order they happened.
func TestWalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	ann := uuid.New()
	walls := NewWalls(q)

	if err := walls.PageClosed(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err := walls.PageOpened(ctx, ann); err != nil {
		t.Fatal(err)
	}

	head, _ := q.Head(ctx, 10)
	if len(head) != 2 || head[0].Kind != CloseWallKind || head[1].Kind != OpenWallKind {
		t.Fatalf("recorded %+v, want a closed and an opened wall", head)
	}

	api := &fakePost{}
	deliver := Post(api, time.Second)
	for _, m := range head {
		if err := deliver(ctx, m); err != nil {
			t.Fatalf("deliver %s: %v", m.Kind, err)
		}
	}

	want := []string{"closed " + ann.String(), "opened " + ann.String()}
	if len(api.sources) != 2 || api.sources[0] != want[0] || api.sources[1] != want[1] {
		t.Fatalf("post heard %v, want %v", api.sources, want)
	}
}

func TestDeliver_Errors(t *testing.T) {
	t.Parallel()

	ann := uuid.New()
	erase := outbox.Message{Kind: Kind, Payload: ann[:]}
	follow := outbox.Message{Kind: FollowKind, Payload: append(ann[:], ann[:]...)}

	tests := []struct {
		name      string
		message   outbox.Message
		apiErr    error
		permanent bool
	}{
		{name: "down is retried", message: erase, apiErr: status.Error(codes.Unavailable, "down")},
		{name: "plain error is retried", message: erase, apiErr: errors.New("broken")},
		{name: "hold down is retried", message: outbox.Message{Kind: HoldKind, Payload: ann[:]}, apiErr: status.Error(codes.Unavailable, "down")},
		// An older post, still running while the site is rolled out, does
		// not know the call yet: the record waits for the new one.
		{name: "unimplemented is retried", message: erase, apiErr: status.Error(codes.Unimplemented, "old")},
		{name: "closing unimplemented is retried", message: outbox.Message{Kind: CloseWallKind, Payload: ann[:]}, apiErr: status.Error(codes.Unimplemented, "old")},
		{name: "follow unimplemented is retried", message: follow, apiErr: status.Error(codes.Unimplemented, "old")},
		{name: "follow down is retried", message: follow, apiErr: status.Error(codes.Unavailable, "down")},
		{name: "follow rejected is dropped", message: follow, apiErr: status.Error(codes.InvalidArgument, "bad"), permanent: true},
		{name: "half a follow is dropped", message: outbox.Message{Kind: UnfollowKind, Payload: ann[:]}, permanent: true},
		{name: "rejected is dropped", message: erase, apiErr: status.Error(codes.InvalidArgument, "bad"), permanent: true},
		{name: "unknown kind is dropped", message: outbox.Message{Kind: "notify"}, permanent: true},
		{name: "bad payload is dropped", message: outbox.Message{Kind: Kind, Payload: []byte{1, 2}}, permanent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Post(&fakePost{err: tt.apiErr}, time.Second)(context.Background(), tt.message)
			if err == nil || outbox.IsPermanent(err) != tt.permanent {
				t.Errorf("deliver = %v (permanent %v), want an error, permanent %v", err, outbox.IsPermanent(err), tt.permanent)
			}
		})
	}
}

// The notification stream carries notifications too: they go on to the
// notification deliverer.
func TestNotification(t *testing.T) {
	t.Parallel()

	var others []string
	api := &fakeNotification{}
	deliver := Notification(api, time.Second, func(_ context.Context, m outbox.Message) error {
		others = append(others, m.Kind)
		return nil
	})

	ann := uuid.New()
	for _, m := range []outbox.Message{{Kind: "notify"}, {Kind: Kind, Payload: ann[:]}} {
		if err := deliver(context.Background(), m); err != nil {
			t.Fatal(err)
		}
	}

	if len(others) != 1 || others[0] != "notify" || api.purged != ann.String() {
		t.Fatalf("others %v, purged %q", others, api.purged)
	}
}
