package rest_test

import (
	"context"
	"net/http"
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
	role                 string
	closedWall           bool
	err                  error
}

var cats = model.Club{ID: "k1", Name: "Кошки", Address: "cats", Wall: "open", Members: 2, CreatedAt: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}

func (f *fakeClubs) view() model.ClubView {
	return model.ClubView{Club: cats, Role: f.role, OwnerID: "u1"}
}

func (f *fakeClubs) Create(_ context.Context, token, name, description string) (model.ClubView, error) {
	f.token = token
	c := cats
	c.Name, c.Description, c.Address, c.Members = name, description, "", 1
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
			found = append(found, cats)
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

func (f *fakeClubs) Join(_ context.Context, token, clubID string) (model.ClubView, error) {
	f.token, f.joined = token, clubID
	v := f.view()
	v.Role = "member"
	return v, f.err
}

func (f *fakeClubs) Leave(_ context.Context, token, clubID string) (model.ClubView, error) {
	f.token, f.left = token, clubID
	return f.view(), f.err
}

func (f *fakeClubs) Members(_ context.Context, clubID, cursor string, limit int, adminsOnly bool) (model.ClubMemberPage, error) {
	f.adminsOnly = adminsOnly
	return model.ClubMemberPage{Members: f.members, NextCursor: "m1"}, f.err
}

func (f *fakeClubs) SetAdmin(_ context.Context, token, clubID, userID string, admin bool) error {
	f.token = token
	if f.admin == nil {
		f.admin = map[string]bool{}
	}
	f.admin[userID] = admin
	return f.err
}

func (f *fakeClubs) Mine(_ context.Context, token, cursor string, limit int) (model.ClubPage, error) {
	f.token = token
	return model.ClubPage{Clubs: []model.Membership{{Club: cats, Role: "admin"}}}, f.err
}

func (f *fakeClubs) MineIDs(_ context.Context, token string) ([]string, error) {
	return []string{"k1"}, f.err
}

func (f *fakeClubs) CanPostOnWall(_ context.Context, token, clubID string, fromClub bool) (bool, error) {
	f.fromClub = fromClub
	if fromClub || f.closedWall {
		return f.role == "admin" || f.role == "owner", nil
	}
	return true, nil
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

	w = h.do("PUT", "/api/v1/clubs/cats", `{"name":"Коты","description":"","address":"kotiki","wall":"closed"}`, "Authorization", "Bearer tok")
	if got = decode[clubJSON](t, w); w.Code != http.StatusOK || got.Address != "kotiki" || got.Wall != "closed" {
		t.Fatalf("update: %d %+v", w.Code, got)
	}
	if h.clubs.changes != (model.ClubChanges{Name: "Коты", Address: "kotiki", Wall: "closed"}) {
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
	h.clubs.members = []model.ClubMember{{UserID: "u2", Role: "admin"}, {UserID: "u3", Role: "member"}, {UserID: "u1", Role: "owner"}}

	w := h.do("GET", "/api/v1/clubs/cats/members?admins=true", "")
	got := decode[struct {
		Members []struct {
			User map[string]any `json:"user"`
			Role string         `json:"role"`
		} `json:"members"`
		NextCursor string `json:"nextCursor"`
	}](t, w)

	// Cat's page is deleted: she is left out.
	if w.Code != http.StatusOK || len(got.Members) != 2 || got.Members[0].User["nickname"] != "bob" || got.Members[0].Role != "admin" || got.NextCursor != "m1" || !h.clubs.adminsOnly {
		t.Fatalf("%d %+v", w.Code, got)
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
	if w := h.do("GET", "/api/v1/feed", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || len(h.posts.walls) != 1 || h.posts.walls[0] != "k1" {
		t.Fatalf("feed: %d, walls %v", w.Code, h.posts.walls)
	}
}
