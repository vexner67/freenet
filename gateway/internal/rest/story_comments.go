package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// storyCommentRequest is a comment to write under a story.
type storyCommentRequest struct {
	Text string `json:"text"`
}

// storyCommentJSON is a comment under a story; mine: the viewer wrote it.
type storyCommentJSON struct {
	ID        string     `json:"id"`
	Author    authorJSON `json:"author"`
	Text      string     `json:"text"`
	CreatedAt string     `json:"createdAt"`
	Mine      bool       `json:"mine"`
}

type storyCommentPageResponse struct {
	Comments   []storyCommentJSON `json:"comments"`
	NextCursor string             `json:"nextCursor"`
	Total      int                `json:"total"`
}

func toStoryCommentJSON(c model.StoryComment, author model.Author) storyCommentJSON {
	return storyCommentJSON{
		ID:        c.ID,
		Author:    toAuthorJSON(author),
		Text:      c.Text,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		Mine:      c.Mine,
	}
}

// likeStory likes a story the caller sees; its author hears of it.
func (a *api) likeStory(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stories.Like(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) unlikeStory(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stories.Unlike(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listStoryComments are the comments of a story the caller sees, the
// oldest first; those of deleted pages are left out.
func (a *api) listStoryComments(w http.ResponseWriter, r *http.Request) {
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

	page, err := a.stories.Comments(r.Context(), token, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(page.Comments))
	for _, c := range page.Comments {
		ids = append(ids, c.AuthorID)
	}

	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := storyCommentPageResponse{
		Comments:   make([]storyCommentJSON, 0, len(page.Comments)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}
	for _, c := range page.Comments {
		if author, ok := people[c.AuthorID]; ok && !author.Deactivated {
			resp.Comments = append(resp.Comments, toStoryCommentJSON(c, author))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// createStoryComment needs a filled-in profile, as commenting a post
// does, and no blacklist between the caller and the author of the story.
func (a *api) createStoryComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req storyCommentRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before commenting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	story, err := a.stories.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if story.AuthorID != me.ID {
		rel, err := a.users.Relation(r.Context(), token, story.AuthorID)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		if rel.Blocked() {
			a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot comment on this story"))
			return
		}
	}

	c, err := a.stories.Comment(r.Context(), token, story.ID, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toStoryCommentJSON(c, meAuthor(me)))
}

// deleteStoryComment: its author or the author of the story, which the
// post service checks.
func (a *api) deleteStoryComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.stories.DeleteComment(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
