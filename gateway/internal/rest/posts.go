package rest

import (
	"maps"
	"net/http"
	"slices"
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
	// Deactivated: the page is deleted (in dialogs; elsewhere such people
	// are left out).
	Deactivated bool `json:"deactivated,omitempty"`
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
	Reposts   int         `json:"reposts"`
	// Repost is the shared post of a repost, null for other posts.
	Repost *repostJSON `json:"repost"`
	// EditedAt is null for a post never edited; its author may edit it
	// until EditableUntil.
	EditedAt      *string `json:"editedAt"`
	EditableUntil string  `json:"editableUntil"`
	// Pinned: shown first on its wall.
	Pinned bool `json:"pinned"`
}

type repostRequest struct {
	// What the caller says about the shared post; may be empty.
	Text string `json:"text"`
}

// repostJSON is the post a repost shares, shown inside it as openvk does:
// without its counters and menu. Deleted (the post or its author is gone)
// leaves only the ID. RepostOf is set when the shared post is itself a
// repost with words of its own: the site links to it instead of nesting.
type repostJSON struct {
	ID        string      `json:"id"`
	Deleted   bool        `json:"deleted"`
	Author    *authorJSON `json:"author"`
	Wall      *authorJSON `json:"wall"`
	Text      string      `json:"text"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	RepostOf  string      `json:"repostOf"`
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

func authorOfProfile(p model.PublicProfile) model.Author {
	a := authorOf(p.ID, p.AvatarID, p.Profile)
	a.Deactivated = p.Deactivated

	return a
}

func toAuthorJSON(a model.Author) authorJSON {
	return authorJSON{
		ID:          a.ID,
		Avatar:      toAvatarJSON(a.ID, a.AvatarID),
		Nickname:    a.Nickname,
		Name:        a.Name,
		Surname:     a.Surname,
		Gender:      a.Gender,
		Deactivated: a.Deactivated,
	}
}

// wallJSON is the owner of the wall when not the author: null otherwise.
func wallJSON(p model.Post) *authorJSON {
	if p.Wall.ID == p.Author.ID {
		return nil
	}

	w := toAuthorJSON(p.Wall)

	return &w
}

func toPostResponse(p model.Post) postResponse {
	return postResponse{
		ID:            p.ID,
		Author:        toAuthorJSON(p.Author),
		Wall:          wallJSON(p),
		Text:          p.Text,
		Photos:        toPhotosJSON(p.Author.ID, p.PhotoIDs),
		CreatedAt:     p.CreatedAt.UTC().Format(time.RFC3339),
		Likes:         p.Likes,
		Liked:         p.Liked,
		Comments:      p.Comments,
		Reposts:       p.Reposts,
		EditedAt:      optionalTime(p.EditedAt),
		EditableUntil: timeString(p.EditableUntil),
		Pinned:        p.Pinned,
	}
}

// optionalTime is t as RFC 3339, null when zero.
func optionalTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}

	s := timeString(t)

	return &s
}

// toRepostJSON is p as shared inside a repost; p has its people filled in.
func toRepostJSON(p model.Post) *repostJSON {
	author := toAuthorJSON(p.Author)

	return &repostJSON{
		ID:        p.ID,
		Author:    &author,
		Wall:      wallJSON(p),
		Text:      p.Text,
		Photos:    toPhotosJSON(p.Author.ID, p.PhotoIDs),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		RepostOf:  p.RepostOf,
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

// withPeople fills in the posts that reposts share and the profiles of the
// authors and of the owners of the walls, each looked up in one request.
// A post whose author or wall owner deleted the account is left out: the
// wall is gone, and so is what the author wrote. A shared post that is
// gone is shown as deleted inside its repost.
func (a *api) withPeople(r *http.Request, posts []model.Post) ([]postResponse, error) {
	shared, err := a.sharedPosts(r, posts)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, 2*(len(posts)+len(shared)))
	for _, p := range slices.Concat(posts, slices.Collect(maps.Values(shared))) {
		ids = append(ids, p.Author.ID, p.Wall.ID)
	}

	people, err := a.authors(r, ids)
	if err != nil {
		return nil, err
	}

	shown := make([]postResponse, 0, len(posts))
	for _, p := range posts {
		if !withProfiles(&p, people) {
			continue
		}

		resp := toPostResponse(p)

		if p.RepostOf != "" {
			resp.Repost = &repostJSON{ID: p.RepostOf, Deleted: true}
			if s, ok := shared[p.RepostOf]; ok && withProfiles(&s, people) {
				resp.Repost = toRepostJSON(s)
			}
		}

		shown = append(shown, resp)
	}

	return shown, nil
}

// sharedPosts are the posts that reposts among posts share, by ID; those
// that are gone are missing.
func (a *api) sharedPosts(r *http.Request, posts []model.Post) (map[string]model.Post, error) {
	var ids []string
	for _, p := range posts {
		if p.RepostOf != "" {
			ids = append(ids, p.RepostOf)
		}
	}

	if len(ids) == 0 {
		return nil, nil
	}

	found, err := a.posts.GetMany(r.Context(), unique(ids))
	if err != nil {
		return nil, err
	}

	shared := make(map[string]model.Post, len(found))
	for _, p := range found {
		shared[p.ID] = p
	}

	return shared, nil
}

// withProfiles puts the profiles of p's author and wall owner into p;
// false when either deleted the account.
func withProfiles(p *model.Post, people map[string]model.Author) bool {
	author, hasAuthor := people[p.Author.ID]
	wall, hasWall := people[p.Wall.ID]
	p.Author, p.Wall = author, wall

	return hasAuthor && hasWall
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

	// The wall of a deleted page is gone, as on openvk, until restored.
	if owner.Deactivated {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "this page is deleted"))
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

// repostPost shares a post on the caller's wall, as openvk's "Поделиться",
// and returns the repost with the shared post inside.
func (a *api) repostPost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req repostRequest
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

	p, err := a.posts.Repost(r.Context(), token, r.PathValue("id"), req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.withPeople(r, []model.Post{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// The caller's own profile was just read: only a race with deleting
	// the account leaves nothing to show.
	if len(shown) == 0 {
		a.writeError(w, r, failure.New(failure.NotFound, "user not found"))
		return
	}

	writeJSON(w, http.StatusCreated, shown[0])
}

// deletePost deletes a post, by its author or the owner of its wall; the
// post service has media delete its photos.
func (a *api) deletePost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.posts.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// editPost changes the text and photos of the caller's post, as openvk's
// "Редактировать"; new photos must be the caller's, as when posting.
func (a *api) editPost(w http.ResponseWriter, r *http.Request) {
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

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Edit(r.Context(), token, r.PathValue("id"), req.Text, req.Photos)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.withPeople(r, []model.Post{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// Only a race with deleting the account leaves nothing to show.
	if len(shown) == 0 {
		a.writeError(w, r, failure.New(failure.NotFound, "post not found"))
		return
	}

	writeJSON(w, http.StatusOK, shown[0])
}

// pinPost and unpinPost are idempotent: the owner of the wall shows the
// post first there, instead of the one pinned before, or stops.
func (a *api) pinPost(w http.ResponseWriter, r *http.Request) {
	a.setPinned(w, r, true)
}

func (a *api) unpinPost(w http.ResponseWriter, r *http.Request) {
	a.setPinned(w, r, false)
}

func (a *api) setPinned(w http.ResponseWriter, r *http.Request, pinned bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	set := a.posts.Unpin
	if pinned {
		set = a.posts.Pin
	}

	if err = set(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

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
