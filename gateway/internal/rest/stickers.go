package rest

import (
	"net/http"
	"regexp"
	"slices"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// maxNewStickers is how many photos one request adds to a pack: as many as
// media claims and publishes at once. A bigger pack grows by edits.
const maxNewStickers = 10

// photoSticker is a sticker of a pack a person made: the ID of its photo
// in 32 hex digits.
var photoSticker = regexp.MustCompile(`^[0-9a-f]{32}$`)

// stickerPackJSON is a pack of the "Магазин стикеров" as the caller sees
// it: bought — taken, free or for votes; added — among their stickers. A
// pack of the site has no author: the site names and draws it. One a
// person made of their photos has its author, name and description, and
// is in the shop when published; its stickers are at /media/stickers/<name>.
// Holders — how many others took it — is told to the author only.
type stickerPackJSON struct {
	ID        string      `json:"id"`
	Price     int64       `json:"price"`
	Stickers  []string    `json:"stickers"`
	Bought    bool        `json:"bought"`
	Added     bool        `json:"added"`
	Author    *authorJSON `json:"author"`
	Name      string      `json:"name,omitempty"`
	About     string      `json:"about,omitempty"`
	Published bool        `json:"published"`
	Holders   int         `json:"holders,omitempty"`
}

type stickerCatalogResponse struct {
	Packs []stickerPackJSON `json:"packs"`
}

// stickerPackRequest is a pack as its author says it: photos are those of
// a new pack, addPhotos and removePhotos what an edit adds and takes out.
type stickerPackRequest struct {
	Name         string   `json:"name"`
	About        string   `json:"about"`
	Price        int64    `json:"price"`
	Published    bool     `json:"published"`
	Photos       []string `json:"photos"`
	AddPhotos    []string `json:"addPhotos"`
	RemovePhotos []string `json:"removePhotos"`
}

func (q stickerPackRequest) draft(added []string) model.StickerPackDraft {
	return model.StickerPackDraft{
		Name: q.Name, About: q.About, Price: q.Price, Published: q.Published, Added: added, Removed: q.RemovePhotos,
	}
}

// stickerPacksJSON shows the packs with the people who made them; a pack
// whose author's page is deleted shows nobody.
func (a *api) stickerPacksJSON(r *http.Request, packs []model.StickerPack) ([]stickerPackJSON, error) {
	var authorIDs []string
	for _, p := range packs {
		if p.AuthorID != "" {
			authorIDs = append(authorIDs, p.AuthorID)
		}
	}

	people, err := a.people(r, authorIDs)
	if err != nil {
		return nil, err
	}

	out := make([]stickerPackJSON, 0, len(packs))
	for _, p := range packs {
		j := stickerPackJSON{
			ID: p.ID, Price: p.Price, Stickers: p.Stickers, Bought: p.Bought, Added: p.Added,
			Name: p.Name, About: p.About, Published: p.Published, Holders: p.Holders,
		}

		if author, ok := people[p.AuthorID]; ok && !author.Deactivated {
			aj := toAuthorJSON(author)
			j.Author = &aj
		}

		out = append(out, j)
	}

	return out, nil
}

// stickerCatalog is the "Магазин стикеров", public: a caller who signed
// in also learns which packs are theirs.
func (a *api) stickerCatalog(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	packs, err := a.stickers.Catalog(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.stickerPacksJSON(r, packs)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, stickerCatalogResponse{Packs: shown})
}

// myStickerPacks is the "Студия стикеров": the packs the caller made.
func (a *api) myStickerPacks(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	packs, err := a.stickers.Mine(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.stickerPacksJSON(r, packs)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, stickerCatalogResponse{Packs: shown})
}

// createStickerPack is "Сохранить набор" of a new pack: the caller's
// uploads (POST /me/photos) become its stickers. A pack is signed with
// its author's name, so the profile has to be filled in. The photos are
// kept for good before the pack shows them, and go if it is refused.
func (a *api) createStickerPack(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req stickerPackRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before making stickers")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.keepStickerPhotos(r, token, me.ID, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	pack, err := a.stickers.Create(r.Context(), token, req.draft(req.Photos))
	if err != nil {
		a.dropStickerPhotos(r, me.ID, req.Photos)
		a.writeError(w, r, err)
		return
	}

	a.writeStickerPack(w, r, http.StatusCreated, pack)
}

// editStickerPack changes a pack of the caller: what it says and costs,
// photos added after its stickers (as many as a new pack takes) and
// stickers taken out, whose files go.
func (a *api) editStickerPack(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req stickerPackRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.keepStickerPhotos(r, token, me.ID, req.AddPhotos); err != nil {
		a.writeError(w, r, err)
		return
	}

	pack, removed, err := a.stickers.Edit(r.Context(), token, r.PathValue("pack"), req.draft(req.AddPhotos))
	if err != nil {
		a.dropStickerPhotos(r, me.ID, req.AddPhotos)
		a.writeError(w, r, err)
		return
	}

	a.dropStickerPhotos(r, me.ID, removed)
	a.writeStickerPack(w, r, http.StatusOK, pack)
}

// deleteStickerPack takes a pack of the caller away with its photos; one
// that others have taken stays theirs (409).
func (a *api) deleteStickerPack(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	removed, err := a.stickers.Delete(r.Context(), token, r.PathValue("pack"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.dropStickerPhotos(r, me.ID, removed)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) writeStickerPack(w http.ResponseWriter, r *http.Request, code int, pack model.StickerPack) {
	shown, err := a.stickerPacksJSON(r, []model.StickerPack{pack})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, code, shown[0])
}

// keepStickerPhotos takes the caller's uploads for a pack and keeps them
// for good, as a backdrop does: a pack may wait longer than media keeps
// what nothing shows. Refused, they are given back.
func (a *api) keepStickerPhotos(r *http.Request, token, ownerID string, photoIDs []string) error {
	if len(photoIDs) > maxNewStickers {
		return failure.New(failure.InvalidArgument, "too many photos")
	}

	if err := a.claimPhotos(r, token, photoIDs); err != nil {
		return err
	}

	if len(photoIDs) == 0 {
		return nil
	}

	if err := a.media.PublishPhotos(r.Context(), ownerID, photoIDs); err != nil {
		a.releasePhotos(r, token, photoIDs, err)
		return err
	}

	return nil
}

// dropStickerPhotos deletes the files of stickers no pack shows. A
// failure only leaves unused files behind, so it is logged.
func (a *api) dropStickerPhotos(r *http.Request, ownerID string, photoIDs []string) {
	for chunk := range slices.Chunk(photoIDs, maxNewStickers) {
		if err := a.media.DeletePublishedPhotos(r.Context(), ownerID, chunk); err != nil {
			a.logger.WarnContext(r.Context(), "photos of stickers left behind", "error", err)
		}
	}
}

// serveSticker is the picture of a sticker a person made: the smaller
// size of its photo. Its name is the photo's, so it is kept for good.
func (a *api) serveSticker(w http.ResponseWriter, r *http.Request) {
	sticker := r.PathValue("sticker")
	if !photoSticker.MatchString(sticker) {
		a.writeError(w, r, failure.New(failure.NotFound, "sticker not found"))
		return
	}

	author, err := a.stickers.Author(r.Context(), sticker)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if author == "" {
		a.writeError(w, r, failure.New(failure.NotFound, "sticker not found"))
		return
	}

	photoID := sticker[:8] + "-" + sticker[8:12] + "-" + sticker[12:16] + "-" + sticker[16:20] + "-" + sticker[20:]

	image, err := a.media.Photo(r.Context(), author, photoID, true)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	serveImage(w, image)
}

// addStickerPack is "Добавить бесплатно" and "Купить": a pack with a price
// is paid for with votes the first time.
func (a *api) addStickerPack(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stickers.Add(r.Context(), token, r.PathValue("pack")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// removeStickerPack is "Убрать набор"; a bought pack comes back free.
func (a *api) removeStickerPack(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stickers.Remove(r.Context(), token, r.PathValue("pack")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// checkSticker lets the caller send only a sticker of a pack they keep
// among their stickers: the message service knows only the name.
func (a *api) checkSticker(r *http.Request, token, sticker string) error {
	if sticker == "" {
		return nil
	}

	packs, err := a.stickers.Catalog(r.Context(), token)
	if err != nil {
		return err
	}

	for _, p := range packs {
		if !slices.Contains(p.Stickers, sticker) {
			continue
		}

		if !p.Added {
			return failure.New(failure.PermissionDenied, "you do not have this sticker")
		}

		return nil
	}

	return failure.New(failure.InvalidArgument, "invalid sticker")
}
