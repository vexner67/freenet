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

type profileWithStatsResponse struct {
	publicProfileResponse
	followStatsJSON
	relationJSON
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

	target, err := a.users.GetProfile(r.Context(), r.PathValue("nickname"))
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
	a.listFollows(w, r, a.users.ListFollowing)
}

func (a *api) listFollows(
	w http.ResponseWriter,
	r *http.Request,
	list func(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error),
) {
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

// feed is the caller's news: posts of the people they follow and their own,
// and every post on the walls of their clubs, newest first, each with its
// author.
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

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

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

	sources := model.FeedSources{Authors: append(following, me.ID), Walls: clubs}

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

	writeJSON(w, http.StatusOK, resp)
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
