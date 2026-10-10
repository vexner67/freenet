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
	// FromClub: the comment speaks for the community whose wall the post
	// is on ("От имени сообщества"), its admins only.
	FromClub bool `json:"fromClub"`
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
	// FromWall: the comment speaks for the community, shown as its author;
	// who wrote it is not told. Mine: the viewer wrote it.
	FromWall bool `json:"fromWall"`
	Mine     bool `json:"mine"`
}

type commentPageResponse struct {
	Comments   []commentResponse `json:"comments"`
	NextCursor string            `json:"nextCursor"`
	Total      int               `json:"total"`
}

// toCommentResponse shows c by shown: its author, or the community it
// speaks for. Its photos stay filed under the author.
func toCommentResponse(c model.Comment, shown model.Author) commentResponse {
	return commentResponse{
		ID:        c.ID,
		PostID:    c.PostID,
		Author:    toAuthorJSON(shown),
		Text:      c.Text,
		Photos:    toPhotosJSON(c.Author.ID, c.PhotoIDs),
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		ReplyTo:   optionalString(c.ReplyTo),
		Likes:     c.Likes,
		Liked:     c.Liked,
		FromWall:  c.FromWall,
		Mine:      c.Mine,
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

	ids := make([]string, 0, len(page.Comments)+1)
	forWall := false
	for _, c := range page.Comments {
		ids = append(ids, c.Author.ID)
		forWall = forWall || c.FromWall
	}

	// Comments for the community show it: the owner of the post's wall.
	wallID := ""
	if forWall {
		p, err := a.posts.Get(r.Context(), token, r.PathValue("id"))
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		wallID = p.Wall.ID
		ids = append(ids, wallID)
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

		shown := author
		if c.FromWall {
			if shown, ok = authors[wallID]; !ok {
				continue
			}
		}

		resp.Comments = append(resp.Comments, toCommentResponse(c, shown))
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

	p, err := a.posts.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.checkNotBlocked(r, token, me.ID, p); err != nil {
		a.writeError(w, r, err)
		return
	}

	shown := authorOf(me.ID, me.AvatarID, me.Profile)
	if req.FromClub {
		if shown, err = a.speakForWall(r, token, p); err != nil {
			a.writeError(w, r, err)
			return
		}
	}

	if err = a.checkOwnPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.posts.CreateComment(r.Context(), token, p.ID, model.NewComment{
		Text: req.Text, PhotoIDs: req.Photos, ReplyTo: req.ReplyTo, FromWall: req.FromClub, Mentions: mentions,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c.Author = authorOf(me.ID, me.AvatarID, me.Profile)

	writeJSON(w, http.StatusCreated, toCommentResponse(c, shown))
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

// speakForWall is the community whose wall p is on, for a comment the
// caller writes on its behalf: the club service tells whether they may.
func (a *api) speakForWall(r *http.Request, token string, p model.Post) (model.Author, error) {
	refusal := failure.New(failure.PermissionDenied, "you cannot comment for this community")

	c, err := a.clubs.Get(r.Context(), token, p.Wall.ID)
	if failure.From(err).Kind == failure.NotFound {
		// A user's wall: no community to speak for.
		return model.Author{}, refusal
	}
	if err != nil {
		return model.Author{}, err
	}

	access, err := a.clubs.CanPostOnWall(r.Context(), token, c.Club.ID, true)
	if err != nil {
		return model.Author{}, err
	}

	if !access.Post {
		return model.Author{}, refusal
	}

	return clubAuthor(c.Club), nil
}

// checkNotBlocked refuses a comment under the post p of someone who
// blacklisted the caller, or whom the caller blacklisted: the author of the
// post and the owner of its wall. A community blacklists nobody, and the
// author of a post that speaks for one is not to be told by a refusal.
func (a *api) checkNotBlocked(r *http.Request, token, myID string, p model.Post) error {
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
