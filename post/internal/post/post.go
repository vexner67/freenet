// Package post is the domain of wall posts.
package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxTextLen = 1000

	// editWindow is how long a post can be edited, as in openvk: long
	// enough to fix a typo, short enough that an old post is not rewritten
	// under its likes and comments.
	editWindow = 7 * 24 * time.Hour
)

var (
	ErrTextEmpty    = errors.New("post text is empty")
	ErrTextTooLong  = errors.New("post text is too long")
	ErrNotFound     = errors.New("post not found")
	ErrCannotDelete = errors.New("only the author or the owner of the wall can delete the post")
	ErrCannotEdit   = errors.New("only the author can edit the post")
	ErrEditExpired  = errors.New("the post can no longer be edited")
	ErrRepostPhotos = errors.New("a repost has no photos of its own")
	ErrCannotPin    = errors.New("only the owner of the wall can pin the post")
	ErrFromOwnWall  = errors.New("a post on your own wall speaks for you")
	// ErrAvatarUpdate: the post of a new photo is that photo and nothing
	// else.
	ErrAvatarUpdate = errors.New("a new photo is posted alone")
)

// Text is what a post says: trimmed, with "\n" line breaks. It may be
// empty when the post has photos (see NewContent).
type Text string

func NewText(s string) (Text, error) {
	s, err := cleanText(s, maxTextLen, nil, ErrTextTooLong)
	if err != nil {
		return "", err
	}

	return Text(s), nil
}

// cleanText trims s and normalises its line breaks, so length and storage
// do not depend on the client. A nil errEmpty allows empty text.
func cleanText(s string, maxLen int, errEmpty, errTooLong error) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if s == "" && errEmpty != nil {
		return "", errEmpty
	}

	if utf8.RuneCountInString(s) > maxLen {
		return "", errTooLong
	}

	return s, nil
}

func (t Text) String() string {
	return string(t)
}

// Counters are how many likes, comments and reposts a post has. Storage
// keeps them with the post, so a wall is read without counting.
type Counters struct {
	Likes    int
	Comments int
	Reposts  int
}

// Post is written by its author on a wall; the wall is that of its owner,
// the author's own or someone else's. A post does not change once made:
// Edited returns a new one, so readers may keep the pointers they got.
type Post struct {
	id        uuid.UUID
	wallID    uuid.UUID
	authorID  uuid.UUID
	content   Content
	createdAt time.Time
	// editedAt is zero for a post never edited.
	editedAt time.Time
	counters Counters
	// pinned: the owner of the wall shows the post first on it.
	pinned bool
	// fromWall: the post speaks for its wall, a community, rather than
	// for its author, who is not shown ("От имени сообщества" of openvk)
	// unless signed ("Подпись автора").
	fromWall bool
	signed   bool
	// suggested: the post was suggested for a community's wall and
	// published by its admins; it is the community's now, and its author
	// neither edits nor deletes it, as openvk's "Вы не можете удалить ваш
	// принятый пост".
	suggested bool
	// avatarUpdate: the post shows the new photo of the wall's owner, a
	// user or a community ("обновил фотографию на своей странице" of
	// openvk); nobody edits it.
	avatarUpdate bool
}

// New creates a post by authorID on the wall of wallID. Its UUIDv7 ID
// grows with time, so walls are ordered and paged by ID. Whether the author
// may write on that wall is the wall owner's privacy, which the caller checks.
func New(wallID, authorID uuid.UUID, content Content, now time.Time) (*Post, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Post{
		id:       id,
		wallID:   wallID,
		authorID: authorID,
		content:  content,
		// PostgreSQL keeps microseconds; trim now so a post reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// NewFromWall creates a post by authorID that speaks for the wall of
// wallID, a community: its readers see the community and, if signed, the
// author's name under it ("Автор: ..."). Whether the author may speak for
// it is the community's, which the caller checks.
func NewFromWall(wallID, authorID uuid.UUID, content Content, signed bool, now time.Time) (*Post, error) {
	if wallID == authorID {
		return nil, ErrFromOwnWall
	}

	p, err := New(wallID, authorID, content, now)
	if err != nil {
		return nil, err
	}

	p.fromWall = true
	p.signed = signed

	return p, nil
}

// NewAvatarUpdate creates the post of the new photo of the wall of wallID,
// as openvk does when it is uploaded with "Опубликовать запись на стене":
// content is that one photo. On a user's own wall it is theirs; for a
// community, which the caller has checked authorID runs, it speaks for it.
func NewAvatarUpdate(wallID, authorID uuid.UUID, content Content, now time.Time) (*Post, error) {
	if content.text != "" || len(content.photos) != 1 || content.repostOf != uuid.Nil {
		return nil, ErrAvatarUpdate
	}

	p, err := New(wallID, authorID, content, now)
	if err != nil {
		return nil, err
	}

	p.fromWall = wallID != authorID
	p.avatarUpdate = true

	return p, nil
}

// Stored is a post as storage keeps it.
type Stored struct {
	ID           uuid.UUID
	WallID       uuid.UUID
	AuthorID     uuid.UUID
	Content      Content
	CreatedAt    time.Time
	EditedAt     time.Time
	Counters     Counters
	Pinned       bool
	FromWall     bool
	Signed       bool
	Suggested    bool
	AvatarUpdate bool
}

// Restore rebuilds a post from storage without validation or new IDs.
func Restore(s Stored) *Post {
	return &Post{
		id:        s.ID,
		wallID:    s.WallID,
		authorID:  s.AuthorID,
		content:   s.Content,
		createdAt: s.CreatedAt,
		editedAt:  s.EditedAt,
		counters:  s.Counters,
		pinned:    s.Pinned,
		fromWall:  s.FromWall,
		signed:    s.Signed,
		suggested: s.Suggested,

		avatarUpdate: s.AvatarUpdate,
	}
}

// Stored is what storage keeps of p.
func (p *Post) Stored() Stored {
	return Stored{
		ID:        p.id,
		WallID:    p.wallID,
		AuthorID:  p.authorID,
		Content:   p.content,
		CreatedAt: p.createdAt,
		EditedAt:  p.editedAt,
		Counters:  p.counters,
		Pinned:    p.pinned,
		FromWall:  p.fromWall,
		Signed:    p.signed,
		Suggested: p.suggested,

		AvatarUpdate: p.avatarUpdate,
	}
}

// Edited returns p saying text with photos instead, edited by requesterID
// at now. Only the author edits, and for editWindow after posting, as in
// openvk. A repost keeps the post it shares and, as when it was made, has
// no photos of its own. The post of a new photo is not edited at all.
func (p *Post) Edited(requesterID uuid.UUID, text Text, photos Photos, now time.Time) (*Post, error) {
	if !p.BelongsTo(requesterID) || p.avatarUpdate {
		return nil, ErrCannotEdit
	}

	if now.After(p.EditableUntil()) {
		return nil, ErrEditExpired
	}

	var (
		content Content
		err     error
	)

	if p.content.repostOf != uuid.Nil {
		if len(photos) > 0 {
			return nil, ErrRepostPhotos
		}
		content, err = NewRepost(p.content.repostOf, text)
	} else {
		content, err = NewContent(text, photos)
	}
	if err != nil {
		return nil, err
	}

	edited := *p
	edited.content = content
	edited.editedAt = now.UTC().Truncate(time.Microsecond)

	return &edited, nil
}

// CanBeDeletedBy: a post is removed by whoever wrote it or by the owner of
// the wall, who keeps the wall clean.
func (p *Post) CanBeDeletedBy(userID uuid.UUID) bool {
	return p.BelongsTo(userID) || userID == p.wallID
}

// BelongsTo tells whether userID wrote the post and still has it: the
// author of a post suggested and published for a community no longer
// does.
func (p *Post) BelongsTo(userID uuid.UUID) bool {
	return userID == p.authorID && !p.suggested
}

// CanBePinnedBy: the owner of the wall chooses what it shows first,
// whoever wrote it.
func (p *Post) CanBePinnedBy(userID uuid.UUID) bool {
	return userID == p.wallID
}

// Shared is what reposting p shares: p itself, or, when p only passes on
// another post without a word, that post — a chain of empty reposts
// would show nothing but the post at its end.
func (p *Post) Shared() uuid.UUID {
	if p.content.passesOn() {
		return p.content.repostOf
	}

	return p.id
}

// Notified is who hears of the likes, comments and reposts of p: its
// author, or nobody (uuid.Nil) when it speaks for a community, which has
// no notifications and whose author is not to be told apart.
func (p *Post) Notified() uuid.UUID {
	if p.fromWall {
		return uuid.Nil
	}

	return p.authorID
}

// OnOwnWall tells whether the author wrote on their own wall.
func (p *Post) OnOwnWall() bool {
	return p.wallID == p.authorID
}

func (p *Post) ID() uuid.UUID {
	return p.id
}

func (p *Post) WallID() uuid.UUID {
	return p.wallID
}

func (p *Post) AuthorID() uuid.UUID {
	return p.authorID
}

func (p *Post) Content() Content {
	return p.content
}

func (p *Post) CreatedAt() time.Time {
	return p.createdAt
}

// EditableUntil is when the author can no longer edit the post: when it
// was made for the post of a new photo, which is never edited.
func (p *Post) EditableUntil() time.Time {
	if p.avatarUpdate {
		return p.createdAt
	}

	return p.createdAt.Add(editWindow)
}

// EditedAt is zero for a post never edited.
func (p *Post) EditedAt() time.Time {
	return p.editedAt
}

func (p *Post) Counters() Counters {
	return p.counters
}

func (p *Post) Pinned() bool {
	return p.pinned
}

func (p *Post) FromWall() bool {
	return p.fromWall
}

// Signed: a post that speaks for a community names its author.
func (p *Post) Signed() bool {
	return p.signed
}

func (p *Post) Suggested() bool {
	return p.suggested
}

// AvatarUpdate: the post shows the new photo of its wall's owner.
func (p *Post) AvatarUpdate() bool {
	return p.avatarUpdate
}
