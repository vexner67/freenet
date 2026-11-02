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

// FollowingRepository keeps the accounts of other servers the site
// follows; Get of one it does not is fedi.ErrNotFound.
type FollowingRepository interface {
	Get(ctx context.Context, actor fedi.URL) (*fedi.Following, error)
	Save(ctx context.Context, f *fedi.Following) error
	Delete(ctx context.Context, actor fedi.URL) error
	// Featured are the accounts the administrators chose for the news of
	// everyone, the earliest chosen first.
	Featured(ctx context.Context) ([]*fedi.Following, error)
	// Channels are the channels of Telegram the site reads.
	Channels(ctx context.Context) ([]fedi.URL, error)
}

// SubscriptionRepository keeps whom of other servers the people here
// read. Add of one already there is fine, as is Remove of one gone.
type SubscriptionRepository interface {
	Add(ctx context.Context, userID uuid.UUID, actor fedi.URL, now time.Time) error
	Remove(ctx context.Context, userID uuid.UUID, actor fedi.URL) error
	// RemoveActor forgets an account deleted on its server.
	RemoveActor(ctx context.Context, actor fedi.URL) error
	// Of are whom userID reads, the latest first.
	Of(ctx context.Context, userID uuid.UUID) ([]fedi.URL, error)
	// Read: somebody here reads actor.
	Read(ctx context.Context, actor fedi.URL) (bool, error)
}

// NewsQuery is which remote posts to read: those of the featured accounts
// in Languages, or those of the accounts Reader subscribed to; with
// Before, older than it.
type NewsQuery struct {
	Featured      bool
	Reader        uuid.UUID
	Languages     []string
	HideSensitive bool
	Before        uuid.UUID
	Limit         int
}

// RemotePostRepository keeps the notes of the accounts the site follows,
// newest first; Get and ByURI of one it does not keep are
// fedi.ErrNotFound. Save keeps a note anew or as edited, by its URI.
type RemotePostRepository interface {
	Get(ctx context.Context, id uuid.UUID) (*fedi.RemotePost, error)
	ByURI(ctx context.Context, uri fedi.URL) (*fedi.RemotePost, error)
	Save(ctx context.Context, p *fedi.RemotePost) error
	Delete(ctx context.Context, uri fedi.URL) error
	DeleteActor(ctx context.Context, actor fedi.URL) error
	List(ctx context.Context, q NewsQuery) ([]*fedi.RemotePost, error)
	// URIsOf are the notes kept of actor.
	URIsOf(ctx context.Context, actor fedi.URL) ([]fedi.URL, error)
	// DeleteBefore forgets the notes published before t, telling how many.
	DeleteBefore(ctx context.Context, t time.Time) (int, error)
}

// Finder finds the actor of an address on another server by WebFinger:
// @username@host.
type Finder interface {
	Finger(ctx context.Context, username, host string) (fedi.URL, error)
}

// Media is a picture of another server, read for the site to show.
type Media struct {
	Body        []byte
	ContentType string
}

// MediaFetcher reads a picture of another server; one that is not a
// picture, or too big, is fedi.ErrNotFound.
type MediaFetcher interface {
	Fetch(ctx context.Context, u fedi.URL) (Media, error)
}

// CoverRepository keeps the first frames of the videos of remote posts,
// as JPEG: an empty one for a video with no frame to cut. Get of one not
// kept is fedi.ErrNotFound; the frames go with their post.
type CoverRepository interface {
	Get(ctx context.Context, postID uuid.UUID, video fedi.URL) ([]byte, error)
	Save(ctx context.Context, postID uuid.UUID, video fedi.URL, frame []byte) error
}

// Framer cuts the first frame of a video of another server, as JPEG; a
// video it finds no frame in is fedi.ErrNotFound.
type Framer interface {
	Frame(ctx context.Context, video fedi.URL) ([]byte, error)
}

// Channels read the pages of the public channels of Telegram; a name of
// no such channel is fedi.ErrNotFound.
type Channels interface {
	Channel(ctx context.Context, name string) (fedi.ChannelDoc, error)
}

// Liker is a like of a remote post: its ID, a UUIDv7, is the cursor of
// the likers.
type Liker struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// Likes are how many like a remote post, and whether the reader does.
type Likes struct {
	Count int
	Liked bool
}

// LikeRepository keeps who here like remote posts; the likes go with
// their post. Add of a like already there is fine, as is Remove of one
// gone.
type LikeRepository interface {
	Add(ctx context.Context, postID uuid.UUID, l Liker, now time.Time) error
	Remove(ctx context.Context, postID, userID uuid.UUID) error
	// Of are the likes of each of posts, those liked by none left out.
	Of(ctx context.Context, postIDs []uuid.UUID, reader uuid.UUID) (map[uuid.UUID]Likes, error)
	// Likers are who like a post, the latest first; with before, those
	// who liked it before that like.
	Likers(ctx context.Context, postID, before uuid.UUID, limit int) ([]Liker, error)
}
