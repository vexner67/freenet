package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// lockPaid hides the photos and documents of paid posts from viewers who
// have not paid for them, telling only how many there are; the author
// sees them always. Those paid for are asked of the user service in one
// request; an anonymous viewer has paid for nothing.
func (a *api) lockPaid(r *http.Request, posts []model.Post) error {
	var priced []string
	for _, p := range posts {
		if p.Price > 0 && !p.Mine {
			priced = append(priced, p.ID)
		}
	}

	if len(priced) == 0 {
		return nil
	}

	bought := map[string]bool{}
	if token := viewerToken(r); token != "" {
		ids, err := a.users.BoughtPosts(r.Context(), token, unique(priced))
		if err != nil {
			return err
		}

		for _, id := range ids {
			bought[id] = true
		}
	}

	for i, p := range posts {
		if p.Price == 0 || p.Mine || bought[p.ID] {
			continue
		}

		posts[i].Locked = true
		posts[i].LockedPhotos, posts[i].LockedDocuments = len(p.PhotoIDs), len(p.DocumentIDs)
		posts[i].PhotoIDs, posts[i].DocumentIDs = nil, nil
	}

	return nil
}

// lockShared is lockPaid for the posts reposts share, kept by ID.
func (a *api) lockShared(r *http.Request, shared map[string]model.Post) error {
	posts := make([]model.Post, 0, len(shared))
	for _, p := range shared {
		posts = append(posts, p)
	}

	if err := a.lockPaid(r, posts); err != nil {
		return err
	}

	for _, p := range posts {
		shared[p.ID] = p
	}

	return nil
}

type purchaseResponse struct {
	// Balance is the votes the buyer has left.
	Balance int64        `json:"balance"`
	Post    postResponse `json:"post"`
}

// buyPost pays for the photos and documents of a paid post, at its price,
// to its author, and returns the post with them. The user service keeps
// the votes but does not know posts: the post is read here first, as the
// caller sees it, and its author and price told along.
func (a *api) buyPost(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := a.posts.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if p.Price == 0 || p.Mine {
		a.writeError(w, r, failure.New(failure.Conflict, "this post is not for sale"))
		return
	}

	balance, err := a.users.BuyPost(r.Context(), token, p.ID, p.Author.ID, p.Price)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	shown, err := a.withPeople(r, []model.Post{p})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if len(shown) == 0 {
		a.writeError(w, r, failure.New(failure.NotFound, "post not found"))
		return
	}

	writeJSON(w, http.StatusOK, purchaseResponse{Balance: balance, Post: shown[0]})
}
