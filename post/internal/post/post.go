// Package post is the domain of wall posts.
package post

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxTextLen is openvk's (wall.postSizes.maxSize), for posts and
	// comments alike.
	maxTextLen = 60000

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
	ErrRepostPhotos = errors.New("a post sharing a photo has no photos of its own")
	ErrCannotPin    = errors.New("only the owner of the wall can pin the post")
	ErrFromOwnWall  = errors.New("a post on your own wall speaks for you")
	// ErrAvatarUpdate: the post of a new photo is that photo and nothing
	// else.
	ErrAvatarUpdate = errors.New("a new photo is posted alone")
	// ErrCannotArchive and ErrCannotCloseComments: the owner of the wall,
	// or the admins of the community it is, keep its archive and say
	// which posts take comments.
	ErrCannotArchive       = errors.New("only the owner of the wall keeps its archive")
	ErrCannotCloseComments = errors.New("only the owner of the wall can close the comments")
	ErrCommentsClosed      = errors.New("the comments are closed")
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

// Excerpt is the beginning of the text, at most limit characters, and
// whether it was cut: lists carry it instead of a text of thousands of
// characters. It ends after a whole word and not inside an address, when
// it can. Without a limit the text is whole.
func (t Text) Excerpt(limit int) (string, bool) {
	if limit <= 0 || len(t) <= limit {
		return string(t), false
	}

	runes := []rune(string(t))
	if len(runes) <= limit {
		return string(t), false
	}

	return string(runes[:wholeEnd(runes, limit, 0)]), true
}

// excerptSlack is how far an excerpt gives way to end well: back to the
// space before the word it would cut, or to the start of an address
// ("[ann|Анна]") it would end inside. A longer word is cut where it is.
const excerptSlack = 100

// wholeEnd is where an excerpt of runes, at most limit of them and fewer
// than there are, ends: after a whole word and outside an address, when
// one is near enough, and never before keep.
func wholeEnd(runes []rune, limit, keep int) int {
	end := limit
	floor := max(limit-excerptSlack, keep)

	if !unicode.IsSpace(runes[end]) {
		for end > floor && !unicode.IsSpace(runes[end-1]) {
			end--
		}

		if end == 0 || !unicode.IsSpace(runes[end-1]) {
			end = limit
		}
	}

	if open := addressStart(runes, end); open >= keep {
		end = open
	}

	for end > keep && unicode.IsSpace(runes[end-1]) {
		end--
	}

	if end == 0 {
		return limit
	}

	return end
}

// addressStart is where the address that end falls inside of begins: its
// bracket was opened shortly before and is closed shortly after, on the
// same line. It is -1 when end is inside none.
func addressStart(runes []rune, end int) int {
	open := -1

	for i := end - 1; i >= max(end-excerptSlack, 0) && open < 0; i-- {
		switch runes[i] {
		case ']', '\n':
			return -1
		case '[':
			open = i
		}
	}

	if open < 0 {
		return -1
	}

	for i := end; i < min(end+excerptSlack, len(runes)); i++ {
		switch runes[i] {
		case ']':
			return open
		case '[', '\n':
			return -1
		}
	}

	return -1
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
	// archived: the owner of the wall put the post away ("Архив записей"
	// of openvk); only they see it, and no wall or feed shows it.
	archived bool
	// commentsClosed: the owner of the wall lets nobody comment it anymore.
	commentsClosed bool
	// nsfw: the author marked the post as adult ("Содержит NSFW-контент"
	// of openvk); readers see it blurred until they ask.
	nsfw bool
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
	if content.text != "" || len(content.photos) != 1 || content.repostOf != uuid.Nil || content.polled {
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
	Archived     bool
	// CommentsClosed: nobody comments the post anymore.
	CommentsClosed bool
	NSFW           bool
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

		avatarUpdate:   s.AvatarUpdate,
		archived:       s.Archived,
		commentsClosed: s.CommentsClosed,
		nsfw:           s.NSFW,
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

		AvatarUpdate:   p.avatarUpdate,
		Archived:       p.archived,
		CommentsClosed: p.commentsClosed,
		NSFW:           p.nsfw,
	}
}

// Edited returns p saying text with photos instead, edited by requesterID
// at now. Only the author edits, and for editWindow after posting, as in
// openvk. A repost keeps the post it shares; a post that shares a photo of
// an album keeps it and has no photos of its own; a post that asks a poll
// asks it still. The post of a new photo is not edited at all.
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

	switch {
	case p.content.repostOf != uuid.Nil:
		content, err = NewRepost(p.content.repostOf, text, photos)
	case p.content.sharedPhoto != uuid.Nil:
		// It shares the photo still: the words are what changes.
		if len(photos) > 0 {
			return nil, ErrRepostPhotos
		}
		content, err = NewPhotoShare(p.content.sharedPhoto, text)
	case p.content.polled:
		content = NewPolled(text, photos)
	default:
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

// ListedFor tells whether the post is among those userID wrote ("Написанные
// мной записи"): theirs, on whichever wall, but for what was archived on
// the wall of another — only its owner reads that.
func (p *Post) ListedFor(userID uuid.UUID) bool {
	return p.BelongsTo(userID) && (!p.archived || p.wallID == userID)
}

// CanBePinnedBy: the owner of the wall chooses what it shows first,
// whoever wrote it.
func (p *Post) CanBePinnedBy(userID uuid.UUID) bool {
	return userID == p.wallID
}

// OnWallOf tells whether userID owns the wall the post is on: they keep
// its archive and close its comments, whoever wrote the post. For a
// community the caller asks who runs it.
func (p *Post) OnWallOf(userID uuid.UUID) bool {
	return userID == p.wallID
}

// Archived: the post is put away in the archive of its wall.
func (p *Post) Archived() bool {
	return p.archived
}

// NSFW: the author marked the post as adult.
func (p *Post) NSFW() bool {
	return p.nsfw
}

// Marked is p marked as adult or not, as its author says when writing or
// editing it. Like Edited, it returns a new post.
func (p *Post) Marked(nsfw bool) *Post {
	marked := *p
	marked.nsfw = nsfw

	return &marked
}

// CommentsClosed: nobody comments the post anymore.
func (p *Post) CommentsClosed() bool {
	return p.commentsClosed
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

// InWallAlbum tells whether the photos of the post go to its author's
// "Фотографии со стены", as openvk keeps what a person uploads writing on
// their own wall. A new photo of the page is in "Фотографии со страницы".
func (p *Post) InWallAlbum() bool {
	return p.OnOwnWall() && !p.avatarUpdate
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
