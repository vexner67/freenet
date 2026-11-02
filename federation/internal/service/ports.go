// Package service holds the use cases of federation and the ports they
// need: storage, the user and post services, the network.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
)

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// AccountRepository keeps the pages opened to the Fediverse; Get of a
// page never opened is fedi.ErrNotFound.
type AccountRepository interface {
	Get(ctx context.Context, userID uuid.UUID) (*fedi.Account, error)
	Save(ctx context.Context, a *fedi.Account) error
}

// FollowerRepository keeps who of other servers follows whom here. Add
// of a follower already there is fine, as is Remove of one gone.
type FollowerRepository interface {
	Add(ctx context.Context, f *fedi.Follower) error
	Remove(ctx context.Context, userID uuid.UUID, actor fedi.URL) error
	// RemoveActor forgets an actor who is gone, wherever it followed.
	RemoveActor(ctx context.Context, actor fedi.URL) error
	// RemoveAll forgets the followers of a page closed to the Fediverse.
	RemoveAll(ctx context.Context, userID uuid.UUID) error
	// Inboxes are where the followers of a page get its posts, each once.
	Inboxes(ctx context.Context, userID uuid.UUID) ([]fedi.URL, error)
	Count(ctx context.Context, userID uuid.UUID) (int, error)
}

// RemoteRepository keeps the documents of remote actors read lately.
type RemoteRepository interface {
	Get(ctx context.Context, id fedi.URL) (*fedi.Remote, error)
	Save(ctx context.Context, r *fedi.Remote) error
	Delete(ctx context.Context, id fedi.URL) error
}

// Person is a page of the site as a guest sees it.
type Person struct {
	ID       uuid.UUID
	Nickname string
	Name     string
	Status   string
	AvatarID string
	// Hidden: a guest may not see the page, or it is deleted.
	Hidden       bool
	RegisteredAt time.Time
}

// People are the pages of the user service; an unknown one is
// fedi.ErrNotFound.
type People interface {
	ByID(ctx context.Context, id uuid.UUID) (Person, error)
	ByNickname(ctx context.Context, nickname string) (Person, error)
}

// Posts are the posts of the post service as a guest sees them; one a
// guest may not see, or none, is fedi.ErrNotFound.
type Posts interface {
	Post(ctx context.Context, id uuid.UUID) (fedi.Post, error)
}

// Resolver reads the document of a remote actor from its server, signing
// the request as signer when there is one (some servers want it). An
// actor deleted for good is fedi.ErrGone.
type Resolver interface {
	Actor(ctx context.Context, id fedi.URL, signer *fedi.Account) (fedi.RemoteDoc, error)
}

// SignedRequest is a request that came to an inbox: what its signature
// covers. Header names are in lower case.
type SignedRequest struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    []byte
}

// Signatures check the HTTP signatures of requests.
type Signatures interface {
	// KeyID is the key the request says it is signed with.
	KeyID(r SignedRequest) (fedi.URL, error)
	// Verify checks the signature with the public key (PEM), and that the
	// request is fresh and its body the one signed: fedi.ErrBadSignature.
	Verify(r SignedRequest, publicKey string, now time.Time) error
}

// Sender sends an activity of a page here to an inbox, signed with the
// page's key, later and until it is taken: in the transaction of ctx.
type Sender interface {
	Send(ctx context.Context, inbox fedi.URL, signerID uuid.UUID, a fedi.Outgoing) error
}

// KeyMaker makes the key pair of a page opened to the Fediverse.
type KeyMaker interface {
	NewKeys() (fedi.KeyPair, error)
}

// Limiter counts what a remote server sends: false once it sent limit
// within period.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, period time.Duration) (bool, error)
}
