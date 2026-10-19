package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// postponeRequest is a post to publish at PublishAt (RFC 3339), VK's
// "Таймер": text, photos or both. Signed names the author under a post
// for a club.
type postponeRequest struct {
	Text      string   `json:"text"`
	Photos    []string `json:"photos"`
	NSFW      bool     `json:"nsfw"`
	Signed    bool     `json:"signed"`
	PublishAt string   `json:"publishAt"`
}

// postponedEditRequest changes what a postponed post says and when it
// comes out; its photos stay.
type postponedEditRequest struct {
	Text      string `json:"text"`
	NSFW      bool   `json:"nsfw"`
	PublishAt string `json:"publishAt"`
}

// postponedJSON is a post that waits for its time: who wrote it, what it
// says and when it comes out.
type postponedJSON struct {
	ID     string     `json:"id"`
	Author authorJSON `json:"author"`
	Text   string     `json:"text"`
	// TextCut: text is only its beginning, as the list carries it; the
	// whole is at /postponed/{id}.
	TextCut bool        `json:"textCut,omitempty"`
	Photos  []photoJSON `json:"photos"`
	NSFW    bool        `json:"nsfw,omitempty"`
	// FromClub: it will speak for the club, naming its author if Signed.
	FromClub  bool   `json:"fromClub,omitempty"`
	Signed    bool   `json:"signed,omitempty"`
	PublishAt string `json:"publishAt"`
	CreatedAt string `json:"createdAt"`
}

type postponedResponse struct {
	Postponed []postponedJSON `json:"postponed"`
}

func toPostponedJSON(p model.Postponed, author model.Author) postponedJSON {
	return postponedJSON{
		ID:        p.ID,
		Author:    toAuthorJSON(author),
		Text:      p.Text,
		TextCut:   p.TextCut,
		Photos:    toPhotosJSON(p.AuthorID, p.PhotoIDs),
		NSFW:      p.NSFW,
		FromClub:  p.FromWall,
		Signed:    p.Signed,
		PublishAt: p.PublishAt.UTC().Format(time.RFC3339),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// publishTime reads when a postponed post is to come out; whether it is
// ahead of now, and not too far, the post service checks.
func publishTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, failure.New(failure.InvalidArgument, "invalid publish time")
	}

	return t, nil
}

// postponeMyPost postpones a post for the caller's own wall, which exists
// only once the profile has a nickname.
func (a *api) postponeMyPost(w http.ResponseWriter, r *http.Request) {
	a.postpone(w, r, func(string) (string, error) { return "", nil })
}

// postponeClubPost postpones a post that will speak for a club: only its
// admins may, as only they write in its name.
func (a *api) postponeClubPost(w http.ResponseWriter, r *http.Request) {
	a.postpone(w, r, func(token string) (string, error) {
		c, err := a.club(r, token)
		if err != nil {
			return "", err
		}

		access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, true)
		if err != nil {
			return "", err
		}

		if !access.Post {
			return "", failure.New(failure.PermissionDenied, "you cannot post on this wall")
		}

		return c.Club.ID, nil
	})
}

// postpone keeps a post for the wall wall tells, "" for the caller's own,
// with the photos taken from media for it.
func (a *api) postpone(w http.ResponseWriter, r *http.Request, wall func(token string) (string, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req postponeRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	publishAt, err := publishTime(req.PublishAt)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before posting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	wallID, err := wall(token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.claimPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Postpone(r.Context(), token, model.NewPostponed{
		WallID: wallID, Text: req.Text, PhotoIDs: req.Photos, FromWall: wallID != "", Signed: wallID != "" && req.Signed,
		NSFW: req.NSFW, Mentions: mentions, PublishAt: publishAt,
	})
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toPostponedJSON(p, meAuthor(me)))
}

// listMyPostponed are the posts that wait for the caller's own wall.
func (a *api) listMyPostponed(w http.ResponseWriter, r *http.Request) {
	a.listPostponed(w, r, func(token string) (string, error) {
		me, err := a.users.GetMe(r.Context(), token)
		if err != nil {
			return "", err
		}

		return me.ID, nil
	})
}

// listClubPostponed are the posts that wait for a club's wall, for its
// admins.
func (a *api) listClubPostponed(w http.ResponseWriter, r *http.Request) {
	a.listPostponed(w, r, func(token string) (string, error) { return a.wallOfClub(r, token) })
}

// listPostponed are the posts that wait for the wall wall tells, the
// soonest first. Those whose author's page is deleted are left out.
func (a *api) listPostponed(w http.ResponseWriter, r *http.Request, wall func(token string) (string, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	wallID, err := wall(token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	found, err := a.posts.PostponedPosts(r.Context(), token, wallID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := postponedResponse{}
	if resp.Postponed, err = a.withAuthors(r, found); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// withAuthors shows postponed posts with their authors, leaving out those
// whose author's page is deleted.
func (a *api) withAuthors(r *http.Request, found []model.Postponed) ([]postponedJSON, error) {
	ids := make([]string, len(found))
	for i, p := range found {
		ids[i] = p.AuthorID
	}

	people, err := a.authors(r, ids)
	if err != nil {
		return nil, err
	}

	shown := make([]postponedJSON, 0, len(found))
	for _, p := range found {
		if author, ok := people[p.AuthorID]; ok && !author.Club {
			shown = append(shown, toPostponedJSON(p, author))
		}
	}

	return shown, nil
}

// getPostponed is one postponed post, whole: for its author and those who
// list it.
func (a *api) getPostponed(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.PostponedPost(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePostponed(w, r, http.StatusOK, p)
}

// editPostponed changes what a postponed post says and when it comes out;
// the people its new text names will hear of it.
func (a *api) editPostponed(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req postponedEditRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	publishAt, err := publishTime(req.PublishAt)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.EditPostponed(r.Context(), token, r.PathValue("id"), model.PostponedEdit{
		Text: req.Text, NSFW: req.NSFW, Mentions: mentions, PublishAt: publishAt,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writePostponed(w, r, http.StatusOK, p)
}

// writePostponed writes p with its author; one whose author's page is
// deleted is not found.
func (a *api) writePostponed(w http.ResponseWriter, r *http.Request, code int, p model.Postponed) {
	shown, err := a.withAuthors(r, []model.Postponed{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(shown) == 0 {
		a.writeError(w, r, failure.New(failure.NotFound, "postponed post not found"))
		return
	}

	writeJSON(w, code, shown[0])
}

// publishPostponed is "Опубликовать сейчас": the post it makes, as the
// wall shows it.
func (a *api) publishPostponed(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.PublishPostponed(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	posts, err := a.withPeople(r, []model.Post{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(posts) == 0 {
		// Its author's page was deleted meanwhile: it is not shown.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusCreated, posts[0])
}

// deletePostponed deletes a postponed post with its photos.
func (a *api) deletePostponed(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.posts.DeletePostponed(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
