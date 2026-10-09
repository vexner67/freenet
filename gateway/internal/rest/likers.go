package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// listPostLikers is openvk's "Понравилось N людям" of a post: who like
// it, newest first, public as the post.
func (a *api) listPostLikers(w http.ResponseWriter, r *http.Request) {
	a.listLikers(w, r, a.posts.PostLikers)
}

func (a *api) listCommentLikers(w http.ResponseWriter, r *http.Request) {
	a.listLikers(w, r, a.posts.CommentLikers)
}

// listLikers writes a page of likers as people: those of deleted pages
// are left out, as everything they did, but counted.
func (a *api) listLikers(
	w http.ResponseWriter,
	r *http.Request,
	list func(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error),
) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := list(r.Context(), r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := userPageResponse{
		Users:      make([]publicProfileResponse, 0, len(page.UserIDs)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	if len(page.UserIDs) > 0 {
		profiles, err := a.users.GetProfiles(r.Context(), page.UserIDs)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		byID := make(map[string]model.PublicProfile, len(profiles))
		for _, p := range profiles {
			byID[p.ID] = p
		}

		for _, id := range page.UserIDs {
			if p, ok := byID[id]; ok && !p.Deactivated {
				resp.Users = append(resp.Users, toPublicProfileResponse(p))
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
