package rest

import (
	"context"
	"net/http"

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
// newest first, each with its author.
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

	page, err := a.posts.Feed(r.Context(), token, append(following, me.ID), r.URL.Query().Get("cursor"), limit)
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

// authors looks up the authors of posts or comments in one request; an
// author who deleted the account is missing from the result.
func (a *api) authors(r *http.Request, authorIDs []string) (map[string]model.Author, error) {
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

	profiles, err := a.users.GetProfiles(r.Context(), ids)
	if err != nil {
		return nil, err
	}

	authors := make(map[string]model.Author, len(profiles))
	for _, p := range profiles {
		authors[p.ID] = authorOf(p.ID, p.AvatarID, p.Profile)
	}

	return authors, nil
}
