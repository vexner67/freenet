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
	// "Обсуждения": anyone starts topics ("Все могут создавать темы"),
	// listed above the wall ("Отображать список тем над стеной").
	OpenTopics      bool `json:"openTopics"`
	TopicsAboveWall bool `json:"topicsAboveWall"`
	// Closed is "Тип группы: закрытая".
	Closed bool `json:"closed"`
}

type clubPostRequest struct {
	Text   string   `json:"text"`
	Photos []string `json:"photos"`
	// FromClub: the post speaks for the club ("От имени сообщества"),
	// naming its author with Signed ("Подпись автора").
	FromClub bool `json:"fromClub"`
	Signed   bool `json:"signed"`
	// NSFW marks the post as adult.
	NSFW bool `json:"nsfw"`
	// Poll is the poll the post asks, as on a person's wall.
	Poll *pollRequest `json:"poll"`
	// Documents are IDs of the caller's documents the post attaches.
	Documents []string `json:"documents"`
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

	OpenTopics      bool `json:"openTopics"`
	TopicsAboveWall bool `json:"topicsAboveWall"`
	// Backdrop is "Фон страницы", null for none.
	Backdrop *backdropJSON `json:"backdrop"`

	// Closed: one joins by a request its admins accept, and only the
	// members see what is in the club.
	Closed bool `json:"closed"`
	// Verified: the club wears the checkmark of a confirmed one.
	Verified bool `json:"verified"`
}

// backdropJSON is the photos on the left and on the right of a club's
// page; either may be null.
type backdropJSON struct {
	Left  *photoJSON `json:"left"`
	Right *photoJSON `json:"right"`
}

// clubResponse is a club as the viewer sees it: role is "" (not a member,
// or anonymous), "member", "admin" or "owner"; owner is null when the
// owner's page is deleted or erased, or the club hides its owner from the
// viewer; ownerTitle is what the owner does in the club.
type clubResponse struct {
	clubJSON
	Role       string      `json:"role"`
	Owner      *authorJSON `json:"owner"`
	OwnerTitle string      `json:"ownerTitle"`
	// CanStartTopic: "Создать тему" for the viewer.
	CanStartTopic bool `json:"canStartTopic"`
	// IgnoredByMe: the viewer's news leave this wall out; told only by
	// the page of the club.
	IgnoredByMe bool `json:"ignoredByMe,omitempty"`
	// Requested: the viewer asked to join the closed club and waits;
	// Requests: how many do, told to those who run it.
	Requested bool `json:"requested"`
	Requests  int  `json:"requests"`
}

// joinRequestsResponse are those who ask to join a club, the newest
// first; Total counts them on all pages, those whose page is deleted too.
type joinRequestsResponse struct {
	People     []authorJSON `json:"people"`
	NextCursor string       `json:"nextCursor"`
	Total      int          `json:"total"`
}

// clubMemberJSON: title is what an admin does in the club; hidden: the
// club shows them as a plain member to everyone who does not run it.
type clubMemberJSON struct {
	User   authorJSON `json:"user"`
	Role   string     `json:"role"`
	Title  string     `json:"title"`
	Hidden bool       `json:"hidden"`
}

// describeAdminRequest: title is at most 36 characters, "" for none. A
// field that is not sent stays as it is, so two people changing
// different things of one admin undo nothing of each other's.
type describeAdminRequest struct {
	Title *string `json:"title"`
	// BaseTitle is the title the change started from; with it a seat
	// that says another by now is not written over.
	BaseTitle *string `json:"baseTitle"`
	Hidden    *bool   `json:"hidden"`
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

// userClubJSON is a club in someone's list: Role is what the viewer is in
// it, "" for none and without a token.
type userClubJSON struct {
	clubJSON
	Role string `json:"role"`
}

type userClubsResponse struct {
	Clubs      []userClubJSON `json:"clubs"`
	NextCursor string         `json:"nextCursor"`
	Total      int            `json:"total"`
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

		OpenTopics:      c.OpenTopics,
		TopicsAboveWall: c.TopicsAboveWall,
		Backdrop:        toBackdropJSON(c.ID, model.Backdrop{Left: c.BackdropLeft, Right: c.BackdropRight}),

		Closed:   c.Closed,
		Verified: c.Verified,
	}
}

// toBackdropJSON is b of the page of ownerID, a person or a club; nil
// for none.
func toBackdropJSON(ownerID string, b model.Backdrop) *backdropJSON {
	if b.Left == "" && b.Right == "" {
		return nil
	}

	side := func(id string) *photoJSON {
		if id == "" {
			return nil
		}
		p := toPhotoJSON(ownerID, id)
		return &p
	}

	return &backdropJSON{Left: side(b.Left), Right: side(b.Right)}
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
	resp := clubResponse{
		clubJSON: toClubJSON(v.Club), Role: v.Role, CanStartTopic: v.CanStartTopic,
		Requested: v.Requested, Requests: v.Requests,
	}

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
		resp.OwnerTitle = v.OwnerTitle
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

	resp, err := a.clubResponseOf(r, v)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if resp.IgnoredByMe, err = a.ignores(r, token, v.Club.ID); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// club is the club of the path for a caller with a token.
func (a *api) club(r *http.Request, token string) (model.ClubView, error) {
	return a.clubs.Get(r.Context(), token, r.PathValue("ref"))
}

// openClub is the club of the path, refused unless the caller, anonymous
// without token, sees what is in it: a closed club ("Закрытая группа")
// shows its wall, members, discussions and albums to its members only.
func (a *api) openClub(r *http.Request, token string) (model.ClubView, error) {
	c, err := a.club(r, token)
	if err != nil {
		return model.ClubView{}, err
	}

	if !c.Admits() {
		return model.ClubView{}, failure.New(failure.PermissionDenied, "this club is closed")
	}

	return c, nil
}

// listJoinRequests is "Заявки" of a closed club: who asks to join it, for
// those who run it.
func (a *api) listJoinRequests(w http.ResponseWriter, r *http.Request) {
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

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.clubs.JoinRequests(r.Context(), token, c.Club.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.authors(r, page.UserIDs)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := joinRequestsResponse{People: make([]authorJSON, 0, len(page.UserIDs)), NextCursor: page.NextCursor, Total: page.Total}
	for _, id := range page.UserIDs {
		if p, ok := people[id]; ok && !p.Club {
			resp.People = append(resp.People, toAuthorJSON(p))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// acceptJoinRequest and rejectJoinRequest are "Принять заявку" and
// "Отклонить заявку" of those who run a closed club.
func (a *api) acceptJoinRequest(w http.ResponseWriter, r *http.Request) {
	a.decideJoinRequest(w, r, true)
}

func (a *api) rejectJoinRequest(w http.ResponseWriter, r *http.Request) {
	a.decideJoinRequest(w, r, false)
}

func (a *api) decideJoinRequest(w http.ResponseWriter, r *http.Request, accept bool) {
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

	user, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.clubs.Decide(r.Context(), token, c.Club.ID, user.ID, accept); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	v, err := a.clubs.Update(r.Context(), token, c.Club.ID, model.ClubChanges{
		Name: req.Name, Description: req.Description, Address: req.Address, Website: req.Website,
		Wall: req.Wall, AdminsDisplay: req.AdminsDisplay,
		OpenTopics: req.OpenTopics, TopicsAboveWall: req.TopicsAboveWall,
		Closed: req.Closed,
	})
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

	c, err := a.openClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	filter := model.ClubMemberFilter{AdminsOnly: query.Get("admins") == "true"}

	page, err := a.clubs.Members(r.Context(), token, c.Club.ID, query.Get("cursor"), limit, filter)
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
			resp.Members = append(resp.Members, clubMemberJSON{User: toAuthorJSON(p), Role: m.Role, Title: m.Title, Hidden: m.Hidden})
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

	user, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
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

// describeAdmin is "Изменить комментарий" and "Скрыт" of openvk: those
// who run a club give one of them a title and hide or show them.
func (a *api) describeAdmin(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req describeAdminRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if req.Title == nil && req.Hidden == nil {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "title or hidden is required"))
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	user, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	change := model.SeatChange{Title: req.Title, BaseTitle: req.BaseTitle, Hidden: req.Hidden}
	if err = a.clubs.Describe(r.Context(), token, c.Club.ID, user.ID, change); err != nil {
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

	user, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
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

// deleteClub is the owner's "Удалить группу": the club goes with its
// members, wall, topics, albums and photos. openvk deletes no club.
func (a *api) deleteClub(w http.ResponseWriter, r *http.Request) {
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

	if err = a.clubs.Delete(r.Context(), token, c.Club.ID); err != nil {
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

	user, err := a.visiblePage(r, token, clubsPart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.clubs.OfUser(r.Context(), user.ID, r.URL.Query().Get("cursor"), limit, model.ClubFilter{})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// What the viewer is in each, for "Вступить в группу" and "Выйти из
	// группы" by it.
	mine, err := a.myRoles(r, token, page.Clubs)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := userClubsResponse{Clubs: make([]userClubJSON, 0, len(page.Clubs)), NextCursor: page.NextCursor, Total: page.Total}
	for _, c := range page.Clubs {
		resp.Clubs = append(resp.Clubs, userClubJSON{clubJSON: toClubJSON(c), Role: mine[c.ID]})
	}

	writeJSON(w, http.StatusOK, resp)
}

// myRoles tells what the viewer is in the clubs of a page; nothing for an
// anonymous viewer.
func (a *api) myRoles(r *http.Request, token string, clubs []model.Club) (map[string]string, error) {
	if token == "" || len(clubs) == 0 {
		return nil, nil
	}

	ids := make([]string, len(clubs))
	for i, c := range clubs {
		ids[i] = c.ID
	}

	return a.clubs.MyRoles(r.Context(), token, ids)
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

	filter := model.ClubFilter{Managed: query.Get("managed") == "true"}

	page, err := a.clubs.Mine(r.Context(), token, query.Get("cursor"), limit, filter)
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

// searchClubs finds clubs by name or address: GET /api/v1/clubs?q=; with
// ?verified=true, the clubs with the checkmark, words or not.
func (a *api) searchClubs(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	q := model.ClubQuery{Text: query.Get("q"), Verified: query.Get("verified") == "true"}

	page, err := a.clubs.Search(r.Context(), q, query.Get("cursor"), limit)
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
// shows the new one. Only its admins may; media trusts the gateway to
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
	a.keepInAvatarAlbum(r, token, c.Club.ID, image)
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

	c, err := a.openClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp, err := a.wallPage(r, token, c.Club.ID, limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if token != "" && r.URL.Query().Get("cursor") == "" {
		access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, false)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		resp.CanPost, resp.CanSuggest = access.Post, access.Suggest
	}

	writeJSON(w, http.StatusOK, resp)
}

// archiveClubWall and unarchiveClubWall put the posts of a year of a
// club's wall away, or all of them, and bring them back: its admins'.
func (a *api) archiveClubWall(w http.ResponseWriter, r *http.Request) {
	a.switchWall(w, r, a.wallOfClub, true)
}

func (a *api) unarchiveClubWall(w http.ResponseWriter, r *http.Request) {
	a.switchWall(w, r, a.wallOfClub, false)
}

// wallOfClub is the ID of the wall of the club the path names.
func (a *api) wallOfClub(r *http.Request, token string) (string, error) {
	c, err := a.club(r, token)
	if err != nil {
		return "", err
	}

	return c.Club.ID, nil
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

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.checkDocuments(r.Context(), me.ID, req.Documents); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.claimPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	link := a.linkCard(r, token, me.ID, req.Text)

	p, err := a.posts.Create(r.Context(), token, model.NewPost{
		Link:        link,
		DocumentIDs: req.Documents,
		WallID:      c.Club.ID,
		Text:        req.Text,
		PhotoIDs:    req.Photos,
		ClubWall:    true,
		FromWall:    fromClub,
		Signed:      fromClub && req.Signed,
		NSFW:        req.NSFW,
		Mentions:    mentions,
		Poll:        toNewPoll(req.Poll),
	})
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.dropCard(r, me.ID, link)
		a.writeError(w, r, err)
		return
	}

	p.Author, p.Wall = meAuthor(me), clubAuthor(c.Club)

	resp, err := a.asked(r, p)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}
