package postclient

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/outbox"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakePost struct {
	postv1.PostServiceClient
	followed   *postv1.FollowSourceRequest
	unfollowed *postv1.UnfollowSourceRequest
	synced     *postv1.SyncSourcesRequest
	deadline   bool
	err        error
	walls      []string
}

func (f *fakePost) SyncSources(_ context.Context, req *postv1.SyncSourcesRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.synced = req
	return &emptypb.Empty{}, f.err
}

// fakeLock records the clubs whose members were locked.
type fakeLock struct {
	clubs []uuid.UUID
}

func (f *fakeLock) LockClub(_ context.Context, clubID uuid.UUID) error {
	f.clubs = append(f.clubs, clubID)
	return nil
}

func (f *fakePost) FollowSource(ctx context.Context, req *postv1.FollowSourceRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.followed = req
	_, f.deadline = ctx.Deadline()
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) UnfollowSource(_ context.Context, req *postv1.UnfollowSourceRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.unfollowed = req
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) CloseWall(_ context.Context, req *postv1.CloseWallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.walls = append(f.walls, "closed "+req.GetWallId())
	if !req.GetClub() {
		f.walls = append(f.walls, "not a club")
	}
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) OpenWall(_ context.Context, req *postv1.OpenWallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.walls = append(f.walls, "opened "+req.GetWallId())
	return &emptypb.Empty{}, f.err
}

func (f *fakePost) PurgeClub(_ context.Context, req *postv1.PurgeClubRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.walls = append(f.walls, "purged "+req.GetClubId())
	return &emptypb.Empty{}, f.err
}

// A club that is gone goes to post with its wall.
func TestPurge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	cats := uuid.New()
	lock := &fakeLock{}
	api := &fakePost{}
	deliver := Deliver(api, time.Second)

	if err := NewOutbox(q, lock).Deleted(ctx, cats); err != nil {
		t.Fatal(err)
	}
	head, _ := q.Head(ctx, 10)
	if len(head) != 1 || head[0].Kind != PurgeKind || len(lock.clubs) != 1 || lock.clubs[0] != cats {
		t.Fatalf("recorded %+v under %v, want a purge under the club's lock", head, lock.clubs)
	}
	if err := deliver(ctx, head[0]); err != nil {
		t.Fatal(err)
	}
	if len(api.walls) != 1 || api.walls[0] != "purged "+cats.String() {
		t.Fatalf("post heard %v", api.walls)
	}
	if err := (Nop{}).Deleted(ctx, cats); err != nil {
		t.Fatal(err)
	}

	if err := deliver(ctx, outbox.Message{Kind: PurgeKind, Payload: append(cats[:], cats[:]...)}); !outbox.IsPermanent(err) {
		t.Fatalf("a purge of two IDs: %v, want it dropped", err)
	}
}

// A club closed and opened goes to post as a wall of a club.
func TestWalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	cats := uuid.New()
	lock := &fakeLock{}
	o := NewOutbox(q, lock)

	if err := o.Closed(ctx, cats); err != nil {
		t.Fatal(err)
	}
	if err := o.Opened(ctx, cats); err != nil {
		t.Fatal(err)
	}

	head, _ := q.Head(ctx, 10)
	if len(head) != 2 || head[0].Kind != CloseKind || head[1].Kind != OpenKind {
		t.Fatalf("recorded %+v, want a closed and an opened wall", head)
	}

	api := &fakePost{}
	deliver := Deliver(api, time.Second)
	for _, m := range head {
		if err := deliver(ctx, m); err != nil {
			t.Fatalf("deliver %s: %v", m.Kind, err)
		}
	}

	if len(api.walls) != 2 || api.walls[0] != "closed "+cats.String() || api.walls[1] != "opened "+cats.String() {
		t.Fatalf("post heard %v", api.walls)
	}

	// A wall is one ID, not two.
	err := deliver(ctx, outbox.Message{Kind: CloseKind, Payload: append(cats[:], cats[:]...)})
	if !outbox.IsPermanent(err) {
		t.Fatalf("a closed wall of two IDs: %v, want it dropped", err)
	}

	if err := (Nop{}).Closed(ctx, cats); err != nil {
		t.Fatal(err)
	}
	if err := (Nop{}).Opened(ctx, cats); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxAndDelivery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	cats, ann := uuid.New(), uuid.New()
	lock := &fakeLock{}
	o := NewOutbox(q, lock)

	if err := o.Joined(ctx, cats, ann); err != nil {
		t.Fatal(err)
	}
	if err := o.Left(ctx, cats, ann); err != nil {
		t.Fatal(err)
	}

	bob, cat := uuid.MustParse("00000000-0000-7000-8000-000000000001"), uuid.MustParse("00000000-0000-7000-8000-000000000002")
	if err := o.Roster(ctx, cats, bob, []uuid.UUID{cat}, true); err != nil {
		t.Fatal(err)
	}

	// Each is recorded under the lock of the club.
	if want := []uuid.UUID{cats, cats, cats}; !slices.Equal(lock.clubs, want) {
		t.Fatalf("locked %v, want %v", lock.clubs, want)
	}

	head, _ := q.Head(ctx, 10)
	if len(head) != 3 || head[0].Kind != JoinKind || head[1].Kind != LeaveKind || head[2].Kind != SyncKind {
		t.Fatalf("recorded %+v, want a join, a leave and a stretch", head)
	}

	api := &fakePost{}
	deliver := Deliver(api, time.Second)
	for _, m := range head {
		if err := deliver(ctx, m); err != nil {
			t.Fatalf("deliver %s: %v", m.Kind, err)
		}
	}

	// The member reads the club, not the club the member.
	if f := api.followed; f.GetReaderId() != ann.String() || f.GetSourceId() != cats.String() || !f.GetClub() || !api.deadline {
		t.Fatalf("followed %v (deadline %v), want ann reading the club with a timeout", f, api.deadline)
	}
	if u := api.unfollowed; u.GetReaderId() != ann.String() || u.GetSourceId() != cats.String() {
		t.Fatalf("unfollowed %v, want ann and the club", u)
	}

	// The stretch tells the members of a club, above the one it starts at.
	want := &postv1.SyncSourcesRequest{OwnerId: cats.String(), Club: true, AfterId: bob.String(), Last: true, Ids: []string{cat.String()}}
	if !proto.Equal(api.synced, want) {
		t.Fatalf("synced %v, want %v", api.synced, want)
	}

	// A stretch cut short is dropped, not retried for ever.
	if err := deliver(ctx, outbox.Message{Kind: SyncKind, Payload: head[2].Payload[:40]}); !outbox.IsPermanent(err) {
		t.Fatalf("a stretch cut short: %v", err)
	}

	if err := (Nop{}).Joined(ctx, cats, ann); err != nil {
		t.Fatal(err)
	}
	if err := (Nop{}).Left(ctx, cats, ann); err != nil {
		t.Fatal(err)
	}
}

func TestDeliver_Errors(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	join := outbox.Message{Kind: JoinKind, Payload: append(id[:], id[:]...)}

	tests := []struct {
		name      string
		message   outbox.Message
		apiErr    error
		permanent bool
	}{
		{name: "down is retried", message: join, apiErr: status.Error(codes.Unavailable, "down")},
		{name: "plain error is retried", message: join, apiErr: errors.New("broken")},
		// An older post does not know the call yet: the record waits.
		{name: "unimplemented is retried", message: join, apiErr: status.Error(codes.Unimplemented, "old")},
		{name: "rejected is dropped", message: join, apiErr: status.Error(codes.InvalidArgument, "bad"), permanent: true},
		{name: "unknown kind is dropped", message: outbox.Message{Kind: "notify"}, permanent: true},
		{name: "bad payload is dropped", message: outbox.Message{Kind: LeaveKind, Payload: id[:]}, permanent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Deliver(&fakePost{err: tt.apiErr}, time.Second)(context.Background(), tt.message)
			if err == nil || outbox.IsPermanent(err) != tt.permanent {
				t.Errorf("deliver = %v (permanent %v), want an error, permanent %v", err, outbox.IsPermanent(err), tt.permanent)
			}
		})
	}
}
