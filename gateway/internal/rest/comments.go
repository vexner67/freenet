package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type commentRequest struct {
	Text string `json:"text"`
}

type commentResponse struct {
	ID        string     `json:"id"`
	PostID    string     `json:"postId"`
	Author    authorJSON `json:"author"`
	Text      string     `json:"text"`
	CreatedAt string     `json:"createdAt"`
}

type commentPageResponse struct {
	Comments   []commentResponse `json:"comments"`
	NextCursor string            `json:"nextCursor"`
	Total      int               `json:"total"`
}

func toCommentResponse(c model.Comment) commentResponse {
	return commentResponse{
		ID:        c.ID,
		PostID:    c.PostID,
		Author:    toAuthorJSON(c.Author),
		Text:      c.Text,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (a *api) listComments(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.posts.Comments(r.Context(), r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, len(page.Comments))
	for i, c := range page.Comments {
		ids[i] = c.Author.ID
	}

	authors, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := commentPageResponse{
		Comments:   make([]commentResponse, 0, len(page.Comments)),
		NextCursor: page.NextCursor,
		Total:      page.Total,
	}

	for _, c := range page.Comments {
		// An author who deleted the account meanwhile is left out.
		author, ok := authors[c.Author.ID]
		if !ok {
			continue
		}

		c.Author = author
		resp.Comments = append(resp.Comments, toCommentResponse(c))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createComment needs a filled-in profile, as posting does: a comment is
// shown with the name and the link of its author.
func (a *api) createComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req commentRequest
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
		a.writeError(w, r, failure.New(failure.InvalidArgument, "fill in your profile before commenting"))
		return
	}

	c, err := a.posts.CreateComment(r.Context(), token, r.PathValue("id"), req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c.Author = authorOf(me.ID, me.AvatarID, me.Profile)

	writeJSON(w, http.StatusCreated, toCommentResponse(c))
}

func (a *api) deleteComment(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.posts.DeleteComment(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
