package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Federation opens pages to the Fediverse (ActivityPub) and serves what
// other servers read of them; the documents are passed on as they are.
type Federation interface {
	Federation(ctx context.Context, accessToken string) (model.Federation, error)
	Open(ctx context.Context, accessToken string) (model.Federation, error)
	Close(ctx context.Context, accessToken string) (model.Federation, error)

	Finger(ctx context.Context, resource string) ([]byte, error)
	Actor(ctx context.Context, userID string) (model.FederationDocument, error)
	Note(ctx context.Context, postID string) (model.FederationDocument, error)
	Followers(ctx context.Context, userID string) ([]byte, error)
	Outbox(ctx context.Context, userID string) ([]byte, error)
	Receive(ctx context.Context, in model.InboxRequest) error

	// The news from other servers: the accounts the site follows and
	// shows posts of.
	SiteActor(ctx context.Context) ([]byte, error)
	Lookup(ctx context.Context, accessToken, address string) (model.FediverseLookup, error)
	Subscriptions(ctx context.Context, accessToken string) ([]model.FediverseAccount, error)
	Subscribe(ctx context.Context, accessToken, address string) (model.FediverseAccount, error)
	Unsubscribe(ctx context.Context, accessToken, id string) error
	News(ctx context.Context, accessToken string, q model.FediverseNews) ([]model.FediversePost, error)
	// Internal: the gateway lets only administrators change these.
	Featured(ctx context.Context) ([]model.FediverseAccount, error)
	Feature(ctx context.Context, address string) (model.FediverseAccount, error)
	Unfeature(ctx context.Context, id string) error
	PostPicture(ctx context.Context, postID string, index int) (model.Picture, error)
	VideoCover(ctx context.Context, postID string, index int) (model.Picture, error)
	AccountPicture(ctx context.Context, accountID string) (model.Picture, error)
}

const (
	activityType = "application/activity+json"
	jrdType      = "application/jrd+json"
	// maxActivityBytes is far above any activity a server sends to an
	// inbox: they carry a note, not its photos.
	maxActivityBytes = 256 << 10
)

// federationJSON: people of other servers find an open page as
// @nickname@domain.
type federationJSON struct {
	Open      bool   `json:"open"`
	Followers int    `json:"followers"`
	Domain    string `json:"domain"`
}

func (a *api) registerFederation(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/federation", a.federationOf(Federation.Federation))
	mux.HandleFunc("PUT /api/v1/me/federation", a.federationOf(Federation.Open))
	mux.HandleFunc("DELETE /api/v1/me/federation", a.federationOf(Federation.Close))

	mux.HandleFunc("GET /.well-known/webfinger", a.webfinger)
	mux.HandleFunc("GET /ap/users/{id}", a.federationPage(Federation.Actor))
	mux.HandleFunc("GET /ap/posts/{id}", a.federationPage(Federation.Note))
	mux.HandleFunc("GET /ap/users/{id}/followers", a.federationDocument(Federation.Followers))
	mux.HandleFunc("GET /ap/users/{id}/outbox", a.federationDocument(Federation.Outbox))
	mux.HandleFunc("POST /ap/inbox", a.inbox)
	mux.HandleFunc("POST /ap/users/{id}/inbox", a.inbox)

	a.registerFediverse(mux)
}

func (a *api) federationOf(do func(Federation, context.Context, string) (model.Federation, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerToken(r)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		f, err := do(a.federation, r.Context(), token)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, federationJSON{Open: f.Open, Followers: f.Followers, Domain: f.Domain})
	}
}

func (a *api) webfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "resource is required"))
		return
	}

	doc, err := a.federation.Finger(r.Context(), resource)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeDocument(w, jrdType, doc)
}

// federationPage serves a document that is also a page: a browser that
// follows the address of a person or a note from another server is sent
// to the page itself.
func (a *api) federationPage(read func(Federation, context.Context, string) (model.FederationDocument, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doc, err := read(a.federation, r.Context(), r.PathValue("id"))
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		if wantsHTML(r) && doc.PageURL != "" {
			http.Redirect(w, r, doc.PageURL, http.StatusFound)
			return
		}

		writeDocument(w, activityType, doc.Body)
	}
}

func (a *api) federationDocument(read func(Federation, context.Context, string) ([]byte, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doc, err := read(a.federation, r.Context(), r.PathValue("id"))
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		writeDocument(w, activityType, doc)
	}
}

// inbox passes what another server sent on as it came: federation checks
// the signature, which covers the path and the headers, Host among them.
func (a *api) inbox(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxActivityBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "request body is too large"))
			return
		}

		a.writeError(w, r, failure.New(failure.InvalidArgument, "request body is unreadable"))
		return
	}

	headers := make(map[string]string, len(r.Header)+1)
	for name, values := range r.Header {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	headers["host"] = r.Host

	err = a.federation.Receive(r.Context(), model.InboxRequest{
		Method:  r.Method,
		Path:    r.URL.RequestURI(),
		Headers: headers,
		Body:    body,
		UserID:  r.PathValue("id"),
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// wantsHTML: a browser asks for a page first; servers ask for
// ActivityPub JSON.
func wantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "activity+json") || strings.Contains(accept, "ld+json") {
		return false
	}

	return strings.Contains(accept, "text/html")
}

func writeDocument(w http.ResponseWriter, contentType string, doc []byte) {
	w.Header().Set("Content-Type", contentType)
	// Servers read these documents from their own pages, e.g. the search
	// of Mastodon's web client.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(doc)
}
