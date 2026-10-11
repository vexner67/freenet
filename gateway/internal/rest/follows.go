package rest

import (
	"context"
	"maps"
	"net/http"
	"slices"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type followStatsJSON struct {
	FollowersCount int  `json:"followersCount"`
	FollowingCount int  `json:"followingCount"`
	IsFollowing    bool `json:"isFollowing"`
}

func toFollowStatsJSON(s model.FollowStats) followStatsJSON {
	return followStatsJSON{
		FollowersCount: s.Followers,
		FollowingCount: s.Following,
		IsFollowing:    s.IsFollowing,
	}
}

// profileWithStatsResponse is the page of a person: the profile with the
// rest of the page, the follows and the blacklist between the viewer and
// them.
type profileWithStatsResponse struct {
	publicProfileResponse
	aboutJSON
	followStatsJSON
	relationJSON
	Access accessJSON `json:"access"`
	// IgnoredByMe: the viewer's news leave this wall out.
	IgnoredByMe bool `json:"ignoredByMe"`
}

// accessJSON is which blocks of a page the viewer may see: the albums, the
// follows, the communities and the notes.
type accessJSON struct {
	Photos  bool `json:"photos"`
	Friends bool `json:"friends"`
	Clubs   bool `json:"clubs"`
	Notes   bool `json:"notes"`
}

type userPageResponse struct {
	Users      []publicProfileResponse `json:"users"`
	NextCursor string                  `json:"nextCursor"`
	Total      int                     `json:"total"`
}

func (a *api) follow(w http.ResponseWriter, r *http.Request) {
	a.changeFollow(w, r, a.users.Follow)
}

func (a *api) unfollow(w http.ResponseWriter, r *http.Request) {
	a.changeFollow(w, r, a.users.Unfollow)
}

// changeFollow follows or unfollows the user of the path and answers with
// their fresh counters, so the page can update without another request.
// Like posting, it needs a filled-in profile: followers are shown by name.
func (a *api) changeFollow(w http.ResponseWriter, r *http.Request, change func(ctx context.Context, token, userID string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !me.ProfileCompleted {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "fill in your profile before following"))
		return
	}

	target, err := a.users.GetProfile(r.Context(), "", r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = change(r.Context(), token, target.ID); err != nil {
		a.writeError(w, r, err)
		return
	}

	stats, err := a.users.FollowStats(r.Context(), token, target.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toFollowStatsJSON(stats))
}

func (a *api) listFollowers(w http.ResponseWriter, r *http.Request) {
	a.listFollows(w, r, a.users.ListFollowers)
}

func (a *api) listFollowing(w http.ResponseWriter, r *http.Request) {
	// "?common=true" is openvk's "Общие друзья": whom the caller follows
	// too.
	if r.URL.Query().Get("common") == "true" {
		token, err := bearerToken(r)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		a.listFollows(w, r, func(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error) {
			return a.users.ListCommonFollowing(ctx, token, userID, cursor, limit)
		})
		return
	}

	// "?online=true" is openvk's "Друзья онлайн".
	if r.URL.Query().Get("online") == "true" {
		a.listFollows(w, r, a.users.ListOnlineFollowing)
		return
	}

	a.listFollows(w, r, a.users.ListFollowing)
}

func (a *api) listFollows(
	w http.ResponseWriter,
	r *http.Request,
	list func(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error),
) {
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

	// "Кому видно моих друзей": the follows, either way.
	owner, err := a.visiblePage(r, token, friendsPart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := list(r.Context(), owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := userPageResponse{
		Users:      make([]publicProfileResponse, 0, len(page.Users)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	for _, u := range page.Users {
		resp.Users = append(resp.Users, toPublicProfileResponse(u))
	}

	writeJSON(w, http.StatusOK, resp)
}

// feedAll is "Все новости" of openvk: the posts of every wall, whoever the
// reader follows.
const feedAll = "all"

// feed is the caller's news: posts of the people they follow and their own,
// and every post on the walls of those people, their own and their clubs',
// newest first, each with its author. openvk's feed is the walls; ours
// also follows its people to the walls of others. "Настройки ленты" of
// openvk come with the request: alien=false takes away what people wrote
// on the walls of others, ignored=true brings back the walls the reader
// ignores.
func (a *api) feed(w http.ResponseWriter, r *http.Request) {
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

	alien, withIgnored := r.URL.Query().Get("alien") != "false", r.URL.Query().Get("ignored") == "true"

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// The news leave out what the people the reader blacklisted wrote.
	blocked, err := a.users.BlockedIDs(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	// "Не показывать в ленте" of the reader's settings.
	sources := model.FeedSources{HideNSFW: me.Interface.NSFW == nsfwHide, Hidden: blocked, OwnOnly: !alien}

	if !withIgnored {
		if sources.Ignored, err = a.users.IgnoredIDs(r.Context(), token); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	switch r.URL.Query().Get("type") {
	case "":
		following, err := a.users.FollowingIDs(r.Context(), token)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		clubs, err := a.clubs.MineIDs(r.Context(), token)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		// Who is ignored is not followed to the walls of others either.
		people := without(append(following, me.ID), sources.Ignored)
		sources.Authors, sources.Walls = people, append(without(clubs, sources.Ignored), people...)
	case feedAll:
		sources.Everyone = true
	default:
		a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid feed type"))
		return
	}

	page, err := a.posts.Feed(r.Context(), token, sources, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := postPageResponse{NextCursor: page.NextCursor}

	if resp.Posts, err = a.withPeople(r, page.Posts); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.markModerated(r, token, resp.Posts); err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// markModerated marks the posts on the walls of the communities the reader
// runs: in the news they delete, pin and put those away as on the wall.
func (a *api) markModerated(r *http.Request, token string, posts []postResponse) error {
	var clubs []string
	for _, p := range posts {
		if id := clubWall(p); id != "" {
			clubs = append(clubs, id)
		}
	}

	if len(clubs) == 0 {
		return nil
	}

	// A page of the news is at most 50 posts, fewer than one request takes.
	mine, err := a.clubs.MyRoles(r.Context(), token, unique(clubs))
	if err != nil {
		return err
	}

	for i, p := range posts {
		posts[i].Moderated = manages(mine[clubWall(p)])
	}

	return nil
}

// clubWall is the community whose wall the post is on, "" for a person's:
// a post in its name has it as the author.
func clubWall(p postResponse) string {
	switch {
	case p.Wall != nil && p.Wall.Club:
		return p.Wall.ID
	case p.Wall == nil && p.Author.Club:
		return p.Author.ID
	default:
		return ""
	}
}

// maxProfilesPerRequest is what one GetProfiles of the user service takes,
// maxClubsPerRequest one GetClubs of the club service.
const (
	maxProfilesPerRequest = 100
	maxClubsPerRequest    = 100
)

// authors looks up the authors of posts, comments or notifications, and
// the communities among the owners of walls; people whose page is deleted
// or erased are missing from the result, so what they wrote is hidden.
func (a *api) authors(r *http.Request, authorIDs []string) (map[string]model.Author, error) {
	people, err := a.people(r, authorIDs)
	if err != nil {
		return nil, err
	}

	// Who is no user may be a community: the owner of a wall, the voice of
	// the posts that speak for it.
	var rest []string
	for _, id := range unique(authorIDs) {
		if _, ok := people[id]; !ok {
			rest = append(rest, id)
		}
	}

	maps.DeleteFunc(people, func(_ string, p model.Author) bool { return p.Deactivated })

	if len(rest) == 0 {
		return people, nil
	}

	if people == nil {
		people = make(map[string]model.Author, len(rest))
	}

	for chunk := range slices.Chunk(rest, maxClubsPerRequest) {
		clubs, err := a.clubs.GetMany(r.Context(), chunk)
		if err != nil {
			return nil, err
		}

		for _, c := range clubs {
			people[c.ID] = clubAuthor(c)
		}
	}

	return people, nil
}

// people looks up users in as few requests as it can; those whose page is
// deleted are there, marked, and those erased are missing.
func (a *api) people(r *http.Request, authorIDs []string) (map[string]model.Author, error) {
	seen := make(map[string]bool, len(authorIDs))
	ids := make([]string, 0, len(authorIDs))

	for _, id := range authorIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return nil, nil
	}

	authors := make(map[string]model.Author, len(ids))

	// A page of reposts names up to four people a post.
	for chunk := range slices.Chunk(ids, maxProfilesPerRequest) {
		profiles, err := a.users.GetProfiles(r.Context(), chunk)
		if err != nil {
			return nil, err
		}

		for _, p := range profiles {
			authors[p.ID] = authorOfProfile(p)
		}
	}

	return authors, nil
}
