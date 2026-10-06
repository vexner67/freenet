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
	ID     string     `json:"id"`
	Author authorJSON `json:"author"`
	// Wall is the owner of the wall when someone else's: null on the
	// author's own wall.
	Wall      *authorJSON `json:"wall"`
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
	// CanPost: the viewer may write on this wall; told on a wall's first
	// page, false for anonymous viewers.
	CanPost bool `json:"canPost"`
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
	var wall *authorJSON
	if p.Wall.ID != p.Author.ID {
		w := toAuthorJSON(p.Wall)
		wall = &w
	}

	return postResponse{
		ID:        p.ID,
		Author:    toAuthorJSON(p.Author),
		Wall:      wall,
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

	shown, err := a.withPeople(r, []model.Post{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(shown) == 0 {
		a.writeError(w, r, failure.New(failure.NotFound, "post not found"))
		return
	}

	writeJSON(w, http.StatusOK, shown[0])
}

// withPeople fills in the profiles of the authors and of the owners of the
// walls, looked up in one request. A post whose author or wall owner
// deleted the account is left out: the wall is gone, and so is what the
// author wrote.
func (a *api) withPeople(r *http.Request, posts []model.Post) ([]postResponse, error) {
	ids := make([]string, 0, 2*len(posts))
	for _, p := range posts {
		ids = append(ids, p.Author.ID, p.Wall.ID)
	}

	people, err := a.authors(r, ids)
	if err != nil {
		return nil, err
	}

	shown := make([]postResponse, 0, len(posts))
	for _, p := range posts {
		author, hasAuthor := people[p.Author.ID]
		wall, hasWall := people[p.Wall.ID]
		if !hasAuthor || !hasWall {
			continue
		}

		p.Author, p.Wall = author, wall
		shown = append(shown, toPostResponse(p))
	}

	return shown, nil
}

// listPosts is the wall of a user, whoever wrote the posts. Its first page
// tells a signed-in viewer whether they may write there.
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

	cursor := r.URL.Query().Get("cursor")

	page, err := a.posts.List(r.Context(), token, owner.ID, cursor, limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := postPageResponse{NextCursor: page.NextCursor, Total: page.Total}

	if resp.Posts, err = a.withPeople(r, page.Posts); err != nil {
		a.writeError(w, r, err)
		return
	}

	if token != "" && cursor == "" {
		if resp.CanPost, err = a.users.CanPostOnWall(r.Context(), token, owner.ID); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// createPost writes on the caller's own wall, which exists only once the
// profile has a nickname.
func (a *api) createPost(w http.ResponseWriter, r *http.Request) {
	a.writePost(w, r, "")
}

// createWallPost writes on the wall of the user with the nickname: someone
// else's, as its owner's privacy allows, or the caller's own.
func (a *api) createWallPost(w http.ResponseWriter, r *http.Request) {
	a.writePost(w, r, r.PathValue("nickname"))
}

// writePost writes on the wall of nickname, "" for the caller's own. The
// author needs a filled-in profile: without a nickname there is no name
// to show next to the post.
func (a *api) writePost(w http.ResponseWriter, r *http.Request, nickname string) {
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

	author := authorOf(me.ID, me.AvatarID, me.Profile)

	wall, err := a.wallFor(r, token, me.ID, nickname)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Create(r.Context(), token, wall.ID, req.Text, req.Photos)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if wall.ID == "" {
		wall = author
	}

	p.Author, p.Wall = author, wall

	writeJSON(w, http.StatusCreated, toPostResponse(p))
}

// wallFor is the owner of the wall of nickname that the caller (myID) may
// write on; an empty Author for the caller's own wall.
func (a *api) wallFor(r *http.Request, token, myID, nickname string) (model.Author, error) {
	if nickname == "" {
		return model.Author{}, nil
	}

	owner, err := a.users.GetProfile(r.Context(), nickname)
	if err != nil {
		return model.Author{}, err
	}

	if owner.ID == myID {
		return model.Author{}, nil
	}

	allowed, err := a.users.CanPostOnWall(r.Context(), token, owner.ID)
	if err != nil {
		return model.Author{}, err
	}

	if !allowed {
		return model.Author{}, failure.New(failure.PermissionDenied, "you cannot post on this wall")
	}

	return authorOf(owner.ID, owner.AvatarID, owner.Profile), nil
}

// deletePost deletes a post, by its author or the owner of its wall, and
// then its photos. The media service deletes only the caller's own
// photos, so when the owner of the wall deletes someone's post, the
// author's photos stay (see the debt in AGENTS.md).
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
