package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// storyRequest is a story to tell: a photo uploaded with POST /me/photos
// and a few words over it.
type storyRequest struct {
	Photo string `json:"photo"`
	Text  string `json:"text"`
}

// storyJSON is a photo with a few words shown for a day.
type storyJSON struct {
	ID        string    `json:"id"`
	Photo     photoJSON `json:"photo"`
	Text      string    `json:"text"`
	CreatedAt string    `json:"createdAt"`
	ExpiresAt string    `json:"expiresAt"`
	Viewed    bool      `json:"viewed"`
	// Mine: the viewer told it, and Views others viewed it.
	Mine  bool `json:"mine,omitempty"`
	Views int  `json:"views"`
	// Liked: the viewer liked it; Likes and Comments count them.
	Liked    bool `json:"liked"`
	Likes    int  `json:"likes"`
	Comments int  `json:"comments"`
}

// storyAuthorJSON is a person with their live stories, the oldest first;
// Viewed: the viewer viewed them all.
type storyAuthorJSON struct {
	Author  authorJSON  `json:"author"`
	Stories []storyJSON `json:"stories"`
	Viewed  bool        `json:"viewed"`
}

type storiesResponse struct {
	Authors []storyAuthorJSON `json:"authors"`
}

func toStoryJSON(s model.Story) storyJSON {
	return storyJSON{
		ID:        s.ID,
		Photo:     toPhotoJSON(s.AuthorID, s.PhotoID),
		Text:      s.Text,
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt: s.ExpiresAt.UTC().Format(time.RFC3339),
		Viewed:    s.Viewed,
		Mine:      s.Mine,
		Views:     s.Views,
		Liked:     s.Liked,
		Likes:     s.Likes,
		Comments:  s.Comments,
	}
}

// listFeedStories are the stories above the news: the caller's and of
// the people they follow, without those the caller blacklisted or
// ignores in the news.
func (a *api) listFeedStories(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	blocked, err := a.users.BlockedIDs(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ignored, err := a.users.IgnoredIDs(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	stories, err := a.stories.Feed(r.Context(), token, blocked, ignored)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeStories(w, r, stories)
}

// listUserStories are the live stories of the person of the path, for
// those who see the page.
func (a *api) listUserStories(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.visiblePage(r, token, pagePart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	blocked, err := a.users.BlockedIDs(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	stories, err := a.stories.User(r.Context(), token, owner.ID, blocked)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.writeStories(w, r, stories)
}

// writeStories writes the stories grouped by author in the order they
// came; those of deleted pages are left out.
func (a *api) writeStories(w http.ResponseWriter, r *http.Request, stories []model.Story) {
	ids := make([]string, len(stories))
	for i, s := range stories {
		ids[i] = s.AuthorID
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := storiesResponse{Authors: []storyAuthorJSON{}}
	at := make(map[string]int)
	for _, s := range stories {
		author, ok := people[s.AuthorID]
		if !ok || author.Club {
			continue
		}

		i, ok := at[s.AuthorID]
		if !ok {
			i = len(resp.Authors)
			at[s.AuthorID] = i
			resp.Authors = append(resp.Authors, storyAuthorJSON{Author: toAuthorJSON(author), Viewed: true})
		}

		group := &resp.Authors[i]
		group.Stories = append(group.Stories, toStoryJSON(s))
		group.Viewed = group.Viewed && s.Viewed
	}

	writeJSON(w, http.StatusOK, resp)
}

// tellStory tells a story of the caller with a photo they uploaded; a
// filled-in profile is needed, as to post.
func (a *api) tellStory(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req storyRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before posting"); err != nil {
		a.writeError(w, r, err)
		return
	}

	photos := []string{req.Photo}
	if err = a.claimPhotos(r, token, photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	story, err := a.stories.Tell(r.Context(), token, req.Photo, req.Text)
	if err != nil {
		a.releasePhotos(r, token, photos, err)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toStoryJSON(story))
}

// viewStory tells that the caller viewed a story.
func (a *api) viewStory(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stories.View(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listStoryViewers are who viewed a story of the caller, as people.
func (a *api) listStoryViewers(w http.ResponseWriter, r *http.Request) {
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

	page, err := a.stories.Viewers(r.Context(), token, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp, err := a.peoplePage(r, page)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *api) deleteStory(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stories.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
