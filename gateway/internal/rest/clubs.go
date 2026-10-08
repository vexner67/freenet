package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type createClubRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// clubSettingsRequest is the whole settings form: an empty address or
// website takes it away.
type clubSettingsRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Address       string `json:"address"`
	Website       string `json:"website"`
	Wall          string `json:"wall"`
	AdminsDisplay string `json:"adminsDisplay"`
}

type clubPostRequest struct {
	Text   string   `json:"text"`
	Photos []string `json:"photos"`
	// FromClub: the post speaks for the club ("От имени сообщества"),
	// naming its author with Signed ("Подпись автора").
	FromClub bool `json:"fromClub"`
	Signed   bool `json:"signed"`
}

type clubJSON struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Address       string      `json:"address"`
	Website       string      `json:"website"`
	Wall          string      `json:"wall"`
	AdminsDisplay string      `json:"adminsDisplay"`
	Members       int         `json:"members"`
	CreatedAt     string      `json:"createdAt"`
	Avatar        *avatarJSON `json:"avatar"`
}

// clubResponse is a club as the viewer sees it: role is "" (not a member,
// or anonymous), "member", "admin" or "owner"; owner is null when the
// owner's page is deleted or erased.
type clubResponse struct {
	clubJSON
	Role  string      `json:"role"`
	Owner *authorJSON `json:"owner"`
}

type clubMemberJSON struct {
	User authorJSON `json:"user"`
	Role string     `json:"role"`
}

// clubMembersResponse: Total counts the members, or the admins, on all
// pages, those whose page is deleted too.
type clubMembersResponse struct {
	Members    []clubMemberJSON `json:"members"`
	NextCursor string           `json:"nextCursor"`
	Total      int              `json:"total"`
}

// myClubJSON: Pinned is in the caller's left menu.
type myClubJSON struct {
	clubJSON
	Role   string `json:"role"`
	Pinned bool   `json:"pinned"`
}

type myClubsResponse struct {
	Clubs      []myClubJSON `json:"clubs"`
	NextCursor string       `json:"nextCursor"`
	Total      int          `json:"total"`
}

type clubListResponse struct {
	Clubs      []clubJSON `json:"clubs"`
	NextCursor string     `json:"nextCursor"`
}

type userClubsResponse struct {
	Clubs      []clubJSON `json:"clubs"`
	NextCursor string     `json:"nextCursor"`
	Total      int        `json:"total"`
}

func toClubJSON(c model.Club) clubJSON {
	return clubJSON{
		ID:            c.ID,
		Name:          c.Name,
		Description:   c.Description,
		Address:       c.Address,
		Website:       c.Website,
		Wall:          c.Wall,
		AdminsDisplay: c.AdminsDisplay,
		Members:       c.Members,
		CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
		Avatar:        toAvatarJSON(c.ID, c.AvatarID),
	}
}

func toClubsJSON(clubs []model.Club) []clubJSON {
	out := make([]clubJSON, 0, len(clubs))
	for _, c := range clubs {
		out = append(out, toClubJSON(c))
	}

	return out
}

// clubResponseOf adds the owner's profile to a club, looked up unless the
// caller knows it (owner, "" for none).
func (a *api) clubResponseOf(r *http.Request, v model.ClubView) (clubResponse, error) {
	resp := clubResponse{clubJSON: toClubJSON(v.Club), Role: v.Role}

	if v.OwnerID == "" {
		return resp, nil
	}

	people, err := a.authors(r, []string{v.OwnerID})
	if err != nil {
		return clubResponse{}, err
	}

	if owner, ok := people[v.OwnerID]; ok && !owner.Club {
		o := toAuthorJSON(owner)
		resp.Owner = &o
	}

	return resp, nil
}

func (a *api) writeClub(w http.ResponseWriter, r *http.Request, code int, v model.ClubView) {
	resp, err := a.clubResponseOf(r, v)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, code, resp)
}

// completedMe is the caller, who needs a filled-in profile to make and
// join clubs, as to post and follow: without one there is no name to show
// in the club.
func (a *api) completedMe(r *http.Request, token, refusal string) (model.User, error) {
	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		return model.User{}, err
	}

	if !me.ProfileCompleted {
		return model.User{}, failure.New(failure.InvalidArgument, refusal)
	}

	return me, nil
}

// createClub makes a club owned by the caller, as openvk's "Создать
// группу": a name and a description.
func (a *api) createClub(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req createClubRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before creating a club"); err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.clubs.Create(r.Context(), token, req.Name, req.Description)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeClub(w, r, http.StatusCreated, v)
}

// getClub is the page of a club, by its ID or its address.
func (a *api) getClub(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.clubs.Get(r.Context(), token, r.PathValue("ref"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeClub(w, r, http.StatusOK, v)
}

// club is the club of the path for a caller with a token.
func (a *api) club(r *http.Request, token string) (model.ClubView, error) {
	return a.clubs.Get(r.Context(), token, r.PathValue("ref"))
}

// updateClub saves the settings form of a club; the club service lets
// only its admins.
func (a *api) updateClub(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req clubSettingsRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.clubs.Update(r.Context(), token, c.Club.ID, model.ClubChanges(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeClub(w, r, http.StatusOK, v)
}

// joinClub is "Вступить в группу": idempotent.
func (a *api) joinClub(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before joining a club"); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.changeMembership(w, r, token, a.clubs.Join)
}

// leaveClub is "Выйти из группы": idempotent; the owner cannot.
func (a *api) leaveClub(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.changeMembership(w, r, token, a.clubs.Leave)
}

func (a *api) changeMembership(w http.ResponseWriter, r *http.Request, token string, change func(ctx context.Context, token, clubID string) (model.ClubView, error)) {
	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := change(r.Context(), token, c.Club.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeClub(w, r, http.StatusOK, v)
}

// listClubMembers is "Участники" of a club, the newest first; with
// ?admins=true only the owner and the admins ("Руководство"). People whose
// page is deleted are left out.
func (a *api) listClubMembers(w http.ResponseWriter, r *http.Request) {
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

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	page, err := a.clubs.Members(r.Context(), c.Club.ID, query.Get("cursor"), limit, query.Get("admins") == "true")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, len(page.Members))
	for i, m := range page.Members {
		ids[i] = m.UserID
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := clubMembersResponse{Members: make([]clubMemberJSON, 0, len(page.Members)), NextCursor: page.NextCursor, Total: page.Total}
	for _, m := range page.Members {
		if p, ok := people[m.UserID]; ok && !p.Club {
			resp.Members = append(resp.Members, clubMemberJSON{User: toAuthorJSON(p), Role: m.Role})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// appointAdmin and dismissAdmin are the owner's "Назначить
// администратором" and "Снять полномочия администратора".
func (a *api) appointAdmin(w http.ResponseWriter, r *http.Request) {
	a.setAdmin(w, r, true)
}

func (a *api) dismissAdmin(w http.ResponseWriter, r *http.Request) {
	a.setAdmin(w, r, false)
}

func (a *api) setAdmin(w http.ResponseWriter, r *http.Request, admin bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	user, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.clubs.SetAdmin(r.Context(), token, c.Club.ID, user.ID, admin); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// transferClub is the owner's "Назначить владельцем": the club goes to
// one of its admins, and the owner stays an admin.
func (a *api) transferClub(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	user, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.clubs.Transfer(r.Context(), token, c.Club.ID, user.ID); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listUserClubs is the block "Группы" of a user's page and its list, as
// the user's "Кому видно мои группы" allows; the clubs of a deleted page
// are hidden.
func (a *api) listUserClubs(w http.ResponseWriter, r *http.Request) {
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

	user, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	allowed, err := a.users.CanSeeClubs(r.Context(), token, user.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !allowed {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "this user's clubs are hidden"))
		return
	}

	page, err := a.clubs.OfUser(r.Context(), user.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, userClubsResponse{Clubs: toClubsJSON(page.Clubs), NextCursor: page.NextCursor, Total: page.Total})
}

// listMyClubs is "Мои Группы": the caller's clubs, the newest joined
// first; with ?managed=true only those they run ("Управление").
func (a *api) listMyClubs(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	page, err := a.clubs.Mine(r.Context(), token, query.Get("cursor"), limit, query.Get("managed") == "true")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := myClubsResponse{Clubs: make([]myClubJSON, 0, len(page.Clubs)), NextCursor: page.NextCursor, Total: page.Total}
	for _, m := range page.Clubs {
		resp.Clubs = append(resp.Clubs, myClubJSON{clubJSON: toClubJSON(m.Club), Role: m.Role, Pinned: m.Pinned})
	}

	writeJSON(w, http.StatusOK, resp)
}

// listPinnedClubs are the clubs in the caller's left menu.
func (a *api) listPinnedClubs(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	clubs, err := a.clubs.Pinned(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, clubListResponse{Clubs: toClubsJSON(clubs)})
}

// searchClubs finds clubs by name or address: GET /api/v1/clubs?q=.
func (a *api) searchClubs(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	page, err := a.clubs.Search(r.Context(), query.Get("q"), query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, clubListResponse{Clubs: toClubsJSON(page.Clubs), NextCursor: page.NextCursor})
}

// pinClub and unpinClub are openvk's "Добавить в левое меню" and "Убрать
// из левого меню", for the clubs the caller runs.
func (a *api) pinClub(w http.ResponseWriter, r *http.Request) {
	a.setPinnedClub(w, r, true)
}

func (a *api) unpinClub(w http.ResponseWriter, r *http.Request) {
	a.setPinnedClub(w, r, false)
}

func (a *api) setPinnedClub(w http.ResponseWriter, r *http.Request, pinned bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.clubs.Pin(r.Context(), token, c.Club.ID, pinned); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// uploadClubAvatar takes the raw image as the request body and makes it
// the club's photo, as the user's own: the old one goes only once the club
// shows the new one, and ?onWall=true posts it for the club ("обновило
// фотографию группы"). Only its admins may; media trusts the gateway to
// have checked.
func (a *api) uploadClubAvatar(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.managedClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	image, err := readImage(w, r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	onWall, err := a.newAvatarPost(r, token, image)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	avatarID, err := a.media.UploadClubAvatar(r.Context(), token, c.Club.ID, image)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.clubs.SetAvatar(r.Context(), token, c.Club.ID, avatarID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.cleanUpClubAvatars(r, token, c.Club.ID, avatarID)

	if onWall != nil {
		if err = onWall(model.NewPost{WallID: c.Club.ID, ClubWall: true, FromWall: true}); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	a.writeClub(w, r, http.StatusOK, v)
}

func (a *api) deleteClubAvatar(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.managedClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.clubs.SetAvatar(r.Context(), token, c.Club.ID, "")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.cleanUpClubAvatars(r, token, c.Club.ID, "")
	a.writeClub(w, r, http.StatusOK, v)
}

// managedClub is the club of the path, which the caller must run.
func (a *api) managedClub(r *http.Request, token string) (model.ClubView, error) {
	c, err := a.club(r, token)
	if err != nil {
		return model.ClubView{}, err
	}

	if !manages(c.Role) {
		return model.ClubView{}, failure.New(failure.PermissionDenied, "only the admins can edit the club")
	}

	return c, nil
}

func manages(role string) bool {
	return role == "admin" || role == "owner"
}

// cleanUpClubAvatars deletes photos the club no longer shows. A failure
// only leaves unused files behind, so it is logged, not returned.
func (a *api) cleanUpClubAvatars(r *http.Request, token, clubID, keep string) {
	if err := a.media.DeleteClubAvatars(r.Context(), token, clubID, keep); err != nil {
		a.logger.WarnContext(r.Context(), "old club avatars left behind", "error", err)
	}
}

// listClubPosts is the wall of a club. Its first page tells a signed-in
// viewer whether they may write there on their own behalf.
func (a *api) listClubPosts(w http.ResponseWriter, r *http.Request) {
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

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	cursor := r.URL.Query().Get("cursor")

	page, err := a.posts.List(r.Context(), token, c.Club.ID, cursor, limit)
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
		access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, false)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		resp.CanPost, resp.CanSuggest = access.Post, access.Suggest
	}

	writeJSON(w, http.StatusOK, resp)
}

// createClubPost writes on the wall of a club, on the author's behalf or,
// for its admins, on the club's, maybe signed. As in openvk, its admins
// always speak for a club whose wall is not open.
func (a *api) createClubPost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req clubPostRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before posting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	fromClub := req.FromClub || (c.Club.Wall != "open" && manages(c.Role))

	access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, fromClub)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !access.Post {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot post on this wall"))
		return
	}

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Create(r.Context(), token, model.NewPost{
		WallID:   c.Club.ID,
		Text:     req.Text,
		PhotoIDs: req.Photos,
		ClubWall: true,
		FromWall: fromClub,
		Signed:   fromClub && req.Signed,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p.Author, p.Wall = authorOf(me.ID, me.AvatarID, me.Profile), clubAuthor(c.Club)

	writeJSON(w, http.StatusCreated, toPostResponse(p))
}
