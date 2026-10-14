package rest

import (
	"net/http"
	"slices"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// stickerPackJSON is a pack of the "Магазин стикеров" as the caller sees
// it: bought — taken, free or for votes; added — among their stickers.
type stickerPackJSON struct {
	ID       string   `json:"id"`
	Price    int64    `json:"price"`
	Stickers []string `json:"stickers"`
	Bought   bool     `json:"bought"`
	Added    bool     `json:"added"`
}

type stickerCatalogResponse struct {
	Packs []stickerPackJSON `json:"packs"`
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

	resp := stickerCatalogResponse{Packs: make([]stickerPackJSON, 0, len(packs))}
	for _, p := range packs {
		resp.Packs = append(resp.Packs, stickerPackJSON(p))
	}

	writeJSON(w, http.StatusOK, resp)
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
