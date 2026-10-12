package postclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/outbox"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakePost struct {
	postv1.PostServiceClient
	followed   *postv1.FollowSourceRequest
	unfollowed *postv1.UnfollowSourceRequest
	deadline   bool
	err        error
	walls      []string
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

// A club closed and opened goes to post as a wall of a club.
func TestWalls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := outbox.NewMemory()
	cats := uuid.New()
	o := NewOutbox(q)

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
	o := NewOutbox(q)

	if err := o.Joined(ctx, cats, ann); err != nil {
		t.Fatal(err)
	}
	if err := o.Left(ctx, cats, ann); err != nil {
		t.Fatal(err)
	}

	head, _ := q.Head(ctx, 10)
	if len(head) != 2 || head[0].Kind != JoinKind || head[1].Kind != LeaveKind {
		t.Fatalf("recorded %+v, want a join and a leave", head)
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
