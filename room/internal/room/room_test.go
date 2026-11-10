package room_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/room"
)

func TestTitle(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, text, want string
		bad              bool
	}{
		{"empty", "", "", true}, {"spaces", "  ", "", true}, {"trim", " Привет 👋 ", "Привет 👋", false},
		{"boundary", strings.Repeat("я", 64), strings.Repeat("я", 64), false}, {"long", strings.Repeat("я", 65), "", true},
		{"newline", "hello\nworld", "", true}, {"tab", "hello\tworld", "", true}, {"separator", "hello\u2028world", "", true}, {"invalid utf8", "\xff", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := room.NewTitle(tt.text)
			if errors.Is(err, room.ErrInvalidTitle) != tt.bad || got.String() != tt.want {
				t.Fatalf("got %q %v", got, err)
			}
		})
	}
}

func TestEnums(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"talk", "watch", "listen"} {
		if _, err := room.NewKind(v); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{"everyone", "friends", "link", "club"} {
		if _, err := room.NewAccess(v); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{"owner", "everyone"} {
		if _, err := room.NewControl(v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := room.NewKind("video"); !errors.Is(err, room.ErrInvalidKind) {
		t.Fatal(err)
	}
	if _, err := room.NewAccess(""); !errors.Is(err, room.ErrInvalidAccess) {
		t.Fatal(err)
	}
	if _, err := room.NewControl("all"); !errors.Is(err, room.ErrInvalidControl) {
		t.Fatal(err)
	}
}

func makeRoom(t *testing.T, access room.Access, permanent bool) *room.Room {
	t.Helper()
	r, err := room.New(room.Input{OwnerID: uuid.New(), ClubID: uuid.New(), Title: "Friends", Kind: room.KindTalk, Access: access, Control: room.ControlOwner, Permanent: permanent}, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID().Version() != 7 {
		t.Fatal("not UUIDv7")
	}
	return r
}

func TestAccess(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		access room.Access
		viewer room.Viewer
		err    error
	}{
		{"guest", room.AccessEveryone, room.Viewer{}, room.ErrForbidden},
		{"everyone", room.AccessEveryone, room.Viewer{ID: uuid.New()}, nil},
		{"link", room.AccessLink, room.Viewer{ID: uuid.New()}, nil},
		{"not friend", room.AccessFriends, room.Viewer{ID: uuid.New()}, room.ErrForbidden},
		{"friend", room.AccessFriends, room.Viewer{ID: uuid.New(), Friend: true}, nil},
		{"not member", room.AccessClub, room.Viewer{ID: uuid.New()}, room.ErrForbidden},
		{"member", room.AccessClub, room.Viewer{ID: uuid.New(), ClubMember: true}, nil},
		{"blocked", room.AccessEveryone, room.Viewer{ID: uuid.New(), Blocked: true}, room.ErrForbidden},
		{"banned", room.AccessEveryone, room.Viewer{ID: uuid.New(), Banned: true}, room.ErrForbidden},
		{"kicked", room.AccessEveryone, room.Viewer{ID: uuid.New(), Kicked: true}, room.ErrKicked},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := makeRoom(t, tt.access, false)
			if err := r.CanEnter(tt.viewer); !errors.Is(err, tt.err) {
				t.Fatalf("got %v want %v", err, tt.err)
			}
		})
	}
	r := makeRoom(t, room.AccessFriends, false)
	if err := r.CanEnter(room.Viewer{ID: r.OwnerID()}); err != nil {
		t.Fatal(err)
	}
	if r.CanView(room.Viewer{}, false) {
		t.Fatal("guest sees friends room")
	}
	r = makeRoom(t, room.AccessEveryone, false)
	if !r.CanView(room.Viewer{}, false) {
		t.Fatal("guest cannot see public room")
	}
	r = makeRoom(t, room.AccessLink, false)
	if r.CanView(room.Viewer{ID: r.OwnerID()}, true) {
		t.Fatal("link room listed")
	}
	if !r.CanView(room.Viewer{ID: r.OwnerID()}, false) {
		t.Fatal("owner cannot see link room")
	}
}

func TestControlKickClose(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	for _, permanent := range []bool{false, true} {
		r := makeRoom(t, room.AccessEveryone, permanent)
		other := uuid.New()
		if r.CanControl(other, true) || r.CanControl(r.OwnerID(), false) || !r.CanControl(r.OwnerID(), true) {
			t.Fatal("invalid control")
		}
		if _, err := r.Kick(other, r.OwnerID(), now); !errors.Is(err, room.ErrForbidden) {
			t.Fatal(err)
		}
		if _, err := r.Kick(r.OwnerID(), r.OwnerID(), now); !errors.Is(err, room.ErrForbidden) {
			t.Fatal(err)
		}
		until, err := r.Kick(r.OwnerID(), other, now)
		if err != nil || (permanent && !until.Equal(now.Add(24*time.Hour))) || (!permanent && until.Year() != 9999) {
			t.Fatalf("kick %v %v", until, err)
		}
		if err := r.Close(other, now); !errors.Is(err, room.ErrForbidden) {
			t.Fatal(err)
		}
		if err := r.Close(r.OwnerID(), now); err != nil {
			t.Fatal(err)
		}
		if err := r.CanEnter(room.Viewer{ID: r.OwnerID()}); !errors.Is(err, room.ErrClosed) {
			t.Fatal(err)
		}
		if r.CanView(room.Viewer{}, false) || r.CanControl(r.OwnerID(), true) {
			t.Fatal("closed room visible")
		}
	}
}

func TestPlayback(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	item, err := room.NewQueueItem(room.PlaybackYouTube, "dQw4w9WgXcQ", "Song")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		in   room.PlaybackInput
		bad  bool
	}{
		{"empty", room.PlaybackInput{}, false},
		{"youtube", room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Playing: true, Position: 42}, false},
		{"audio", room.PlaybackInput{Kind: room.PlaybackAudio, MediaID: uuid.NewString()}, false},
		{"injection", room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "https://evil"}, true},
		{"nil uuid", room.PlaybackInput{Kind: room.PlaybackAudio, MediaID: uuid.Nil.String()}, true},
		{"negative", room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Position: -1}, true},
		{"nan", room.PlaybackInput{Position: math.NaN()}, true}, {"infinite", room.PlaybackInput{Position: math.Inf(1)}, true},
		{"empty playing", room.PlaybackInput{Playing: true}, true}, {"too many", room.PlaybackInput{Queue: make([]room.QueueItem, 51)}, true},
		{"invalid item", room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Queue: []room.QueueItem{{}}}, true},
		{"queue", room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Queue: []room.QueueItem{item}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := room.NewPlayback(tt.in, 3, now)
			if errors.Is(err, room.ErrInvalidPlayback) != tt.bad {
				t.Fatal(err)
			}
			if !tt.bad && (p.Version() != 3 || !p.UpdatedAt().Equal(now)) {
				t.Fatal("state lost")
			}
		})
	}
	in := room.PlaybackInput{Kind: room.PlaybackYouTube, MediaID: "dQw4w9WgXcQ", Queue: []room.QueueItem{item}}
	p, err := room.NewPlayback(in, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	in.Queue[0] = room.QueueItem{}
	q := p.Queue()
	q[0] = room.QueueItem{}
	if p.Queue()[0].MediaID() != "dQw4w9WgXcQ" {
		t.Fatal("mutable queue")
	}
}

func TestChat(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		text string
		bad  bool
	}{{" hi ", false}, {strings.Repeat("я", 1000), false}, {strings.Repeat("я", 1001), true}, {"", true}, {"\xff", true}} {
		_, err := room.NewChatText(tt.text)
		if errors.Is(err, room.ErrInvalidChat) != tt.bad {
			t.Fatal(err)
		}
	}
	r, u := uuid.New(), uuid.New()
	c, err := room.NewChat(r, u, uuid.Nil, "hello", time.Now())
	if err != nil || c.ID().Version() != 7 || c.RoomID() != r || c.AuthorID() != u {
		t.Fatal(err)
	}
	if _, err := room.NewChat(r, u, uuid.New(), "text", time.Now()); !errors.Is(err, room.ErrInvalidChat) {
		t.Fatal(err)
	}
	if _, err := room.NewChat(r, u, uuid.New(), "", time.Now()); err != nil {
		t.Fatal(err)
	}
}
