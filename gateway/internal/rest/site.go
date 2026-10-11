package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// aboutResponse is what the page "о сайте" counts, as openvk's "Об этой
// инстанции": the people, those on the site now, the groups and the posts.
type aboutResponse struct {
	Users  int `json:"users"`
	Online int `json:"online"`
	Clubs  int `json:"clubs"`
	Posts  int `json:"posts"`
}

// about counts the site for its page "о сайте"; anyone may ask.
func (a *api) about(w http.ResponseWriter, r *http.Request) {
	var (
		resp aboutResponse
		err  error
	)

	if resp.Users, resp.Online, err = a.users.Statistics(r.Context()); err != nil {
		a.writeError(w, r, err)
		return
	}

	if resp.Clubs, err = a.clubs.Count(r.Context()); err != nil {
		a.writeError(w, r, err)
		return
	}

	if resp.Posts, err = a.posts.Count(r.Context()); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// maxKnownNicknames is how many nicknames one request asks about.
const maxKnownNicknames = 100

type nicknamesResponse struct {
	Nicknames []string `json:"nicknames"`
}

// knownNicknames tells which of the nicknames asked ("?nickname=", up to
// maxKnownNicknames) are someone's: the site links "@word" of a text to a
// page only when there is one. Pages are public, and so is this.
func (a *api) knownNicknames(w http.ResponseWriter, r *http.Request) {
	asked := r.URL.Query()["nickname"]
	if len(asked) > maxKnownNicknames {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "too many nicknames"))
		return
	}

	resp := nicknamesResponse{Nicknames: []string{}}

	if len(asked) > 0 {
		known, err := a.users.KnownNicknames(r.Context(), asked)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		resp.Nicknames = append(resp.Nicknames, known...)
	}

	writeJSON(w, http.StatusOK, resp)
}
