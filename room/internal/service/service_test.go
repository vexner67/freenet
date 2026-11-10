package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pubsub"
	"github.com/vexner67/freenet/room/internal/inmem"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

var ctx = context.Background()

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

type pair struct{ owner, viewer uuid.UUID }
type users struct {
	relations map[pair]service.Relationships
	blocks    map[pair]bool
	noInvite  bool
}

func (u *users) Access(_ context.Context, owner, viewer uuid.UUID) (service.Relationships, error) {
	a := u.relations[pair{owner, viewer}]
	a.Blocked = a.Blocked || u.blocks[pair{owner, viewer}] || u.blocks[pair{viewer, owner}]
	return a, nil
}
func (u *users) Blocked(_ context.Context, viewer uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	m := map[uuid.UUID]bool{}
	for _, id := range ids {
		m[id] = u.blocks[pair{viewer, id}] || u.blocks[pair{id, viewer}]
	}
	return m, nil
}
func (u *users) CanInvite(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return !u.noInvite, nil
}

type clubs struct{ access service.ClubAccess }

func (c *clubs) Access(context.Context, uuid.UUID, uuid.UUID) (service.ClubAccess, error) {
	return c.access, nil
}

type notifier struct{ invited, revoked int }

func (n *notifier) Invite(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) error {
	n.invited++
	return nil
}
func (n *notifier) Revoke(context.Context, uuid.UUID) error { n.revoked++; return nil }

type tokenVerifier struct {
	id, user uuid.UUID
	err      error
}

func (v *tokenVerifier) Verify(string) (uuid.UUID, uuid.UUID, error) { return v.id, v.user, v.err }

type harness struct {
	tokens *tokenVerifier
	s      *service.Service
	clock  *clock
	users  *users
	clubs  *clubs
	state  *inmem.State
	repo   *inmem.Repository
	media  *inmem.Media
	events *pubsub.Hub[uuid.UUID, service.Event]
	notify *notifier
}

func setup() *harness {
	c := &clock{time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)}
	h := &harness{tokens: &tokenVerifier{}, clock: c, users: &users{relations: map[pair]service.Relationships{}, blocks: map[pair]bool{}}, clubs: &clubs{}, state: inmem.NewState(c), repo: inmem.NewRepository(), media: inmem.NewMedia(c), events: pubsub.New[uuid.UUID, service.Event](128), notify: &notifier{}}
	h.s = service.New(service.Deps{MediaTokens: h.tokens, Rooms: h.repo, Bans: h.repo, Presence: h.state, Index: h.state, Expiry: h.state, SweepLock: h.state, Playback: h.state, Chat: h.state, Users: h.users, Clubs: h.clubs, Media: h.media, Notify: h.notify, Events: h.events, Clock: c, Tx: &inmem.TxManager{}, MaxMembers: 5, MaxLive: 20, EmptyTTL: 5 * time.Minute})
	return h
}
func (h *harness) create(t *testing.T, owner uuid.UUID, permanent bool, access room.Access) *room.Room {
	t.Helper()
	r, err := h.s.CreateRoom(ctx, service.CreateInput{ActorID: owner, Room: room.Input{Title: "Friends", Kind: room.KindTalk, Access: access, Control: room.ControlEveryone, Permanent: permanent}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func join(t *testing.T, h *harness, r *room.Room, id uuid.UUID) service.Joined {
	t.Helper()
	v, err := h.s.JoinRoom(ctx, r.ID(), id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestJoinFullAndSwitch(t *testing.T) {
	t.Parallel()
	h := setup()
	owner := uuid.New()
	a := h.create(t, owner, false, room.AccessEveryone)
	b := h.create(t, owner, false, room.AccessEveryone)
	for range 5 {
		join(t, h, b, uuid.New())
	}
	user := uuid.New()
	join(t, h, a, user)
	if _, err := h.s.JoinRoom(ctx, b.ID(), user); !errors.Is(err, room.ErrFull) {
		t.Fatal(err)
	}
	current, _ := h.state.Current(ctx, []uuid.UUID{user})
	if current[user] != a.ID() {
		t.Fatal("full destination removed original membership")
	}
	c := h.create(t, owner, false, room.AccessEveryone)
	result := join(t, h, c, user)
	if len(result.View.Members) != 1 || result.Media.Token == "" {
		t.Fatal("join state missing")
	}
	current, _ = h.state.Current(ctx, []uuid.UUID{user})
	if current[user] != c.ID() {
		t.Fatal("switch failed")
	}
	members, _ := h.state.Members(ctx, a.ID())
	if len(members) != 0 {
		t.Fatal("old membership kept")
	}
	removed := h.media.Removed()
	if len(removed) != 1 || removed[0] != [2]uuid.UUID{a.ID(), user} {
		t.Fatal("old media subscription kept")
	}
	join(t, h, c, user)
	members, _ = h.state.Members(ctx, c.ID())
	if len(members) != 1 {
		t.Fatal("join not idempotent")
	}
}

func TestAccessAndListing(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, viewer := uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessFriends)
	if _, err := h.s.GetRoom(ctx, r.ID(), viewer); !errors.Is(err, room.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := h.s.JoinRoom(ctx, r.ID(), viewer); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
	h.users.relations[pair{owner, viewer}] = service.Relationships{Friend: true}
	join(t, h, r, viewer)
	h.users.blocks[pair{viewer, owner}] = true
	if _, err := h.s.GetRoom(ctx, r.ID(), viewer); !errors.Is(err, room.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := h.s.JoinRoom(ctx, r.ID(), viewer); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
	delete(h.users.blocks, pair{viewer, owner})
	h.users.relations[pair{owner, viewer}] = service.Relationships{Banned: true}
	if _, err := h.s.JoinRoom(ctx, r.ID(), viewer); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
	link := h.create(t, owner, false, room.AccessLink)
	join(t, h, link, owner)
	public := h.create(t, owner, false, room.AccessEveryone)
	join(t, h, public, uuid.New())
	rows, _, err := h.s.ListRooms(ctx, uuid.Nil, service.Filter{Limit: 20}, true)
	if err != nil || len(rows) != 1 || rows[0].Room.ID() != public.ID() {
		t.Fatalf("private listing: %v %v", rows, err)
	}
}

func TestPermanentQuotas(t *testing.T) {
	t.Parallel()
	h := setup()
	owner := uuid.New()
	for range 3 {
		h.create(t, owner, true, room.AccessEveryone)
	}
	_, err := h.s.CreateRoom(ctx, service.CreateInput{ActorID: owner, Room: room.Input{Title: "four", Kind: room.KindTalk, Access: room.AccessEveryone, Control: room.ControlOwner, Permanent: true}})
	if !errors.Is(err, room.ErrTooManyRooms) {
		t.Fatal(err)
	}
	club := uuid.New()
	in := service.CreateInput{ActorID: owner, Room: room.Input{ClubID: club, Title: "club", Kind: room.KindTalk, Access: room.AccessClub, Control: room.ControlOwner, Permanent: true}}
	if _, err = h.s.CreateRoom(ctx, in); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
	h.clubs.access = service.ClubAccess{Member: true, Manager: true}
	for range 5 {
		if _, err = h.s.CreateRoom(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = h.s.CreateRoom(ctx, in); !errors.Is(err, room.ErrTooManyRooms) {
		t.Fatal(err)
	}
}

func TestKickAndPermanentBanExpiry(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, user := uuid.New(), uuid.New()
	r := h.create(t, owner, true, room.AccessEveryone)
	join(t, h, r, owner)
	join(t, h, r, user)
	in := service.MemberInput{ActorID: user, RoomID: r.ID(), MemberID: owner}
	if err := h.s.KickMember(ctx, in); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
	in.ActorID, in.MemberID = owner, user
	if err := h.s.KickMember(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.JoinRoom(ctx, r.ID(), user); !errors.Is(err, room.ErrKicked) {
		t.Fatal(err)
	}
	if _, err := h.s.Token(ctx, r.ID(), user); !errors.Is(err, room.ErrNotFound) {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(24 * time.Hour)
	join(t, h, r, user)
}

func TestSweepAndRenewal(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, user := uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessEveryone)
	join(t, h, r, user)
	h.clock.now = h.clock.now.Add(40 * time.Second)
	if err := h.s.Heartbeat(ctx, r.ID(), user); err != nil {
		t.Fatal(err)
	}
	h.clock.now = h.clock.now.Add(10 * time.Second)
	if err := h.s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	members, _ := h.state.Members(ctx, r.ID())
	if len(members) != 1 {
		t.Fatal("renewed user evicted")
	}
	h.clock.now = h.clock.now.Add(35 * time.Second)
	if err := h.s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	members, _ = h.state.Members(ctx, r.ID())
	if len(members) != 0 {
		t.Fatal("stale user kept")
	}
	h.clock.now = h.clock.now.Add(5 * time.Minute)
	if err := h.s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	saved, _ := h.repo.Get(ctx, r.ID())
	if !saved.Closed() {
		t.Fatal("empty temporary room not closed")
	}
	permanent := h.create(t, owner, true, room.AccessEveryone)
	h.clock.now = h.clock.now.Add(time.Hour)
	if err := h.s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	saved, _ = h.repo.Get(ctx, permanent.ID())
	if saved.Closed() {
		t.Fatal("permanent room closed")
	}
}

func TestPlaybackCASAndControls(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, user := uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessEveryone)
	join(t, h, r, owner)
	join(t, h, r, user)
	in := service.PlaybackInput{ActorID: user, RoomID: r.ID(), Playback: room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Playing: true}}
	p, err := h.s.SetPlayback(ctx, in)
	if err != nil || p.Version() != 1 || !p.UpdatedAt().Equal(h.clock.now) {
		t.Fatal(err)
	}
	if _, err = h.s.SetPlayback(ctx, in); !errors.Is(err, room.ErrStalePlayback) {
		t.Fatal(err)
	}
	_, err = h.s.UpdateRoom(ctx, service.UpdateInput{ActorID: owner, RoomID: r.ID(), Room: room.Input{Title: "owner", Kind: room.KindWatch, Access: room.AccessEveryone, Control: room.ControlOwner}})
	if err != nil {
		t.Fatal(err)
	}
	in.Version = 1
	if _, err = h.s.SetPlayback(ctx, in); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
}

func TestChatFilteringAndLimit(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, a, b := uuid.New(), uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessEveryone)
	join(t, h, r, a)
	join(t, h, r, b)
	events, cancel := h.events.Subscribe(b)
	defer cancel()
	h.users.blocks[pair{b, a}] = true
	for range 55 {
		if _, err := h.s.SendChat(ctx, service.ChatInput{ActorID: a, RoomID: r.ID(), Text: "hello"}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case e := <-events:
		t.Fatalf("blocked chat delivered %v", e.Kind)
	default:
	}
	history, _ := h.state.Chat(ctx, r.ID())
	if len(history) != 50 {
		t.Fatalf("history length %d", len(history))
	}
	joined := join(t, h, r, b)
	if len(joined.Chat) != 0 || len(joined.MutedIDs) != 1 || joined.MutedIDs[0] != a {
		t.Fatal("blocked history/audio exposed")
	}
	if err := h.s.LeaveRoom(ctx, r.ID(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.SendChat(ctx, service.ChatInput{ActorID: a, RoomID: r.ID(), Text: "after leave"}); !errors.Is(err, room.ErrNotMember) {
		t.Fatal(err)
	}
}

func TestConcurrentJoins(t *testing.T) {
	t.Parallel()
	h := setup()
	r := h.create(t, uuid.New(), false, room.AccessEveryone)
	var wg sync.WaitGroup
	successes := make(chan uuid.UUID, 20)
	for range 20 {
		wg.Go(func() {
			id := uuid.New()
			_, err := h.s.JoinRoom(ctx, r.ID(), id)
			if err == nil {
				successes <- id
			} else if !errors.Is(err, room.ErrFull) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	close(successes)
	if len(successes) != 5 {
		t.Fatalf("accepted %d", len(successes))
	}
	members, _ := h.state.Members(ctx, r.ID())
	if len(members) != 5 {
		t.Fatal("overfilled")
	}
}

func TestCloseInvitePurge(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, user := uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessEveryone)
	join(t, h, r, owner)
	join(t, h, r, user)
	in := service.MemberInput{ActorID: owner, RoomID: r.ID(), MemberID: user}
	if err := h.s.Invite(ctx, in); err != nil || h.notify.invited != 1 {
		t.Fatal(err)
	}
	h.users.noInvite = true
	if err := h.s.Invite(ctx, in); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
	if err := h.s.PurgeUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.JoinRoom(ctx, r.ID(), user); !errors.Is(err, room.ErrNotFound) {
		t.Fatal(err)
	}
	current, _ := h.state.Current(ctx, []uuid.UUID{user, owner})
	if len(current) != 0 || len(h.media.Deleted()) != 1 || h.notify.revoked != 1 {
		t.Fatal("close did not clean up")
	}
}

func TestOwnerChangingKindClearsIncompatibleScreen(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, member := uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessEveryone)
	join(t, h, r, member)
	p, err := h.s.SetPlayback(ctx, service.PlaybackInput{ActorID: member, RoomID: r.ID(), Playback: room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Playing: true}})
	if err != nil {
		t.Fatal(err)
	}
	// An owner need not join just to change their room's settings.
	view, err := h.s.UpdateRoom(ctx, service.UpdateInput{ActorID: owner, RoomID: r.ID(), Room: room.Input{Title: "Conversation", Kind: room.KindTalk, Access: room.AccessEveryone, Control: room.ControlOwner}})
	if err != nil || view.Room.Kind() != room.KindTalk || view.Playback.Kind() != room.PlaybackNone || view.Playback.Version() != p.Version()+1 {
		t.Fatal(view, err)
	}
}

func TestMediaConnectionRechecksMembershipAndBan(t *testing.T) {
	t.Parallel()
	h := setup()
	owner, user := uuid.New(), uuid.New()
	r := h.create(t, owner, false, room.AccessEveryone)
	h.tokens.id, h.tokens.user = r.ID(), user
	if err := h.s.AuthorizeMedia(ctx, "cached"); !errors.Is(err, room.ErrNotMember) {
		t.Fatal(err)
	}
	join(t, h, r, user)
	if err := h.s.AuthorizeMedia(ctx, "cached"); err != nil {
		t.Fatal(err)
	}
	if err := h.s.LeaveRoom(ctx, r.ID(), user); err != nil {
		t.Fatal(err)
	}
	if err := h.s.AuthorizeMedia(ctx, "cached"); !errors.Is(err, room.ErrNotMember) {
		t.Fatal("old media token survived leave", err)
	}
	join(t, h, r, user)
	if err := h.s.KickMember(ctx, service.MemberInput{ActorID: owner, RoomID: r.ID(), MemberID: user}); err != nil {
		t.Fatal(err)
	}
	if err := h.s.AuthorizeMedia(ctx, "cached"); err == nil {
		t.Fatal("kicked participant retained media access")
	}
	h.tokens.err = room.ErrForbidden
	if err := h.s.AuthorizeMedia(ctx, "forged"); !errors.Is(err, room.ErrForbidden) {
		t.Fatal(err)
	}
}
