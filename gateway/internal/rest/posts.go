package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const maxPageSize = 50

type postRequest struct {
	Text string `json:"text"`
	// Photos are IDs from POST /api/v1/me/photos.
	Photos []string `json:"photos"`
}

type authorJSON struct {
	ID       string      `json:"id"`
	Avatar   *avatarJSON `json:"avatar"`
	Nickname string      `json:"nickname"`
	Name     string      `json:"name"`
	Surname  string      `json:"surname"`
	Gender   string      `json:"gender"`
}

type postResponse struct {
	ID        string      `json:"id"`
	Author    authorJSON  `json:"author"`
	Text      string      `json:"text"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	Likes     int         `json:"likes"`
	Liked     bool        `json:"liked"`
	Comments  int         `json:"comments"`
}

type likeResponse struct {
	Likes int  `json:"likes"`
	Liked bool `json:"liked"`
}

type postPageResponse struct {
	Posts      []postResponse `json:"posts"`
	NextCursor string         `json:"nextCursor"`
	Total      int            `json:"total"`
}

func authorOf(id, avatarID string, p model.Profile) model.Author {
	return model.Author{
		ID:       id,
		AvatarID: avatarID,
		Nickname: p.Nickname,
		Name:     p.Name,
		Surname:  p.Surname,
		Gender:   p.Gender,
	}
}

func toAuthorJSON(a model.Author) authorJSON {
	return authorJSON{
		ID:       a.ID,
		Avatar:   toAvatarJSON(a.ID, a.AvatarID),
		Nickname: a.Nickname,
		Name:     a.Name,
		Surname:  a.Surname,
		Gender:   a.Gender,
	}
}

func toPostResponse(p model.Post) postResponse {
	return postResponse{
		ID:        p.ID,
		Author:    toAuthorJSON(p.Author),
		Text:      p.Text,
		Photos:    toPhotosJSON(p.Author.ID, p.PhotoIDs),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		Likes:     p.Likes,
		Liked:     p.Liked,
		Comments:  p.Comments,
	}
}

// getPost is one post with its author, for the page of the post.
func (a *api) getPost(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	authors, err := a.authors(r, []string{p.Author.ID})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// The wall of an author who deleted the account is gone with it.
	author, ok := authors[p.Author.ID]
	if !ok {
		a.writeError(w, r, failure.New(failure.NotFound, "post not found"))
		return
	}

	p.Author = author

	writeJSON(w, http.StatusOK, toPostResponse(p))
}

// listPosts is the wall of a user. All posts are by the wall's owner, so
// the author is the profile looked up by nickname.
func (a *api) listPosts(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.posts.List(r.Context(), token, owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	author := authorOf(owner.ID, owner.AvatarID, owner.Profile)
	resp := postPageResponse{
		Posts:      make([]postResponse, 0, len(page.Posts)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	for _, p := range page.Posts {
		p.Author = author
		resp.Posts = append(resp.Posts, toPostResponse(p))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createPost writes on the caller's own wall, which exists only once the
// profile has a nickname.
func (a *api) createPost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req postRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !me.ProfileCompleted {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "fill in your profile before posting"))
		return
	}

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Create(r.Context(), token, req.Text, req.Photos)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p.Author = authorOf(me.ID, me.AvatarID, me.Profile)

	writeJSON(w, http.StatusCreated, toPostResponse(p))
}

// deletePost deletes the caller's post and then its photos: only the
// author may delete a post, and the photos are the author's.
func (a *api) deletePost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	id := r.PathValue("id")

	p, err := a.posts.Get(r.Context(), token, id)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.posts.Delete(r.Context(), token, id); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.forgetPhotos(r, token, p.PhotoIDs)

	w.WriteHeader(http.StatusNoContent)
}

// likePost and unlikePost are idempotent, so a double click does no harm.
func (a *api) likePost(w http.ResponseWriter, r *http.Request) {
	a.setLike(w, r, true)
}

func (a *api) unlikePost(w http.ResponseWriter, r *http.Request) {
	a.setLike(w, r, false)
}

func (a *api) setLike(w http.ResponseWriter, r *http.Request, liked bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	set := a.posts.Unlike
	if liked {
		set = a.posts.Like
	}

	likes, err := set(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, likeResponse{Likes: likes, Liked: liked})
}
