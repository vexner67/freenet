package rest

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// pictureMaxAge is how long a browser keeps a picture of another server:
// a photo of an account may change at the same address.
const pictureMaxAge = "public, max-age=86400"

// rankedCursorPrefix starts the cursors of the ranked news, which post
// makes: they are no ID to page remote posts by.
const rankedCursorPrefix = "r:"

// fediverseAccountJSON is an account of another server. Avatar is the
// address the site shows its photo at, "" for none.
type fediverseAccountJSON struct {
	ID       string `json:"id"`
	Handle   string `json:"handle"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Avatar   string `json:"avatar"`
	Accepted bool   `json:"accepted"`
	Telegram bool   `json:"telegram"`
}

type fediverseLookupJSON struct {
	Account    fediverseAccountJSON `json:"account"`
	Subscribed bool                 `json:"subscribed"`
}

type fediverseAccountsJSON struct {
	Accounts []fediverseAccountJSON `json:"accounts"`
}

type fediverseAddressRequest struct {
	Address string `json:"address"`
}

// fediverseJSON is what a post of another server has that the posts here
// do not: its author there, where it is, its pictures shown through the
// site and the warning before its text.
type fediverseJSON struct {
	Author  fediverseAccountJSON `json:"author"`
	URL     string               `json:"url"`
	Summary string               `json:"summary"`
	Images  []fediverseImageJSON `json:"images"`
	Videos  []fediverseVideoJSON `json:"videos"`
}

type fediverseImageJSON struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// fediverseVideoJSON is a video shown as its cover; Width, Height and
// Duration (in seconds) are 0 when its server did not tell them.
type fediverseVideoJSON struct {
	Cover       string `json:"cover"`
	Blurhash    string `json:"blurhash"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Duration    int    `json:"duration"`
	Description string `json:"description"`
}

func (a *api) registerFediverse(mux *http.ServeMux) {
	mux.HandleFunc("GET /ap/actor", a.siteActor)
	mux.HandleFunc("POST /ap/actor/inbox", a.inbox)

	mux.HandleFunc("GET /api/v1/fediverse/lookup", a.lookupFediverse)
	mux.HandleFunc("GET /api/v1/me/fediverse", a.fediverseSubscriptions)
	mux.HandleFunc("POST /api/v1/me/fediverse", a.subscribeFediverse)
	mux.HandleFunc("DELETE /api/v1/me/fediverse", a.unsubscribeFediverse)

	mux.HandleFunc("GET /api/v1/admin/fediverse", a.featuredFediverse)
	mux.HandleFunc("POST /api/v1/admin/fediverse", a.featureFediverse)
	mux.HandleFunc("DELETE /api/v1/admin/fediverse", a.unfeatureFediverse)

	mux.HandleFunc("PUT /api/v1/fediverse/posts/{id}/like", a.likeFediversePost)
	mux.HandleFunc("DELETE /api/v1/fediverse/posts/{id}/like", a.unlikeFediversePost)
	mux.HandleFunc("GET /api/v1/fediverse/posts/{id}/likes", a.listFediverseLikers)

	mux.HandleFunc("GET /media/fediverse/posts/{id}/{index}", a.fediversePostPicture)
	mux.HandleFunc("GET /media/fediverse/posts/{id}/link", a.fediverseLinkPicture)
	mux.HandleFunc("GET /media/fediverse/posts/{id}/videos/{index}", a.fediverseVideoCover)
	mux.HandleFunc("GET /media/fediverse/accounts", a.fediverseAccountPicture)
}

// siteActor is the site itself, which follows accounts of other servers
// for the news; their servers read it to check its signature.
func (a *api) siteActor(w http.ResponseWriter, r *http.Request) {
	doc, err := a.federation.SiteActor(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeDocument(w, activityType, doc)
}

func (a *api) lookupFediverse(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	found, err := a.federation.Lookup(r.Context(), token, r.URL.Query().Get("address"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, fediverseLookupJSON{Account: toFediverseAccountJSON(found.Account), Subscribed: found.Subscribed})
}

func (a *api) fediverseSubscriptions(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	accounts, err := a.federation.Subscriptions(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toFediverseAccountsJSON(accounts))
}

func (a *api) subscribeFediverse(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req fediverseAddressRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	account, err := a.federation.Subscribe(r.Context(), token, req.Address)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toFediverseAccountJSON(account))
}

func (a *api) unsubscribeFediverse(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.federation.Unsubscribe(r.Context(), token, r.URL.Query().Get("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// asAdmin lets only those who run the site through.
func (a *api) asAdmin(w http.ResponseWriter, r *http.Request) bool {
	token, err := bearerToken(r)
	if err == nil {
		_, err = a.adminID(r, token)
	}

	if err != nil {
		a.writeError(w, r, err)
		return false
	}

	return true
}

// featuredFediverse are the accounts of other servers in "Все новости".
func (a *api) featuredFediverse(w http.ResponseWriter, r *http.Request) {
	if !a.asAdmin(w, r) {
		return
	}

	accounts, err := a.federation.Featured(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toFediverseAccountsJSON(accounts))
}

func (a *api) featureFediverse(w http.ResponseWriter, r *http.Request) {
	if !a.asAdmin(w, r) {
		return
	}

	var req fediverseAddressRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	account, err := a.federation.Feature(r.Context(), req.Address)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toFediverseAccountJSON(account))
}

func (a *api) unfeatureFediverse(w http.ResponseWriter, r *http.Request) {
	if !a.asAdmin(w, r) {
		return
	}

	if err := a.federation.Unfeature(r.Context(), r.URL.Query().Get("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// fediversePostPicture is a picture of a post of another server: the
// page may show only pictures of the site (img-src 'self'), and so the
// reader's address stays unknown to that server.
func (a *api) fediversePostPicture(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		a.writeError(w, r, failure.New(failure.NotFound, "picture not found"))
		return
	}

	p, err := a.federation.PostPicture(r.Context(), r.PathValue("id"), index)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	servePicture(w, p)
}

// fediverseVideoCover is the cover of a video of a post of another
// server: the one its author chose, or its first frame.
func (a *api) fediverseVideoCover(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		a.writeError(w, r, failure.New(failure.NotFound, "picture not found"))
		return
	}

	p, err := a.federation.VideoCover(r.Context(), r.PathValue("id"), index)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	servePicture(w, p)
}

// fediverseLinkPicture is the picture of the card of the page a post of
// another server links to.
func (a *api) fediverseLinkPicture(w http.ResponseWriter, r *http.Request) {
	p, err := a.federation.LinkPicture(r.Context(), r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	servePicture(w, p)
}

// likeFediversePost and unlikeFediversePost put and take the heart of the
// caller on a post of another server; it stays here, as a like of a post
// of the site does. Both are idempotent.
func (a *api) likeFediversePost(w http.ResponseWriter, r *http.Request) {
	a.setFediverseLike(w, r, true)
}

func (a *api) unlikeFediversePost(w http.ResponseWriter, r *http.Request) {
	a.setFediverseLike(w, r, false)
}

func (a *api) setFediverseLike(w http.ResponseWriter, r *http.Request, liked bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	set := a.federation.UnlikeNews
	if liked {
		set = a.federation.LikeNews
	}

	likes, err := set(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, likeResponse{Likes: likes, Liked: liked})
}

// listFediverseLikers is "Понравилось N людям" of a post of another
// server: who here like it.
func (a *api) listFediverseLikers(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.listLikers(w, r, func(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error) {
		return a.federation.NewsLikers(ctx, token, id, cursor, limit)
	})
}

func (a *api) fediverseAccountPicture(w http.ResponseWriter, r *http.Request) {
	p, err := a.federation.AccountPicture(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	servePicture(w, p)
}

// servePicture serves only pictures, whatever another server said.
func servePicture(w http.ResponseWriter, p model.Picture) {
	if !strings.HasPrefix(p.ContentType, "image/") || strings.Contains(p.ContentType, "svg") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", p.ContentType)
	w.Header().Set("Cache-Control", pictureMaxAge)
	_, _ = w.Write(p.Body)
}

// withFediverse puts the posts of other servers among the news of the
// site, by time: their IDs are UUIDv7 of the time they were published, as
// are those of the posts here, and the cursor is the ID of the last post
// shown. The ranked news ("Сначала интересные") have none of them: posts
// there have no likes to be ranked by. A failure of federation leaves the
// news of the site as they are.
func (a *api) withFediverse(r *http.Request, token string, q model.FediverseNews, page postPageResponse) postPageResponse {
	if a.federation == nil || strings.HasPrefix(q.Before, rankedCursorPrefix) {
		return page
	}

	remote, err := a.federation.News(r.Context(), token, q)
	if err != nil {
		a.logger.WarnContext(r.Context(), "fediverse news", "error", err)
		return page
	}

	if len(remote) == 0 {
		return page
	}

	posts := slices.Clone(page.Posts)
	for _, p := range remote {
		posts = append(posts, toFediversePostResponse(p))
	}

	// UUIDv7 in their text sort by time.
	slices.SortStableFunc(posts, func(x, y postResponse) int { return cmp.Compare(y.ID, x.ID) })

	more := page.NextCursor != "" || len(remote) == q.Limit || len(posts) > q.Limit
	posts = posts[:min(len(posts), q.Limit)]

	page.Posts, page.NextCursor = posts, ""
	if more {
		page.NextCursor = posts[len(posts)-1].ID
	}

	return page
}

// The cursor of the ranked news carries, after remoteCursorMark, where the
// posts of other servers among them go on: the ID of the last one shown,
// or remoteNewsDone when they ended. siteNewsDone stands for the cursor of
// post when the posts of the site ended before them.
const (
	remoteCursorMark = "|f:"
	remoteNewsDone   = "-"
	siteNewsDone     = "end"
)

// remoteShare is how many posts of the site a post of another server
// comes after in the ranked news: one in three.
const remoteShare = 2

// splitRankedCursor is the cursor of post and that of the posts of other
// servers in a cursor of the ranked news.
func splitRankedCursor(cursor string) (site, remote string) {
	site, remote, _ = strings.Cut(cursor, remoteCursorMark)
	return site, remote
}

// withRankedFediverse puts the posts of other servers (Fediverse,
// Telegram) among the ranked news ("Сначала интересные"): they have no
// likes to be ranked by, so a page shows its posts of the site and after
// every remoteShare of them the next of those, newest first, by their
// own cursor; once the posts of the site end, those fill the page. A
// failure of federation leaves the news of the site.
func (a *api) withRankedFediverse(
	r *http.Request, token string, sources model.FeedSources, remoteCursor string, limit int, page postPageResponse,
) postPageResponse {
	siteNext := cmp.Or(page.NextCursor, siteNewsDone)

	var remote []model.FediversePost
	if a.federation != nil && remoteCursor != remoteNewsDone {
		size := cmp.Or(limit, defaultPageSize)
		take := max(1, size/(remoteShare+1))
		// The posts of the site ended: those of other servers fill the page.
		if siteNext == siteNewsDone {
			take = max(take, size-len(page.Posts))
		}

		var err error
		if remote, err = a.federation.News(r.Context(), token, fediverseNewsOf(sources, remoteCursor, take)); err != nil {
			a.logger.WarnContext(r.Context(), "fediverse news", "error", err)
			remote, remoteCursor = nil, remoteNewsDone
		} else {
			remoteCursor = remoteNewsDone
			if len(remote) == take {
				remoteCursor = remote[len(remote)-1].ID
			}
		}
	}

	posts := make([]postResponse, 0, len(page.Posts)+len(remote))
	for i, p := range page.Posts {
		posts = append(posts, p)
		if (i+1)%remoteShare == 0 && len(remote) > 0 {
			posts, remote = append(posts, toFediversePostResponse(remote[0])), remote[1:]
		}
	}
	for _, p := range remote {
		posts = append(posts, toFediversePostResponse(p))
	}

	page.Posts, page.NextCursor = posts, ""
	if siteNext != siteNewsDone || (remoteCursor != remoteNewsDone && remoteCursor != "") {
		page.NextCursor = siteNext + remoteCursorMark + cmp.Or(remoteCursor, remoteNewsDone)
	}

	return page
}

func toFediversePostResponse(p model.FediversePost) postResponse {
	images := make([]fediverseImageJSON, len(p.ImageDescriptions))
	for i, d := range p.ImageDescriptions {
		images[i] = fediverseImageJSON{
			URL:         "/media/fediverse/posts/" + url.PathEscape(p.ID) + "/" + strconv.Itoa(i),
			Description: d,
		}
	}

	videos := make([]fediverseVideoJSON, len(p.Videos))
	for i, v := range p.Videos {
		videos[i] = fediverseVideoJSON{
			Cover:       "/media/fediverse/posts/" + url.PathEscape(p.ID) + "/videos/" + strconv.Itoa(i),
			Blurhash:    v.Blurhash,
			Width:       v.Width,
			Height:      v.Height,
			Duration:    int(v.Duration / time.Second),
			Description: v.Description,
		}
	}

	page := p.PageURL
	if page == "" {
		page = p.URL
	}

	return postResponse{
		ID:        p.ID,
		Author:    authorJSON{Name: p.Author.Name},
		Text:      p.Text,
		Photos:    []photoJSON{},
		CreatedAt: timeString(p.PublishedAt),
		EditedAt:  optionalTime(p.UpdatedAt),
		NSFW:      p.Sensitive,
		Likes:     p.Likes,
		Liked:     p.Liked,
		Link:      toFediverseLinkJSON(p.ID, p.Link),
		Fediverse: &fediverseJSON{
			Author:  toFediverseAccountJSON(p.Author),
			URL:     page,
			Summary: p.Summary,
			Images:  images,
			Videos:  videos,
		},
	}
}

// toFediverseLinkJSON is the card of the page a post of another server
// links to; its picture comes through the site, as the pictures of the
// post do.
func toFediverseLinkJSON(postID string, l *model.FediverseLink) *linkJSON {
	if l == nil {
		return nil
	}

	j := &linkJSON{URL: l.URL, Title: l.Title, Description: l.Description, Site: l.Site}
	if l.HasImage {
		picture := "/media/fediverse/posts/" + url.PathEscape(postID) + "/link"
		j.Photo = &photoJSON{Large: picture, Small: picture}
	}

	return j
}

func toFediverseAccountJSON(acc model.FediverseAccount) fediverseAccountJSON {
	j := fediverseAccountJSON{
		ID:       acc.ID,
		Handle:   acc.Handle,
		Name:     acc.Name,
		URL:      cmp.Or(acc.PageURL, acc.ID),
		Accepted: acc.Accepted,
		Telegram: acc.Telegram,
	}

	if acc.HasAvatar {
		j.Avatar = "/media/fediverse/accounts?" + url.Values{"id": {acc.ID}}.Encode()
	}

	return j
}

func toFediverseAccountsJSON(accounts []model.FediverseAccount) fediverseAccountsJSON {
	out := fediverseAccountsJSON{Accounts: make([]fediverseAccountJSON, len(accounts))}
	for i, acc := range accounts {
		out.Accounts[i] = toFediverseAccountJSON(acc)
	}

	return out
}

// fediverseNewsOf is what remote posts the news ask for.
func fediverseNewsOf(sources model.FeedSources, cursor string, limit int) model.FediverseNews {
	return model.FediverseNews{
		Featured: sources.Everyone, Before: cursor, Limit: cmp.Or(limit, defaultPageSize), HideSensitive: sources.HideNSFW,
	}
}
