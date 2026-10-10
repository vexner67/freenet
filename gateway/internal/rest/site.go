package rest

import "net/http"

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
