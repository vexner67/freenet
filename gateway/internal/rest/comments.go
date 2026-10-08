package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type commentRequest struct {
	Text string `json:"text"`
	// Photos are IDs from POST /api/v1/me/photos.
	Photos []string `json:"photos"`
	// ReplyTo is the comment of the same post this one answers.
	ReplyTo string `json:"replyTo"`
}

type commentResponse struct {
	ID        string      `json:"id"`
	PostID    string      `json:"postId"`
	Author    authorJSON  `json:"author"`
	Text      string      `json:"text"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	// ReplyTo is the comment this one answers: null for none, and once it
	// is deleted.
	ReplyTo *string `json:"replyTo"`
	Likes   int     `json:"likes"`
	Liked   bool    `json:"liked"`
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
		Photos:    toPhotosJSON(c.Author.ID, c.PhotoIDs),
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		ReplyTo:   optionalString(c.ReplyTo),
		Likes:     c.Likes,
		Liked:     c.Liked,
	}
}

// optionalString is s, null when empty.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// listComments is public; with a token, liked says which the caller likes.
func (a *api) listComments(w http.ResponseWriter, r *http.Request) {
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

	page, err := a.posts.Comments(r.Context(), token, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
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

	if err = a.checkNotBlocked(r, token, me.ID, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.posts.CreateComment(r.Context(), token, r.PathValue("id"), model.NewComment{Text: req.Text, PhotoIDs: req.Photos, ReplyTo: req.ReplyTo})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c.Author = authorOf(me.ID, me.AvatarID, me.Profile)

	writeJSON(w, http.StatusCreated, toCommentResponse(c))
}

// likeComment and unlikeComment are idempotent, as for posts.
func (a *api) likeComment(w http.ResponseWriter, r *http.Request) {
	a.setCommentLike(w, r, true)
}

func (a *api) unlikeComment(w http.ResponseWriter, r *http.Request) {
	a.setCommentLike(w, r, false)
}

func (a *api) setCommentLike(w http.ResponseWriter, r *http.Request, liked bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	set := a.posts.UnlikeComment
	if liked {
		set = a.posts.LikeComment
	}

	likes, err := set(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, likeResponse{Likes: likes, Liked: liked})
}

// checkNotBlocked refuses a comment under the post of someone who
// blacklisted the caller, or whom the caller blacklisted: the author of the
// post and the owner of its wall. A community blacklists nobody, and the
// author of a post that speaks for one is not to be told by a refusal.
func (a *api) checkNotBlocked(r *http.Request, token, myID, postID string) error {
	p, err := a.posts.Get(r.Context(), token, postID)
	if err != nil {
		return err
	}

	people := []string{p.Wall.ID}
	if !p.FromWall {
		people = append(people, p.Author.ID)
	}

	for _, id := range unique(people) {
		if id == myID {
			continue
		}

		rel, err := a.users.Relation(r.Context(), token, id)
		if err != nil {
			return err
		}

		if rel.Blocked() {
			return failure.New(failure.PermissionDenied, "you cannot comment on this post")
		}
	}

	return nil
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
