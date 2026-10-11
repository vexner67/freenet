package rest

import (
	"context"
	"net/http"
	"slices"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// maxUnignored is how many sources one request brings back, as openvk's
// newsfeed.deleteBan.
const maxUnignored = 10

// ignoredResponse is openvk's "Игнорируемые источники": the people and
// the communities whose walls the caller's news leave out, newest first.
type ignoredResponse struct {
	Sources []authorJSON `json:"sources"`
}

// ignores tells whether the caller's news leave the wall out; nobody
// without a token ignores anything.
func (a *api) ignores(r *http.Request, token, sourceID string) (bool, error) {
	if token == "" {
		return false, nil
	}

	ignored, err := a.users.IgnoredIDs(r.Context(), token)
	if err != nil {
		return false, err
	}

	return slices.Contains(ignored, sourceID), nil
}

// without is ids but those of them that are in left; ids itself is not
// changed.
func without(ids, left []string) []string {
	if len(left) == 0 {
		return ids
	}

	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(left, id) {
			kept = append(kept, id)
		}
	}

	return kept
}

// listIgnored names the sources the caller ignores. A source that is
// gone (an erased page) stops being ignored here: nobody could take it
// off the list, and it would hold one of the 50 places.
func (a *api) listIgnored(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids, err := a.users.IgnoredIDs(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// Deleted pages are named too, unlike among the authors of posts: one
	// may stop ignoring them.
	found, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var rest []string
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			rest = append(rest, id)
		}
	}

	// At most 50 sources: fewer than one request of the club service takes.
	var clubs []model.Club
	if len(rest) > 0 {
		if clubs, err = a.clubs.GetMany(r.Context(), rest); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	resp := ignoredResponse{Sources: make([]authorJSON, 0, len(ids))}

	var gone []string
	for _, id := range ids {
		if p, ok := found[id]; ok {
			resp.Sources = append(resp.Sources, toAuthorJSON(p))
		} else if i := slices.IndexFunc(clubs, func(c model.Club) bool { return c.ID == id }); i >= 0 {
			resp.Sources = append(resp.Sources, toAuthorJSON(clubAuthor(clubs[i])))
		} else {
			gone = append(gone, id)
		}
	}

	for chunk := range slices.Chunk(gone, maxUnignored) {
		if err = a.users.Unignore(r.Context(), token, chunk); err != nil {
			a.logger.WarnContext(r.Context(), "forget ignored sources that are gone", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ignoreUser leaves the wall of the user with the nickname out of the
// caller's news, as openvk's "Игнорировать пользователя". Idempotent, as
// the other three are.
func (a *api) ignoreUser(w http.ResponseWriter, r *http.Request) {
	a.changeIgnored(w, r, a.userSource, a.users.Ignore)
}

func (a *api) unignoreUser(w http.ResponseWriter, r *http.Request) {
	a.changeIgnored(w, r, a.userSource, a.unignoreOne)
}

// ignoreClub is openvk's "Игнорировать группу".
func (a *api) ignoreClub(w http.ResponseWriter, r *http.Request) {
	a.changeIgnored(w, r, a.clubSource, a.users.Ignore)
}

func (a *api) unignoreClub(w http.ResponseWriter, r *http.Request) {
	a.changeIgnored(w, r, a.clubSource, a.unignoreOne)
}

func (a *api) unignoreOne(ctx context.Context, token, sourceID string) error {
	return a.users.Unignore(ctx, token, []string{sourceID})
}

// userSource is the ID of the user the path names, clubSource of the
// community: the user service takes a source as it is.
func (a *api) userSource(r *http.Request, _ string) (string, error) {
	target, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		return "", err
	}

	return target.ID, nil
}

func (a *api) clubSource(r *http.Request, token string) (string, error) {
	v, err := a.club(r, token)
	if err != nil {
		return "", err
	}

	return v.Club.ID, nil
}

func (a *api) changeIgnored(
	w http.ResponseWriter,
	r *http.Request,
	source func(r *http.Request, token string) (string, error),
	change func(ctx context.Context, token, sourceID string) error,
) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	id, err := source(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = change(r.Context(), token, id); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unignoreMany is "Не игнорировать" of the list: up to 10 sources at
// once, by their IDs.
func (a *api) unignoreMany(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := unique(r.URL.Query()["id"])
	if len(ids) == 0 || len(ids) > maxUnignored {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "id must name between 1 and 10 sources"))
		return
	}

	if err = a.users.Unignore(r.Context(), token, ids); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
