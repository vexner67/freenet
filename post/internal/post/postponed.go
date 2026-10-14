package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxPostponedAhead is how far ahead a post may be postponed: a year,
	// as on VK.
	MaxPostponedAhead = 365 * 24 * time.Hour
	// MaxPostponed is how many posts wait for one wall at once.
	MaxPostponed = 100
)

var (
	ErrPostponedNotFound  = errors.New("postponed post not found")
	ErrPublishTime        = errors.New("a postponed post is published within a year from now")
	ErrPostponedElsewhere = errors.New("a post is postponed for your own wall or for a community's")
	ErrPostponedContent   = errors.New("a postponed post says words and shows photos only")
	ErrCannotSeePostponed = errors.New("only the author or the admins see the postponed posts")
	ErrCannotPostpone     = errors.New("only the author or the admins can change the postponed post")
	ErrTooManyPostponed   = errors.New("too many postponed posts")
)

// Postponed is a post that waits for its time, VK's "отложенная запись"
// (openvk has none): nobody but its author and, on a community's wall,
// the community's admins sees it until it is published, at PublishAt or
// when they publish it at once. It goes on its author's own wall, or
// speaks for a community; its content is text, photos or both, never a
// repost or a poll.
type Postponed struct {
	id        uuid.UUID
	wallID    uuid.UUID
	authorID  uuid.UUID
	content   Content
	fromWall  bool
	signed    bool
	nsfw      bool
	mentions  Mentions
	publishAt time.Time
	createdAt time.Time
}

// PostponedDraft is a post to postpone, what CreateInput is to a post:
// FromWall puts it on the community's wall WallID, in its name; without
// it WallID is the author's own.
type PostponedDraft struct {
	WallID    uuid.UUID
	AuthorID  uuid.UUID
	Content   Content
	FromWall  bool
	Signed    bool
	NSFW      bool
	Mentions  Mentions
	PublishAt time.Time
}

// NewPostponed postpones d at now. Whether the author may speak for the
// community is the club service's, which the caller asks.
func NewPostponed(d PostponedDraft, now time.Time) (*Postponed, error) {
	if d.Content.RepostOf() != uuid.Nil || d.Content.SharedPhoto() != uuid.Nil || d.Content.Polled() {
		return nil, ErrPostponedContent
	}

	if d.FromWall == (d.WallID == d.AuthorID) || (d.Signed && !d.FromWall) {
		return nil, ErrPostponedElsewhere
	}

	publishAt, err := publishTime(d.PublishAt, now)
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Postponed{
		id:        id,
		wallID:    d.WallID,
		authorID:  d.AuthorID,
		content:   d.Content,
		fromWall:  d.FromWall,
		signed:    d.Signed,
		nsfw:      d.NSFW,
		mentions:  d.Mentions,
		publishAt: publishAt,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// publishTime checks that at is ahead of now, within MaxPostponedAhead.
func publishTime(at, now time.Time) (time.Time, error) {
	if !at.After(now) || at.After(now.Add(MaxPostponedAhead)) {
		return time.Time{}, ErrPublishTime
	}

	return at.UTC().Truncate(time.Microsecond), nil
}

// StoredPostponed is a postponed post as storage keeps it.
type StoredPostponed struct {
	ID        uuid.UUID
	WallID    uuid.UUID
	AuthorID  uuid.UUID
	Content   Content
	FromWall  bool
	Signed    bool
	NSFW      bool
	Mentions  Mentions
	PublishAt time.Time
	CreatedAt time.Time
}

// RestorePostponed rebuilds a postponed post from storage without
// validation or new IDs.
func RestorePostponed(s StoredPostponed) *Postponed {
	return &Postponed{
		id: s.ID, wallID: s.WallID, authorID: s.AuthorID, content: s.Content, fromWall: s.FromWall, signed: s.Signed,
		nsfw: s.NSFW, mentions: s.Mentions, publishAt: s.PublishAt, createdAt: s.CreatedAt,
	}
}

// PostponedEdit is what an edit of a postponed post sets: its words, with
// the people they name, whether it is adult and when it comes out. The
// photos stay.
type PostponedEdit struct {
	Text      Text
	Mentions  Mentions
	NSFW      bool
	PublishAt time.Time
}

// Edited is p as edit changes it at now; p itself is left as it was.
func (p *Postponed) Edited(edit PostponedEdit, now time.Time) (*Postponed, error) {
	content, err := NewContent(edit.Text, p.content.Photos())
	if err != nil {
		return nil, err
	}

	publishAt, err := publishTime(edit.PublishAt, now)
	if err != nil {
		return nil, err
	}

	edited := *p
	edited.content, edited.mentions, edited.nsfw, edited.publishAt = content, edit.Mentions, edit.NSFW, publishAt

	return &edited, nil
}

// Published is the post p becomes at now: a new one, dated now, so it
// shows on top of the wall, as when it was written right then.
func (p *Postponed) Published(now time.Time) (*Post, error) {
	var (
		published *Post
		err       error
	)
	if p.fromWall {
		published, err = NewFromWall(p.wallID, p.authorID, p.content, p.signed, now)
	} else {
		published, err = New(p.wallID, p.authorID, p.content, now)
	}
	if err != nil {
		return nil, err
	}

	return published.Marked(p.nsfw), nil
}

// ChangedBy tells whether userID may change p by themselves: the author
// of a post for their own wall. One that speaks for a community is the
// admins', whom the caller asks.
func (p *Postponed) ChangedBy(userID uuid.UUID) bool {
	return !p.fromWall && userID == p.authorID
}

// CanBeDeletedBy: the author takes back what they postponed, whatever
// wall it is for; the admins, whom the caller asks, delete the posts
// postponed for their community.
func (p *Postponed) CanBeDeletedBy(userID uuid.UUID) bool {
	return userID == p.authorID
}

func (p *Postponed) ID() uuid.UUID {
	return p.id
}

func (p *Postponed) WallID() uuid.UUID {
	return p.wallID
}

func (p *Postponed) AuthorID() uuid.UUID {
	return p.authorID
}

func (p *Postponed) Content() Content {
	return p.content
}

func (p *Postponed) FromWall() bool {
	return p.fromWall
}

func (p *Postponed) Signed() bool {
	return p.signed
}

func (p *Postponed) NSFW() bool {
	return p.nsfw
}

func (p *Postponed) Mentions() Mentions {
	return p.mentions
}

func (p *Postponed) PublishAt() time.Time {
	return p.publishAt
}

func (p *Postponed) CreatedAt() time.Time {
	return p.createdAt
}
