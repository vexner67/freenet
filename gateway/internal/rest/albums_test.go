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

// fakeAlbums knows a1 of ann (u1), a2 of the club k1 with photos of bob
// (u2) and of cat (u3, a deleted page), and a3 of cat.
type fakeAlbums struct {
	ownerID, deleted, described string
	created                     model.NewAlbum
	edited                      [2]string
	added                       []model.NewAlbumPhoto
	err                         error
}

var albumTime = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func (f *fakeAlbums) album(id string) (model.Album, error) {
	switch id {
	case "a1":
		return model.Album{ID: "a1", OwnerID: "u1", Name: "Лето", Photos: 2, CreatedAt: albumTime, UpdatedAt: albumTime,
			Cover: &model.AlbumPhoto{ID: "ph1", AlbumID: "a1", AuthorID: "u1"}}, nil
	case "a2":
		return model.Album{ID: "a2", OwnerID: "k1", Name: "Кошки", Photos: 2, CreatedAt: albumTime, UpdatedAt: albumTime}, nil
	case "a3":
		return model.Album{ID: "a3", OwnerID: "u3", Name: "Пропавший", CreatedAt: albumTime, UpdatedAt: albumTime}, nil
	default:
		return model.Album{}, failure.New(failure.NotFound, "album not found")
	}
}

func (f *fakeAlbums) photo(id string) (model.AlbumPhoto, error) {
	switch id {
	case "ph6":
		return model.AlbumPhoto{ID: "ph6", AlbumID: "a2", AuthorID: "u2", Description: "Барсик", CreatedAt: albumTime}, nil
	case "ph5":
		return model.AlbumPhoto{ID: "ph5", AlbumID: "a2", AuthorID: "u3", CreatedAt: albumTime}, nil
	default:
		return model.AlbumPhoto{}, failure.New(failure.NotFound, "photo not found")
	}
}

func (f *fakeAlbums) Create(_ context.Context, _ string, na model.NewAlbum) (model.Album, error) {
	f.created = na
	owner := "u1"
	if na.ClubID != "" {
		owner = na.ClubID
	}
	return model.Album{ID: "a9", OwnerID: owner, Name: na.Name, Description: na.Description, CreatedAt: albumTime, UpdatedAt: albumTime}, f.err
}

func (f *fakeAlbums) Get(_ context.Context, _, id string) (model.Album, error) {
	return f.album(id)
}

func (f *fakeAlbums) List(_ context.Context, _, ownerID, _ string, _ int) (model.AlbumPage, error) {
	f.ownerID = ownerID
	page := model.AlbumPage{Total: 1}
	for _, id := range []string{"a1", "a2"} {
		if a, _ := f.album(id); a.OwnerID == ownerID {
			page.Albums = append(page.Albums, a)
		}
	}
	return page, f.err
}

func (f *fakeAlbums) Edit(_ context.Context, _, id, name, description string) (model.Album, error) {
	f.edited = [2]string{name, description}
	return f.album(id)
}

func (f *fakeAlbums) Delete(_ context.Context, _, id string) error {
	f.deleted = id
	return f.err
}

func (f *fakeAlbums) AddPhotos(_ context.Context, _, albumID string, photos []model.NewAlbumPhoto) ([]model.AlbumPhoto, error) {
	f.added = photos
	added := make([]model.AlbumPhoto, 0, len(photos))
	for _, p := range photos {
		added = append(added, model.AlbumPhoto{ID: p.PhotoID, AlbumID: albumID, AuthorID: "u1", Description: p.Description, CreatedAt: albumTime, Mine: true})
	}
	return added, f.err
}

func (f *fakeAlbums) Photos(_ context.Context, _, albumID, _ string, _ int) (model.AlbumPhotoPage, error) {
	if albumID != "a2" {
		return model.AlbumPhotoPage{}, f.err
	}
	page := model.AlbumPhotoPage{Total: 2}
	for _, id := range []string{"ph6", "ph5"} {
		p, _ := f.photo(id)
		page.Photos = append(page.Photos, p)
	}
	return page, f.err
}

func (f *fakeAlbums) Photo(_ context.Context, _, id string) (model.AlbumPhoto, model.Album, error) {
	p, err := f.photo(id)
	if err != nil {
		return model.AlbumPhoto{}, model.Album{}, err
	}
	a, _ := f.album(p.AlbumID)
	return p, a, nil
}

func (f *fakeAlbums) Describe(_ context.Context, _, id, description string) (model.AlbumPhoto, error) {
	f.described = description
	return f.photo(id)
}

func (f *fakeAlbums) DeletePhoto(_ context.Context, _, id string) error {
	f.deleted = id
	return f.err
}

type albumJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Photos int    `json:"photos"`
	Cover  *struct {
		Large string `json:"large"`
		Small string `json:"small"`
	} `json:"cover"`
}

type albumOwnerJSON struct {
	Owner map[string]any `json:"owner"`
	Role  string         `json:"role"`
}

func TestListAlbums(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann/albums", "")
	got := decode[struct {
		albumOwnerJSON
		Albums []albumJSON `json:"albums"`
		Total  int         `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || h.albums.ownerID != "u1" || got.Owner["nickname"] != "ann" || got.Total != 1 || len(got.Albums) != 1 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if cover := got.Albums[0].Cover; cover == nil || cover.Small != "/media/photos/u1/ph1/small.jpg" {
		t.Fatalf("cover = %+v", cover)
	}

	if w = h.do("GET", "/api/v1/users/cat/albums", ""); w.Code != http.StatusForbidden {
		t.Fatalf("a deleted page: %d", w.Code)
	}
	if w = h.do("GET", "/api/v1/users/nobody/albums", ""); w.Code != http.StatusNotFound {
		t.Fatalf("nobody: %d", w.Code)
	}

	h.clubs.role = "admin"
	w = h.do("GET", "/api/v1/clubs/cats/albums", "", "Authorization", "Bearer tok")
	clubs := decode[struct {
		albumOwnerJSON
		Albums []albumJSON `json:"albums"`
	}](t, w)
	if w.Code != http.StatusOK || h.albums.ownerID != "k1" || clubs.Owner["club"] != true || clubs.Role != "admin" || clubs.Albums[0].Cover != nil {
		t.Fatalf("the club's: %d %+v", w.Code, clubs)
	}
}

func TestCreateAlbum(t *testing.T) {
	t.Parallel()

	body := `{"name":"Лето","description":"На даче"}`

	h := newHarness(rest.Config{})
	if w := h.do("POST", "/api/v1/me/albums", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
	if w := h.do("POST", "/api/v1/me/albums", body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("without a profile: %d", w.Code)
	}

	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	w := h.do("POST", "/api/v1/me/albums", body, "Authorization", "Bearer tok")
	if got := decode[albumJSON](t, w); w.Code != http.StatusCreated || got.Name != "Лето" || h.albums.created != (model.NewAlbum{Name: "Лето", Description: "На даче"}) {
		t.Fatalf("own: %d %+v, created %+v", w.Code, got, h.albums.created)
	}

	if w = h.do("POST", "/api/v1/clubs/cats/albums", body, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || h.albums.created.ClubID != "k1" {
		t.Fatalf("the club's: %d, created %+v", w.Code, h.albums.created)
	}
	if w = h.do("POST", "/api/v1/clubs/nobody/albums", body, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("no club: %d", w.Code)
	}
}

func TestAlbumPage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/albums/a1", "")
	got := decode[struct {
		albumOwnerJSON
		Album albumJSON `json:"album"`
	}](t, w)
	if w.Code != http.StatusOK || got.Album.Name != "Лето" || got.Owner["nickname"] != "ann" || got.Role != "" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	h.clubs.role = "owner"
	w = h.do("GET", "/api/v1/albums/a2", "", "Authorization", "Bearer tok")
	if got = decode[struct {
		albumOwnerJSON
		Album albumJSON `json:"album"`
	}](t, w); w.Code != http.StatusOK || got.Owner["name"] != "Кошки" || got.Role != "owner" {
		t.Fatalf("the club's: %d %+v", w.Code, got)
	}

	if w = h.do("GET", "/api/v1/albums/a3", ""); w.Code != http.StatusNotFound {
		t.Fatalf("an album of a deleted page: %d", w.Code)
	}
	if w = h.do("GET", "/api/v1/albums/a3/photos", ""); w.Code != http.StatusNotFound {
		t.Fatalf("photos of a deleted page: %d", w.Code)
	}

	w = h.do("PATCH", "/api/v1/albums/a1", `{"name":"Осень","description":"Листья"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.albums.edited != [2]string{"Осень", "Листья"} {
		t.Fatalf("edit: %d, edited %v", w.Code, h.albums.edited)
	}
	if w = h.do("DELETE", "/api/v1/albums/a1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.albums.deleted != "a1" {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestAlbumPhotos(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/albums/a2/photos", "")
	got := decode[struct {
		Photos []struct {
			ID          string `json:"id"`
			Large       string `json:"large"`
			Description string `json:"description"`
		} `json:"photos"`
		Total int `json:"total"`
	}](t, w)
	// cat's photo is left out: the page is deleted.
	if w.Code != http.StatusOK || got.Total != 2 || len(got.Photos) != 1 || got.Photos[0].Large != "/media/photos/u2/ph6/large.jpg" || got.Photos[0].Description != "Барсик" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	body := `{"photos":[{"id":"ph1","description":"Море"},{"id":"ph2"}]}`
	if w = h.do("POST", "/api/v1/albums/a1/photos", body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("without a profile: %d", w.Code)
	}

	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	if w = h.do("POST", "/api/v1/albums/a1/photos", `{"photos":[{"id":"ph9"}]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.albums.added != nil {
		t.Fatalf("someone else's photo: %d", w.Code)
	}

	w = h.do("POST", "/api/v1/albums/a1/photos", body, "Authorization", "Bearer tok")
	added := decode[struct {
		Photos []struct {
			Small string `json:"small"`
			Mine  bool   `json:"mine"`
		} `json:"photos"`
	}](t, w)
	if w.Code != http.StatusCreated || len(added.Photos) != 2 || !added.Photos[0].Mine || added.Photos[1].Small != "/media/photos/u1/ph2/small.jpg" {
		t.Fatalf("add: %d %+v", w.Code, added)
	}
	if want := []model.NewAlbumPhoto{{PhotoID: "ph1", Description: "Море"}, {PhotoID: "ph2"}}; len(h.albums.added) != 2 || h.albums.added[0] != want[0] || h.albums.added[1] != want[1] {
		t.Fatalf("added %+v", h.albums.added)
	}
}

func TestAlbumPhotoPage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/album-photos/ph6", "")
	got := decode[struct {
		albumOwnerJSON
		Photo struct {
			Large       string `json:"large"`
			Description string `json:"description"`
		} `json:"photo"`
		Author map[string]any `json:"author"`
		Album  albumJSON      `json:"album"`
	}](t, w)
	if w.Code != http.StatusOK || got.Author["nickname"] != "bob" || got.Album.ID != "a2" || got.Owner["club"] != true || got.Photo.Large != "/media/photos/u2/ph6/large.jpg" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	if w = h.do("GET", "/api/v1/album-photos/ph5", ""); w.Code != http.StatusNotFound {
		t.Fatalf("a photo of a deleted page: %d", w.Code)
	}

	if w = h.do("PATCH", "/api/v1/album-photos/ph6", `{"description":"Мурзик"}`, "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.albums.described != "Мурзик" {
		t.Fatalf("describe: %d", w.Code)
	}
	if w = h.do("DELETE", "/api/v1/album-photos/ph6", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.albums.deleted != "ph6" {
		t.Fatalf("delete: %d", w.Code)
	}
}
