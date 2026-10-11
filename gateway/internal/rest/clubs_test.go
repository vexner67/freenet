package rest_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeClubs knows k1, "Кошки" at the address cats, owned by ann (u1),
// with an open wall. role is what the caller is in it; closedWall shuts
// the wall to members, and only admins speak for the club.
type fakeClubs struct {
	token, joined, left  string
	changes              model.ClubChanges
	admin                map[string]bool
	members              []model.ClubMember
	adminsOnly, fromClub bool
	managed, pinned      bool
	avatarID, query      string
	backdrop             [2]string
	role                 string
	closedWall           bool
	suggestWall          bool
	openTopics           bool
	transferredTo        string
	described            model.ClubMember
	event                model.NewEvent
	going, kind, period  string
	clubsOf              string
	rolesAsked           []string
	err                  error
}

var cats = model.Club{ID: "k1", Name: "Кошки", Address: "cats", Wall: "open", Members: 2, CreatedAt: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}

func (f *fakeClubs) view() model.ClubView {
	c := cats
	c.AvatarID = f.avatarID
	c.BackdropLeft, c.BackdropRight = f.backdrop[0], f.backdrop[1]
	if f.closedWall {
		c.Wall = "closed"
	}
	if f.suggestWall {
		c.Wall = "suggest"
	}
	return model.ClubView{Club: c, Role: f.role, OwnerID: "u1", CanStartTopic: f.openTopics || f.role == "admin" || f.role == "owner"}
}

func (f *fakeClubs) Create(_ context.Context, token, name, description string) (model.ClubView, error) {
	f.token = token
	c := cats
	c.Name, c.Description, c.Address, c.Members = name, description, "", 1
	return model.ClubView{Club: c, Role: "owner", OwnerID: "u1"}, f.err
}

func (f *fakeClubs) CreateEvent(_ context.Context, token string, e model.NewEvent) (model.ClubView, error) {
	f.token, f.event = token, e
	c := cats
	c.Name, c.Kind, c.StartsAt, c.EndsAt, c.Members = e.Name, model.ClubKindEvent, e.StartsAt, e.EndsAt, 1
	return model.ClubView{Club: c, Role: "owner", OwnerID: "u1"}, f.err
}

func (f *fakeClubs) Get(_ context.Context, token, ref string) (model.ClubView, error) {
	f.token = token
	if ref != "k1" && ref != "cats" {
		return model.ClubView{}, failure.New(failure.NotFound, "club not found")
	}
	return f.view(), nil
}

func (f *fakeClubs) GetMany(_ context.Context, ids []string) ([]model.Club, error) {
	var found []model.Club
	for _, id := range ids {
		if id == "k1" {
			found = append(found, f.view().Club)
		}
	}
	return found, nil
}

func (f *fakeClubs) Update(_ context.Context, token, clubID string, changes model.ClubChanges) (model.ClubView, error) {
	f.token, f.changes = token, changes
	v := f.view()
	v.Club.Name, v.Club.Address, v.Club.Wall = changes.Name, changes.Address, changes.Wall
	return v, f.err
}

func (f *fakeClubs) Join(_ context.Context, token, clubID string, maybe bool) (model.ClubView, error) {
	f.token, f.joined = token, clubID
	v := f.view()
	v.Role, v.Maybe = "member", maybe
	return v, f.err
}

func (f *fakeClubs) Leave(_ context.Context, token, clubID string) (model.ClubView, error) {
	f.token, f.left = token, clubID
	return f.view(), f.err
}

func (f *fakeClubs) Members(_ context.Context, token, clubID, cursor string, limit int, filter model.ClubMemberFilter) (model.ClubMemberPage, error) {
	f.token, f.adminsOnly, f.going = token, filter.AdminsOnly, filter.Going
	return model.ClubMemberPage{Members: f.members, NextCursor: "m1", Total: len(f.members)}, f.err
}

// Transfer lets the owner only, as the club service.
func (f *fakeClubs) Transfer(_ context.Context, token, clubID, userID string) error {
	if f.role != "owner" {
		return failure.New(failure.PermissionDenied, "permission denied")
	}
	f.token, f.transferredTo = token, userID
	return f.err
}

func (f *fakeClubs) OfUser(_ context.Context, userID, cursor string, limit int, filter model.ClubFilter) (model.UserClubPage, error) {
	f.clubsOf, f.kind, f.period = userID, filter.Kind, filter.Period
	return model.UserClubPage{Clubs: []model.Club{cats}, NextCursor: "k1", Total: 4}, f.err
}

func (f *fakeClubs) Describe(_ context.Context, token, clubID, userID, title string, hidden bool) error {
	f.token, f.described = token, model.ClubMember{UserID: userID, Title: title, Hidden: hidden}
	return f.err
}

func (f *fakeClubs) SetAdmin(_ context.Context, token, clubID, userID string, admin bool) error {
	f.token = token
	if f.admin == nil {
		f.admin = map[string]bool{}
	}
	f.admin[userID] = admin
	return f.err
}

func (f *fakeClubs) Mine(_ context.Context, token, cursor string, limit int, filter model.ClubFilter) (model.ClubPage, error) {
	f.token, f.managed, f.kind, f.period = token, filter.Managed, filter.Kind, filter.Period
	return model.ClubPage{Clubs: []model.Membership{{Club: cats, Role: "admin", Pinned: true}}, Total: 3}, f.err
}

func (f *fakeClubs) Count(context.Context) (int, error) {
	return 5, f.err
}

func (f *fakeClubs) Search(_ context.Context, query, cursor string, limit int) (model.ClubSearchPage, error) {
	f.query = query
	if query == "" {
		return model.ClubSearchPage{}, failure.New(failure.InvalidArgument, "search query is empty")
	}
	return model.ClubSearchPage{Clubs: []model.Club{cats}, NextCursor: "k1"}, f.err
}

// SetAvatar lets admins only, as the club service.
func (f *fakeClubs) SetAvatar(_ context.Context, token, clubID, avatarID string) (model.ClubView, error) {
	if f.role != "admin" && f.role != "owner" {
		return model.ClubView{}, failure.New(failure.PermissionDenied, "permission denied")
	}
	f.avatarID = avatarID
	return f.view(), f.err
}

func (f *fakeClubs) SetBackdrop(_ context.Context, _, _, side, photoID string) (model.ClubView, error) {
	if f.role != "admin" && f.role != "owner" {
		return model.ClubView{}, failure.New(failure.PermissionDenied, "permission denied")
	}
	switch side {
	case "left":
		f.backdrop[0] = photoID
	case "right":
		f.backdrop[1] = photoID
	default:
		return model.ClubView{}, failure.New(failure.InvalidArgument, "invalid backdrop side")
	}
	return f.view(), f.err
}

func (f *fakeClubs) Pin(_ context.Context, token, clubID string, pinned bool) error {
	f.pinned = pinned
	return f.err
}

func (f *fakeClubs) Pinned(_ context.Context, token string) ([]model.Club, error) {
	return []model.Club{cats}, f.err
}

func (f *fakeClubs) MineIDs(_ context.Context, token string) ([]string, error) {
	return []string{"k1"}, f.err
}

// MyRoles: the caller is what the test says in the club of cats alone.
func (f *fakeClubs) MyRoles(_ context.Context, token string, clubIDs []string) (map[string]string, error) {
	f.rolesAsked = clubIDs
	if f.role == "" || !slices.Contains(clubIDs, "k1") {
		return nil, f.err
	}
	return map[string]string{"k1": f.role}, f.err
}

// CanPostOnWall lets the admins only for the club or on a wall that is
// not open; others suggest on a "Предложка".
func (f *fakeClubs) CanPostOnWall(_ context.Context, token, clubID string, fromClub bool) (model.WallAccess, error) {
	f.fromClub = fromClub
	admin := f.role == "admin" || f.role == "owner"
	if fromClub || f.closedWall || f.suggestWall {
		return model.WallAccess{Post: admin, Suggest: f.suggestWall && !admin}, nil
	}
	return model.WallAccess{Post: true}, nil
}

type clubJSON struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Address string            `json:"address"`
	Wall    string            `json:"wall"`
	Members int               `json:"members"`
	Role    string            `json:"role"`
	Owner   map[string]string `json:"owner"`
}

// ann is the caller's filled-in profile.
var ann = model.Profile{Name: "Ann", Nickname: "ann"}

func TestClub(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	w := h.do("POST", "/api/v1/clubs", `{"name":"Любители котов","description":"мяу"}`, "Authorization", "Bearer tok")
	got := decode[clubJSON](t, w)
	if w.Code != http.StatusCreated || got.Name != "Любители котов" || got.Role != "owner" || got.Owner["nickname"] != "ann" {
		t.Fatalf("create: %d %+v", w.Code, got)
	}

	// Public, by the address or the ID.
	for _, ref := range []string{"cats", "k1"} {
		w = h.do("GET", "/api/v1/clubs/"+ref, "")
		if got = decode[clubJSON](t, w); w.Code != http.StatusOK || got.ID != "k1" || got.Members != 2 || got.Role != "" || got.Owner["id"] != "u1" {
			t.Fatalf("get %s: %d %+v", ref, w.Code, got)
		}
	}
	if w = h.do("GET", "/api/v1/clubs/dogs", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown club: %d", w.Code)
	}

	w = h.do("PUT", "/api/v1/clubs/cats", `{"name":"Коты","description":"","address":"kotiki","website":"cats.example","wall":"closed","adminsDisplay":"all"}`, "Authorization", "Bearer tok")
	if got = decode[clubJSON](t, w); w.Code != http.StatusOK || got.Address != "kotiki" || got.Wall != "closed" {
		t.Fatalf("update: %d %+v", w.Code, got)
	}
	if h.clubs.changes != (model.ClubChanges{Name: "Коты", Address: "kotiki", Website: "cats.example", Wall: "closed", AdminsDisplay: "all"}) {
		t.Fatalf("changes = %+v", h.clubs.changes)
	}

	w = h.do("PUT", "/api/v1/clubs/cats/membership", "", "Authorization", "Bearer tok")
	if got = decode[clubJSON](t, w); w.Code != http.StatusOK || got.Role != "member" || h.clubs.joined != "k1" {
		t.Fatalf("join: %d %+v", w.Code, got)
	}
	if w = h.do("DELETE", "/api/v1/clubs/cats/membership", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.clubs.left != "k1" {
		t.Fatalf("leave: %d", w.Code)
	}

	if w = h.do("PUT", "/api/v1/clubs/cats/admins/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || !h.clubs.admin["u2"] {
		t.Fatalf("appoint: %d %v", w.Code, h.clubs.admin)
	}
	if w = h.do("DELETE", "/api/v1/clubs/cats/admins/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.clubs.admin["u2"] {
		t.Fatalf("dismiss: %d %v", w.Code, h.clubs.admin)
	}
}

func TestClub_NeedsProfile(t *testing.T) {
	t.Parallel()

	for _, call := range []struct{ method, path, body string }{
		{"POST", "/api/v1/clubs", `{"name":"x"}`},
		{"PUT", "/api/v1/clubs/cats/membership", ""},
		{"POST", "/api/v1/clubs/cats/posts", `{"text":"x"}`},
	} {
		h := newHarness(rest.Config{})
		if w := h.do(call.method, call.path, call.body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Errorf("%s %s without a profile: %d", call.method, call.path, w.Code)
		}
	}
}

func TestClubMembers(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.clubs.members = []model.ClubMember{{UserID: "u2", Role: "admin", Title: "кормит котов", Hidden: true}, {UserID: "u3", Role: "member"}, {UserID: "u1", Role: "owner"}}

	w := h.do("GET", "/api/v1/clubs/cats/members?admins=true", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Members []struct {
			User   map[string]any `json:"user"`
			Role   string         `json:"role"`
			Title  string         `json:"title"`
			Hidden bool           `json:"hidden"`
		} `json:"members"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)

	// Cat's page is deleted: she is left out, but counted.
	if w.Code != http.StatusOK || len(got.Members) != 2 || got.Members[0].User["nickname"] != "bob" || got.Members[0].Role != "admin" || got.NextCursor != "m1" || got.Total != 3 || !h.clubs.adminsOnly {
		t.Fatalf("%d %+v", w.Code, got)
	}
	// The viewer's token goes along: the club tells its admins who is hidden.
	if got.Members[0].Title != "кормит котов" || !got.Members[0].Hidden || h.clubs.token != "tok" {
		t.Fatalf("the admin = %+v, token %q", got.Members[0], h.clubs.token)
	}

	w = h.do("PATCH", "/api/v1/clubs/cats/admins/bob", `{"title":"главный по котам","hidden":true}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.clubs.described != (model.ClubMember{UserID: "u2", Title: "главный по котам", Hidden: true}) {
		t.Fatalf("describe: %d %s, %+v", w.Code, w.Body, h.clubs.described)
	}
	if w = h.do("PATCH", "/api/v1/clubs/cats/admins/bob", `{"title":""}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("describe without a token: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/me/clubs", "", "Authorization", "Bearer tok")
	mine := decode[struct {
		Clubs []clubJSON `json:"clubs"`
	}](t, w)
	if w.Code != http.StatusOK || len(mine.Clubs) != 1 || mine.Clubs[0].Role != "admin" || mine.Clubs[0].Address != "cats" {
		t.Fatalf("my clubs: %d %+v", w.Code, mine)
	}
}

func TestClubWall(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/clubs/cats/posts", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Posts []struct {
			ID       string              `json:"id"`
			Author   map[string]any      `json:"author"`
			Wall     map[string]any      `json:"wall"`
			Photos   []map[string]string `json:"photos"`
			FromWall bool                `json:"fromWall"`
		} `json:"posts"`
		CanPost bool `json:"canPost"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Posts) != 2 || !got.CanPost {
		t.Fatalf("%d %+v", w.Code, got)
	}

	// The club's post is shown by the club, its writer not told but in
	// the photos, filed under the writer.
	news := got.Posts[0]
	if !news.FromWall || news.Author["club"] != true || news.Author["name"] != "Кошки" || news.Author["address"] != "cats" || news.Wall != nil {
		t.Fatalf("the club's post = %+v", news)
	}
	if len(news.Photos) != 1 || news.Photos[0]["large"] != "/media/photos/u1/ph1/large.jpg" {
		t.Fatalf("photos = %+v", news.Photos)
	}
	if bobs := got.Posts[1]; bobs.Author["nickname"] != "bob" || bobs.Wall["club"] != true {
		t.Fatalf("bob's post = %+v", bobs)
	}
}

func TestCreateClubPost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.role = "admin"

	w := h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"новости","fromClub":true}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Author   map[string]any `json:"author"`
		Wall     map[string]any `json:"wall"`
		FromWall bool           `json:"fromWall"`
		Mine     bool           `json:"mine"`
	}](t, w)
	if w.Code != http.StatusCreated || !got.FromWall || !got.Mine || got.Author["name"] != "Кошки" || got.Wall != nil {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if np := h.posts.newPost; np.WallID != "k1" || !np.ClubWall || !np.FromWall || !h.clubs.fromClub {
		t.Fatalf("post = %+v", np)
	}

	h.clubs.role = "member"
	if w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"я за клуб","fromClub":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member for the club: %d", w.Code)
	}

	w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"привет"}`, "Authorization", "Bearer tok")
	got = decode[struct {
		Author   map[string]any `json:"author"`
		Wall     map[string]any `json:"wall"`
		FromWall bool           `json:"fromWall"`
		Mine     bool           `json:"mine"`
	}](t, w)
	if w.Code != http.StatusCreated || got.FromWall || got.Author["nickname"] != "ann" || got.Wall["club"] != true {
		t.Fatalf("a member's post: %d %+v", w.Code, got)
	}

	h.clubs.closedWall = true
	if w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"привет"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a closed wall: %d", w.Code)
	}
}

func TestFeed_Clubs(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("GET", "/api/v1/feed", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || !slices.Equal(h.posts.walls, []string{"k1", "u2", "gone", "u1"}) {
		t.Fatalf("feed: %d, walls %v: the clubs, the followed and me", w.Code, h.posts.walls)
	}

	// Whoever runs a community moderates its wall from the news: the
	// posts of others there and those in its name, not the walls of people.
	feed := func(role string) map[string]bool {
		t.Helper()

		h := newHarness(rest.Config{})
		h.clubs.role = role
		h.posts.feedMore = []model.Post{
			{ID: "p6", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "k1"}, Text: "в группе"},
			{ID: "p7", Author: model.Author{ID: "k1"}, Wall: model.Author{ID: "k1"}, Text: "от группы", FromWall: true},
		}
		w := h.do("GET", "/api/v1/feed", "", "Authorization", "Bearer tok")
		got := decode[struct {
			Posts []struct {
				ID        string `json:"id"`
				Moderated bool   `json:"moderated"`
			} `json:"posts"`
		}](t, w)
		if w.Code != http.StatusOK || !slices.Equal(h.clubs.rolesAsked, []string{"k1"}) {
			t.Fatalf("feed as %q: %d, roles asked in %v", role, w.Code, h.clubs.rolesAsked)
		}

		moderated := map[string]bool{}
		for _, p := range got.Posts {
			moderated[p.ID] = p.Moderated
		}
		return moderated
	}
	if got := feed("admin"); !got["p6"] || !got["p7"] || got["p3"] || got["p1"] {
		t.Fatalf("an admin moderates %v", got)
	}
	if got := feed("member"); got["p6"] || got["p7"] {
		t.Fatalf("a member moderates %v", got)
	}
}

func TestClubAvatar(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/clubs/cats/avatar", "jpeg", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden || h.media.uploaded != nil {
		t.Fatalf("a member uploads: %d, uploaded %q", w.Code, h.media.uploaded)
	}

	h.clubs.role = "admin"
	w := h.do("PUT", "/api/v1/clubs/cats/avatar", "jpeg", "Authorization", "Bearer tok")
	got := decode[struct {
		Avatar map[string]string `json:"avatar"`
	}](t, w)
	if w.Code != http.StatusOK || got.Avatar["small"] != "/media/avatars/k1/ka1/small.jpg" || h.media.clubAvatarOf != "k1" || h.media.kept != "ka1" {
		t.Fatalf("upload: %d %+v, media %+v", w.Code, got, h.media)
	}
	if len(h.albums.avatarPhotos) != 1 || h.albums.avatarPhotos[0] != "k1/ph9" {
		t.Fatalf("the club's avatar album got %v", h.albums.avatarPhotos)
	}

	// The club's photo comes with its posts too.
	w = h.do("GET", "/api/v1/clubs/cats/posts", "")
	posts := decode[struct {
		Posts []struct {
			Author map[string]any `json:"author"`
		} `json:"posts"`
	}](t, w)
	if avatar, _ := posts.Posts[0].Author["avatar"].(map[string]any); avatar["small"] != "/media/avatars/k1/ka1/small.jpg" {
		t.Fatalf("the club's post = %+v", posts.Posts[0])
	}

	if h.posts.newPost.AvatarUpdate {
		t.Fatal("posted without onWall")
	}
	if w = h.do("PUT", "/api/v1/clubs/cats/avatar?onWall=true", "jpeg", "Authorization", "Bearer tok"); w.Code != http.StatusOK {
		t.Fatalf("upload on the wall: %d", w.Code)
	}
	if np := h.posts.newPost; !np.AvatarUpdate || np.WallID != "k1" || !np.ClubWall || !np.FromWall || len(np.PhotoIDs) != 1 {
		t.Fatalf("the club's post = %+v", np)
	}

	if w = h.do("DELETE", "/api/v1/clubs/cats/avatar", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.clubs.avatarID != "" || h.media.kept != "" {
		t.Fatalf("delete: %d, club %+v, media %+v", w.Code, h.clubs, h.media)
	}
}

func TestClubSearchAndMenu(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/clubs?q=кошки", "")
	found := decode[struct {
		Clubs      []clubJSON `json:"clubs"`
		NextCursor string     `json:"nextCursor"`
	}](t, w)
	if w.Code != http.StatusOK || len(found.Clubs) != 1 || found.Clubs[0].Address != "cats" || found.NextCursor != "k1" || h.clubs.query != "кошки" {
		t.Fatalf("search: %d %+v", w.Code, found)
	}
	if w = h.do("GET", "/api/v1/clubs?q=", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("an empty query: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/me/clubs?managed=true", "", "Authorization", "Bearer tok")
	mine := decode[struct {
		Clubs []struct {
			Pinned bool `json:"pinned"`
		} `json:"clubs"`
		Total int `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || !h.clubs.managed || mine.Total != 3 || !mine.Clubs[0].Pinned {
		t.Fatalf("managed: %d %+v", w.Code, mine)
	}

	if w = h.do("PUT", "/api/v1/clubs/cats/pin", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || !h.clubs.pinned {
		t.Fatalf("pin: %d", w.Code)
	}
	if w = h.do("DELETE", "/api/v1/clubs/cats/pin", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.clubs.pinned {
		t.Fatalf("unpin: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/me/clubs/pinned", "", "Authorization", "Bearer tok")
	pinned := decode[struct {
		Clubs []clubJSON `json:"clubs"`
	}](t, w)
	if w.Code != http.StatusOK || len(pinned.Clubs) != 1 || pinned.Clubs[0].Name != "Кошки" {
		t.Fatalf("pinned: %d %+v", w.Code, pinned)
	}
}

func TestClubPost_Signed(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.role = "admin"

	w := h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"новости","fromClub":true,"signed":true}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Author map[string]any `json:"author"`
		Signer map[string]any `json:"signer"`
	}](t, w)
	if w.Code != http.StatusCreated || got.Author["club"] != true || got.Signer["nickname"] != "ann" || !h.posts.newPost.Signed {
		t.Fatalf("%d %+v", w.Code, got)
	}

	// A signature needs a post for the club.
	if w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"я","signed":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || h.posts.newPost.Signed {
		t.Fatalf("signed but not for the club: %d %+v", w.Code, h.posts.newPost)
	}

	// On a closed wall the admins always speak for the club, as in openvk.
	h.clubs.closedWall = true
	if w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"новости"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || !h.posts.newPost.FromWall {
		t.Fatalf("closed wall: %d %+v", w.Code, h.posts.newPost)
	}
}

func TestTransferClub(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.clubs.role = "admin"
	if w := h.do("PUT", "/api/v1/clubs/cats/owner/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden || h.clubs.transferredTo != "" {
		t.Fatalf("an admin hands it over: %d", w.Code)
	}

	h.clubs.role = "owner"
	if w := h.do("PUT", "/api/v1/clubs/cats/owner/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.clubs.transferredTo != "u2" {
		t.Fatalf("transfer: %d, to %q", w.Code, h.clubs.transferredTo)
	}
	if w := h.do("PUT", "/api/v1/clubs/cats/owner/nobody", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("to nobody: %d", w.Code)
	}
	if w := h.do("PUT", "/api/v1/clubs/cats/owner/bob", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

func TestUserClubs(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	// Public, as the user's setting allows.
	w := h.do("GET", "/api/v1/users/bob/clubs", "")
	got := decode[struct {
		Clubs []struct {
			Name   string            `json:"name"`
			Avatar map[string]string `json:"avatar"`
			Role   string            `json:"role"`
		} `json:"clubs"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Clubs) != 1 || got.Clubs[0].Name != "Кошки" || got.Total != 4 || got.NextCursor != "k1" ||
		h.clubs.clubsOf != "u2" || h.users.accessAsked != "u2" || got.Clubs[0].Role != "" || h.clubs.rolesAsked != nil {
		t.Fatalf("%d %+v", w.Code, got)
	}

	// A viewer who is in the club is told so, to leave it from the list.
	h.clubs.role = "member"
	w = h.do("GET", "/api/v1/users/bob/clubs", "", "Authorization", "Bearer tok")
	if got = decode[struct {
		Clubs []struct {
			Name   string            `json:"name"`
			Avatar map[string]string `json:"avatar"`
			Role   string            `json:"role"`
		} `json:"clubs"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w); w.Code != http.StatusOK || got.Clubs[0].Role != "member" {
		t.Fatalf("as a member: %d %+v", w.Code, got)
	}
	h.clubs.role = ""

	h.users.hiddenClubs = true
	h.clubs.clubsOf = ""
	if w = h.do("GET", "/api/v1/users/bob/clubs", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden || h.clubs.clubsOf != "" || h.users.token != "tok" {
		t.Fatalf("hidden clubs: %d, listed for %q", w.Code, h.clubs.clubsOf)
	}
	if w = h.do("GET", "/api/v1/users/nobody/clubs", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
}

func TestClubSuggestions(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.role = "member"

	// An open wall takes posts, not suggestions.
	if w := h.do("POST", "/api/v1/clubs/cats/suggestions", `{"text":"котик"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("suggest on an open wall: %d", w.Code)
	}

	h.clubs.suggestWall = true
	w := h.do("GET", "/api/v1/clubs/cats/posts", "", "Authorization", "Bearer tok")
	wall := decode[struct {
		CanPost    bool `json:"canPost"`
		CanSuggest bool `json:"canSuggest"`
	}](t, w)
	if w.Code != http.StatusOK || wall.CanPost || !wall.CanSuggest {
		t.Fatalf("a member on a suggest wall: %d %+v", w.Code, wall)
	}
	if w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"котик"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member posts on a suggest wall: %d", w.Code)
	}

	w = h.do("POST", "/api/v1/clubs/cats/suggestions", `{"text":"котик","photos":["ph1"]}`, "Authorization", "Bearer tok")
	got := decode[struct {
		ID     string              `json:"id"`
		Author map[string]any      `json:"author"`
		Text   string              `json:"text"`
		Photos []map[string]string `json:"photos"`
	}](t, w)
	if w.Code != http.StatusCreated || got.ID != "s1" || got.Author["nickname"] != "ann" || got.Text != "котик" ||
		len(got.Photos) != 1 || got.Photos[0]["large"] != "/media/photos/u1/ph1/large.jpg" || h.posts.suggested.WallID != "k1" {
		t.Fatalf("suggest: %d %+v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/clubs/cats/suggestions?mine=true", "", "Authorization", "Bearer tok")
	list := decode[struct {
		Suggestions []struct {
			ID     string         `json:"id"`
			Author map[string]any `json:"author"`
		} `json:"suggestions"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)
	// Cat's page is deleted: her suggestion is left out, but counted.
	if w.Code != http.StatusOK || len(list.Suggestions) != 1 || list.Suggestions[0].Author["nickname"] != "bob" || list.Total != 2 ||
		list.NextCursor != "s3" || !h.posts.mine {
		t.Fatalf("list: %d %+v", w.Code, list)
	}

	w = h.do("POST", "/api/v1/suggestions/s2/publish", `{"text":"от Петра!","signed":true}`, "Authorization", "Bearer tok")
	published := decode[struct {
		ID       string         `json:"id"`
		Author   map[string]any `json:"author"`
		Signer   map[string]any `json:"signer"`
		FromWall bool           `json:"fromWall"`
	}](t, w)
	if w.Code != http.StatusCreated || published.ID != "p8" || !published.FromWall || published.Author["club"] != true ||
		published.Signer["nickname"] != "bob" || h.posts.published != "s2" || !h.posts.signed {
		t.Fatalf("publish: %d %+v", w.Code, published)
	}

	if w = h.do("DELETE", "/api/v1/suggestions/s2", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.withdrawn != "s2" {
		t.Fatalf("delete: %d", w.Code)
	}
	if w = h.do("GET", "/api/v1/clubs/cats/suggestions", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("suggestions without a token: %d", w.Code)
	}
}

func TestCreateClubPost_SuggestWall(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.role = "admin"
	h.clubs.suggestWall = true

	// As in openvk, admins speak for a club whose wall is not open.
	if w := h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"новости"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || !h.posts.newPost.FromWall {
		t.Fatalf("an admin on a suggest wall: %d %+v", w.Code, h.posts.newPost)
	}
}

func TestComment_ForClub(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	// Only the club's admins speak for it, and only on its wall.
	h.clubs.role = "member"
	if w := h.do("POST", "/api/v1/posts/p5/comments", `{"text":"от группы","fromClub":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member for the club: %d", w.Code)
	}
	h.clubs.role = "admin"
	if w := h.do("POST", "/api/v1/posts/p4/comments", `{"text":"от группы","fromClub":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("for a user's wall: %d", w.Code)
	}

	w := h.do("POST", "/api/v1/posts/p5/comments", `{"text":"от группы","photos":["ph1"],"fromClub":true}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Author   map[string]any      `json:"author"`
		Photos   []map[string]string `json:"photos"`
		FromWall bool                `json:"fromWall"`
		Mine     bool                `json:"mine"`
	}](t, w)
	if w.Code != http.StatusCreated || got.Author["club"] != true || got.Author["name"] != "Кошки" || !got.FromWall || !got.Mine ||
		got.Photos[0]["large"] != "/media/photos/u1/ph1/large.jpg" || !h.posts.newComment.FromWall {
		t.Fatalf("for the club: %d %+v", w.Code, got)
	}

	// Readers see the club, not who wrote it; photos stay under the writer.
	w = h.do("GET", "/api/v1/posts/p5/comments", "")
	page := decode[struct {
		Comments []struct {
			Author   map[string]any      `json:"author"`
			Photos   []map[string]string `json:"photos"`
			FromWall bool                `json:"fromWall"`
			Mine     bool                `json:"mine"`
		} `json:"comments"`
	}](t, w)
	if w.Code != http.StatusOK || len(page.Comments) != 1 || page.Comments[0].Author["club"] != true || page.Comments[0].Author["id"] != "k1" ||
		page.Comments[0].Mine || page.Comments[0].Photos[0]["large"] != "/media/photos/u1/ph1/large.jpg" {
		t.Fatalf("list: %d %+v", w.Code, page)
	}
}

func TestClubEvents(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	type eventJSON struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Kind     string  `json:"kind"`
		StartsAt *string `json:"startsAt"`
		EndsAt   *string `json:"endsAt"`
		Role     string  `json:"role"`
		Maybe    bool    `json:"maybe"`
	}

	w := h.do("POST", "/api/v1/events", `{"name":"Новый год","description":"ёлка","startsAt":"2026-12-31T20:00:00+03:00"}`, "Authorization", "Bearer tok")
	got := decode[eventJSON](t, w)
	if w.Code != http.StatusCreated || got.Kind != "event" || got.StartsAt == nil || *got.StartsAt != "2026-12-31T17:00:00Z" || got.EndsAt != nil || got.Role != "owner" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if h.clubs.event.Name != "Новый год" || h.clubs.event.Description != "ёлка" || !h.clubs.event.EndsAt.IsZero() {
		t.Fatalf("the event sent = %+v", h.clubs.event)
	}

	if w = h.do("POST", "/api/v1/events", `{"name":"Новый год","startsAt":"завтра"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad start: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/events", `{"name":"Новый год","startsAt":"2026-12-31T20:00:00Z"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	// "Возможно приду" and the two lists of those who come.
	w = h.do("PUT", "/api/v1/clubs/cats/membership?maybe=true", "", "Authorization", "Bearer tok")
	if joined := decode[eventJSON](t, w); w.Code != http.StatusOK || !joined.Maybe || joined.Role != "member" {
		t.Fatalf("a maybe: %d %s", w.Code, w.Body)
	}
	if w = h.do("GET", "/api/v1/clubs/cats/members?going=maybe", ""); w.Code != http.StatusOK || h.clubs.going != "maybe" || h.clubs.adminsOnly {
		t.Fatalf("those who might come: %d, going %q", w.Code, h.clubs.going)
	}

	// "Мои Встречи" are the caller's events, "Мои Группы" the groups.
	if w = h.do("GET", "/api/v1/me/events?managed=true", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.clubs.kind != "event" || !h.clubs.managed {
		t.Fatalf("my events: %d, kind %q", w.Code, h.clubs.kind)
	}
	if w = h.do("GET", "/api/v1/me/clubs", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.clubs.kind != "group" || h.clubs.managed {
		t.Fatalf("my groups: %d, kind %q", w.Code, h.clubs.kind)
	}

	// Those ahead and those over are told apart; groups have no time.
	if w = h.do("GET", "/api/v1/me/events?time=past", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.clubs.period != "past" {
		t.Fatalf("my past events: %d, period %q", w.Code, h.clubs.period)
	}
	if w = h.do("GET", "/api/v1/me/clubs?time=past", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.clubs.period != "" {
		t.Fatalf("my groups by time: %d, period %q", w.Code, h.clubs.period)
	}

	// The events of a person are shown as their groups are.
	w = h.do("GET", "/api/v1/users/bob/events?time=upcoming", "")
	if w.Code != http.StatusOK || h.clubs.kind != "event" || h.clubs.period != "upcoming" || h.clubs.clubsOf != "u2" {
		t.Fatalf("bob's events: %d, kind %q, period %q, of %q", w.Code, h.clubs.kind, h.clubs.period, h.clubs.clubsOf)
	}
	if w = h.do("GET", "/api/v1/users/bob/events?time=never", ""); w.Code != http.StatusOK || h.clubs.period != "" {
		t.Fatalf("bob's events of no time: %d, period %q", w.Code, h.clubs.period)
	}

	// An event moves with its settings.
	h.clubs.role = "owner"
	w = h.do("PUT", "/api/v1/clubs/cats", `{"name":"Новый год","wall":"open","adminsDisplay":"creator","startsAt":"2027-01-01T00:00:00Z","endsAt":"2027-01-01T03:00:00Z"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.clubs.changes.StartsAt.Year() != 2027 || h.clubs.changes.EndsAt.Hour() != 3 {
		t.Fatalf("settings: %d %s, %+v", w.Code, w.Body, h.clubs.changes)
	}
}
