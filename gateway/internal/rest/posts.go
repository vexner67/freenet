package rest

import (
	"context"
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
	// NSFW marks the post as adult; an edit without it keeps the mark.
	NSFW *bool `json:"nsfw"`
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
	// Club: a community, which has only a name (in name) and maybe an
	// address, its page being /club/<address or id>.
	Club    bool   `json:"club,omitempty"`
	Address string `json:"address,omitempty"`
	// Online: on the site now ("Онлайн"), if their page tells everyone.
	Online bool `json:"online,omitempty"`
}

type postResponse struct {
	ID     string     `json:"id"`
	Author authorJSON `json:"author"`
	// Wall is the owner of the wall when someone else's: null on the
	// author's own wall.
	Wall *authorJSON `json:"wall"`
	Text string      `json:"text"`
	// TextCut: text is only the beginning of what the post says, as lists
	// carry it; GET /posts/{id} tells it whole.
	TextCut   bool        `json:"textCut,omitempty"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	Likes     int         `json:"likes"`
	Liked     bool        `json:"liked"`
	Comments  int         `json:"comments"`
	Reposts   int         `json:"reposts"`
	// Repost is the shared post of a repost, null for other posts.
	Repost *repostJSON `json:"repost"`
	// SharedPhoto is a photo of an album the post shares ("Поделиться"
	// under a photo), null for other posts.
	SharedPhoto *sharedPhotoJSON `json:"sharedPhoto"`
	// EditedAt is null for a post never edited; its author may edit it
	// until EditableUntil.
	EditedAt      *string `json:"editedAt"`
	EditableUntil string  `json:"editableUntil"`
	// Pinned: shown first on its wall.
	Pinned bool `json:"pinned"`
	// Archived: put away by the owner of the wall, who alone sees it.
	// CommentsClosed: it takes no new comments.
	Archived       bool `json:"archived"`
	CommentsClosed bool `json:"commentsClosed"`
	// NSFW: its author marked it as adult.
	NSFW bool `json:"nsfw"`
	// FromWall: the post speaks for its wall, a community, shown as its
	// author; who wrote it is not told.
	FromWall bool `json:"fromWall"`
	// AvatarUpdate: the post shows the new photo of its wall's owner
	// ("обновил фотографию на своей странице"); never edited.
	AvatarUpdate bool `json:"avatarUpdate"`
	// Mine: the viewer wrote it, and may edit it; false for anonymous
	// viewers.
	Mine bool `json:"mine"`
	// Signer is who wrote a post for a community that names its author
	// ("Автор: ..."); null otherwise.
	Signer *authorJSON `json:"signer"`
	// LastComments are its newest comments, oldest first: only in a list
	// asked for with ?comments=last.
	LastComments []commentResponse `json:"lastComments,omitempty"`
}

type repostRequest struct {
	// What the caller says about the shared post; may be empty.
	Text string `json:"text"`
	// Club is a community to share it in ("в группу"), "" for the caller's
	// wall; FromClub speaks for it, Signed names the caller under it.
	Club     string `json:"club"`
	FromClub bool   `json:"fromClub"`
	Signed   bool   `json:"signed"`
}

// sharedPhotoJSON is a photo of an album a post shares; deleted leaves
// only its ID.
type sharedPhotoJSON struct {
	ID      string     `json:"id"`
	Deleted bool       `json:"deleted"`
	Photo   *photoJSON `json:"photo"`
	AlbumID string     `json:"albumId"`
	// NSFW: the photo came from a post marked as adult.
	NSFW bool `json:"nsfw"`
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
	TextCut   bool        `json:"textCut,omitempty"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	RepostOf  string      `json:"repostOf"`
	// AvatarUpdate: the shared post shows a new photo, as Post's.
	AvatarUpdate bool `json:"avatarUpdate"`
	// NSFW: the author of the shared post marked it as adult.
	NSFW bool `json:"nsfw"`
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
	// CanSuggest: the viewer may suggest a post for this community's wall
	// instead ("Предложка"); told as CanPost.
	CanSuggest bool `json:"canSuggest"`
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
	a.Online = p.Online

	return a
}

// clubAuthor is a community as the owner of a wall or the voice of a post.
func clubAuthor(c model.Club) model.Author {
	return model.Author{ID: c.ID, AvatarID: c.AvatarID, Name: c.Name, Club: true, Address: c.Address}
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
		Club:        a.Club,
		Address:     a.Address,
		Online:      a.Online,
	}
}

// shownAuthor is who a post is shown by: its author, or the community it
// speaks for.
func shownAuthor(p model.Post) authorJSON {
	if p.FromWall {
		return toAuthorJSON(p.Wall)
	}

	return toAuthorJSON(p.Author)
}

// wallJSON is the owner of the wall when not the author shown: null
// otherwise.
func wallJSON(p model.Post) *authorJSON {
	if p.Wall.ID == p.Author.ID || p.FromWall {
		return nil
	}

	w := toAuthorJSON(p.Wall)

	return &w
}

func toPostResponse(p model.Post) postResponse {
	return postResponse{
		ID:             p.ID,
		Author:         shownAuthor(p),
		Wall:           wallJSON(p),
		Text:           p.Text,
		TextCut:        p.TextCut,
		Photos:         toPhotosJSON(p.Author.ID, p.PhotoIDs),
		CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339),
		Likes:          p.Likes,
		Liked:          p.Liked,
		Comments:       p.Comments,
		Reposts:        p.Reposts,
		EditedAt:       optionalTime(p.EditedAt),
		EditableUntil:  timeString(p.EditableUntil),
		Pinned:         p.Pinned,
		Archived:       p.Archived,
		CommentsClosed: p.CommentsClosed,
		NSFW:           p.NSFW,
		FromWall:       p.FromWall,
		AvatarUpdate:   p.AvatarUpdate,
		Mine:           p.Mine,
		Signer:         signerJSON(p),
	}
}

// signerJSON is the author named under a signed post for a community; null
// when not signed, or when the author's page is gone (no name left).
func signerJSON(p model.Post) *authorJSON {
	if !p.FromWall || !p.Signed || p.Author.Nickname == "" {
		return nil
	}

	a := toAuthorJSON(p.Author)

	return &a
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
	author := shownAuthor(p)

	return &repostJSON{
		ID:        p.ID,
		Author:    &author,
		Wall:      wallJSON(p),
		Text:      p.Text,
		TextCut:   p.TextCut,
		Photos:    toPhotosJSON(p.Author.ID, p.PhotoIDs),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		RepostOf:  p.RepostOf,

		AvatarUpdate: p.AvatarUpdate,
		NSFW:         p.NSFW,
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
		resp.SharedPhoto = a.sharedPhoto(r, p.SharedPhotoID)

		if p.RepostOf != "" {
			resp.Repost = &repostJSON{ID: p.RepostOf, Deleted: true}
			if s, ok := shared[p.RepostOf]; ok && withProfiles(&s, people) {
				resp.Repost = toRepostJSON(s)
			}
		}

		shown = append(shown, resp)
	}

	if r.URL.Query().Get("comments") == lastComments {
		if err = a.withLastComments(r, shown, posts); err != nil {
			return nil, err
		}
	}

	return shown, nil
}

// maxPostsPerRequest is how many posts post tells the comments of at once.
const maxPostsPerRequest = 50

// lastComments is ?comments=last of the lists of posts: each post comes
// with its three newest comments, as openvk's "Микроблог" shows a wall.
const lastComments = "last"

// withLastComments gives each of shown its newest comments, read for all
// of them in one request and their authors in another. A comment whose
// author deleted the page is left out, as on the page of the post.
func (a *api) withLastComments(r *http.Request, shown []postResponse, posts []model.Post) error {
	if len(shown) == 0 {
		return nil
	}

	token, err := optionalBearerToken(r)
	if err != nil {
		return err
	}

	// Comments for a community show it: the owner of the post's wall.
	walls := make(map[string]string, len(posts))
	for _, p := range posts {
		walls[p.ID] = p.Wall.ID
	}

	postIDs := make([]string, len(shown))
	for i := range shown {
		postIDs[i] = shown[i].ID
	}

	// A page with its pinned post is one more than post takes at once.
	var comments []model.Comment
	for chunk := range slices.Chunk(postIDs, maxPostsPerRequest) {
		last, err := a.posts.LastComments(r.Context(), token, chunk)
		if err != nil {
			return err
		}
		comments = append(comments, last...)
	}

	ids := make([]string, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.Author.ID)
		if c.FromWall {
			ids = append(ids, walls[c.PostID])
		}
	}

	authors, err := a.authors(r, ids)
	if err != nil {
		return err
	}

	byPost := make(map[string][]commentResponse, len(shown))
	for _, c := range comments {
		by := c.Author.ID
		if c.FromWall {
			by = walls[c.PostID]
		}

		_, written := authors[c.Author.ID]
		if author, ok := authors[by]; ok && written {
			byPost[c.PostID] = append(byPost[c.PostID], toCommentResponse(c, author))
		}
	}

	for i := range shown {
		if last, ok := byPost[shown[i].ID]; ok {
			shown[i].LastComments = last
		}
	}

	return nil
}

// sharedPhoto is the photo of an album a post shares, nil for none; one
// gone, or failing to be read, is deleted.
func (a *api) sharedPhoto(r *http.Request, id string) *sharedPhotoJSON {
	if id == "" {
		return nil
	}

	photo, _, err := a.albums.Photo(r.Context(), "", id)
	if err != nil {
		return &sharedPhotoJSON{ID: id, Deleted: true}
	}

	shown := toPhotoJSON(photo.AuthorID, photo.ID)

	return &sharedPhotoJSON{ID: id, Photo: &shown, AlbumID: photo.AlbumID, NSFW: photo.NSFW}
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
// false when either deleted the account. A post that speaks for its wall
// needs only the wall: its author is not shown, unless the post is signed,
// and keeps at least the ID that the photos are filed under.
func withProfiles(p *model.Post, people map[string]model.Author) bool {
	wall, hasWall := people[p.Wall.ID]
	p.Wall = wall

	if p.FromWall {
		if author, ok := people[p.Author.ID]; ok && p.Signed {
			p.Author = author
		}

		return hasWall
	}

	author, hasAuthor := people[p.Author.ID]
	p.Author = author

	return hasAuthor && hasWall
}

// wallPick is which posts of a wall the request asks for: ?type= is the
// tab of openvk's wall, ?q= its "Поиск".
func wallPick(r *http.Request) model.WallPick {
	return model.WallPick{Filter: r.URL.Query().Get("type"), Query: r.URL.Query().Get("q")}
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

	// The wall of a deleted page is gone, as on openvk, until restored; that
	// of a page hidden from the caller too.
	owner, err := a.visiblePage(r, token, pagePart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	cursor := r.URL.Query().Get("cursor")

	page, err := a.posts.List(r.Context(), token, owner.ID, wallPick(r), cursor, limit)
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

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Create(r.Context(), token, model.NewPost{
		WallID: wall.ID, Text: req.Text, PhotoIDs: req.Photos, NSFW: req.NSFW != nil && *req.NSFW, Mentions: mentions,
	})
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

// mentioned are the people text names ("@nickname", "[nickname|Имя]")
// who are to hear of it, by ID: the post service reads no nicknames, and
// who may be named the user service decides. A text that names nobody
// asks nothing.
func (a *api) mentioned(r *http.Request, token, text string) ([]string, error) {
	nicknames := model.MentionedNicknames(text)
	if len(nicknames) == 0 {
		return nil, nil
	}

	return a.users.ResolveMentions(r.Context(), token, nicknames)
}

// wallFor is the owner of the wall of nickname that the caller (myID) may
// write on; an empty Author for the caller's own wall.
func (a *api) wallFor(r *http.Request, token, myID, nickname string) (model.Author, error) {
	if nickname == "" {
		return model.Author{}, nil
	}

	owner, err := a.users.GetProfile(r.Context(), "", nickname)
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
	a.share(w, r, func(string) (string, string, error) { return r.PathValue("id"), "", nil })
}

// share writes the repost of a post or a photo, which what tells (a
// post's ID, or a photo's), as openvk's "Поделиться": on the caller's
// wall or a community's.
func (a *api) share(w http.ResponseWriter, r *http.Request, what func(token string) (postID, photoID string, err error)) {
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

	postID, photoID, err := what(token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	nr, err := a.repostTarget(r, token, req)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	nr.PhotoID = photoID

	p, err := a.posts.Repost(r.Context(), token, postID, nr)
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

// repostTarget is where the caller shares a post: their wall, or a
// community's they may write on, for it as on its wall (createClubPost).
func (a *api) repostTarget(r *http.Request, token string, req repostRequest) (model.NewRepost, error) {
	nr := model.NewRepost{Text: req.Text}
	if req.Club == "" {
		return nr, nil
	}

	c, err := a.clubs.Get(r.Context(), token, req.Club)
	if err != nil {
		return model.NewRepost{}, err
	}

	fromClub := req.FromClub || (c.Club.Wall != "open" && manages(c.Role))

	access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, fromClub)
	if err != nil {
		return model.NewRepost{}, err
	}

	if !access.Post {
		return model.NewRepost{}, failure.New(failure.PermissionDenied, "you cannot post on this wall")
	}

	nr.WallID, nr.FromWall, nr.Signed = c.Club.ID, fromClub, fromClub && req.Signed

	return nr, nil
}

// repostsByID are the posts of ids as shown inside a repost or a message:
// those that are gone, or whose author is, deleted.
func (a *api) repostsByID(r *http.Request, ids []string) (map[string]*repostJSON, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	found, err := a.posts.GetMany(r.Context(), unique(ids))
	if err != nil {
		return nil, err
	}

	people := make([]string, 0, 2*len(found))
	for _, p := range found {
		people = append(people, p.Author.ID, p.Wall.ID)
	}

	authors, err := a.authors(r, people)
	if err != nil {
		return nil, err
	}

	shown := make(map[string]*repostJSON, len(ids))
	for _, id := range ids {
		shown[id] = &repostJSON{ID: id, Deleted: true}
	}
	for _, p := range found {
		if withProfiles(&p, authors) {
			shown[p.ID] = toRepostJSON(p)
		}
	}

	return shown, nil
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

	p, err := a.posts.Edit(r.Context(), token, r.PathValue("id"), model.PostEdit{Text: req.Text, PhotoIDs: req.Photos, NSFW: req.NSFW})
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

// archivePost and unarchivePost are idempotent: the owner of the wall
// puts the post away in "Архив записей", where only they see it, or back.
func (a *api) archivePost(w http.ResponseWriter, r *http.Request) {
	a.switchPost(w, r, a.posts.Archive, true)
}

func (a *api) unarchivePost(w http.ResponseWriter, r *http.Request) {
	a.switchPost(w, r, a.posts.Archive, false)
}

// closeComments and openComments are idempotent: the owner of the wall
// lets nobody comment the post anymore, or again.
func (a *api) closeComments(w http.ResponseWriter, r *http.Request) {
	a.switchPost(w, r, a.posts.CloseComments, true)
}

func (a *api) openComments(w http.ResponseWriter, r *http.Request) {
	a.switchPost(w, r, a.posts.CloseComments, false)
}

func (a *api) switchPost(w http.ResponseWriter, r *http.Request, set func(ctx context.Context, token, id string, on bool) error, on bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = set(r.Context(), token, r.PathValue("id"), on); err != nil {
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
