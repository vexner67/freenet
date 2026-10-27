package post

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxArticleTitle = 256
	// maxArticleText is how many characters the text of an article holds
	// in all: longer than a post, as an article is what a long read is.
	maxArticleText   = 100_000
	maxArticleBlocks = 500
	maxBlockSpans    = 300
	// maxArticlePhotos counts the cover and the photos of the text.
	maxArticlePhotos  = 30
	maxArticleCaption = 256
	maxArticleLink    = 2048
	// articleWordsPerMinute is how fast an article is read.
	articleWordsPerMinute = 200
)

var (
	ErrArticleNotFound     = errors.New("article not found")
	ErrArticleTitleEmpty   = errors.New("article title is empty")
	ErrArticleTitleTooLong = errors.New("article title is too long")
	ErrArticleEmpty        = errors.New("article has no text")
	ErrArticleTooLong      = errors.New("article is too long")
	ErrArticleBlock        = errors.New("invalid article block")
	ErrArticleLink         = errors.New("invalid link in the article")
	ErrArticlePhotos       = errors.New("an article has at most 30 photos, each once")
	ErrCannotChangeArticle = errors.New("only the author can change the article")
)

// BlockKind is what a block of an article is: a paragraph, a heading, an
// item of a list, a photo and so on.
type BlockKind string

const (
	BlockParagraph  BlockKind = "paragraph"
	BlockHeading    BlockKind = "heading"
	BlockSubheading BlockKind = "subheading"
	BlockQuote      BlockKind = "quote"
	// BlockBullet and BlockNumber are items of a list; the items next to
	// each other make one list.
	BlockBullet  BlockKind = "bullet"
	BlockNumber  BlockKind = "number"
	BlockCode    BlockKind = "code"
	BlockPhoto   BlockKind = "photo"
	BlockDivider BlockKind = "divider"
)

// hasText tells the kinds made of spans of text.
func (k BlockKind) hasText() bool {
	switch k {
	case BlockParagraph, BlockHeading, BlockSubheading, BlockQuote, BlockBullet, BlockNumber, BlockCode:
		return true
	default:
		return false
	}
}

// Span is a piece of the text of a block written one way: bold, italic,
// struck out, a link (an http or https address) or plain.
type Span struct {
	Text   string
	Bold   bool
	Italic bool
	Strike bool
	Link   string
}

func (s Span) sameMarks(o Span) bool {
	return s.Bold == o.Bold && s.Italic == o.Italic && s.Strike == o.Strike && s.Link == o.Link
}

func (s Span) marked() bool {
	return s.Bold || s.Italic || s.Strike || s.Link != ""
}

// Block is a block of an article: the spans of a text kind, the photo
// (a photo of the author in media) and its caption, or a divider.
type Block struct {
	Kind    BlockKind
	Spans   []Span
	PhotoID uuid.UUID
	Caption string
}

// ArticleDraft is what an article is written of.
type ArticleDraft struct {
	Title string
	// CoverID is the photo over the title, uuid.Nil for none.
	CoverID uuid.UUID
	Blocks  []Block
}

// Article is a long read with headings, quotes, lists and photos, as
// "Статьи" of VK: written in the editor, read on its own page and shown
// in posts that link to it. Anybody with the link reads it; only its
// author changes it.
type Article struct {
	id        uuid.UUID
	authorID  uuid.UUID
	title     string
	coverID   uuid.UUID
	blocks    []Block
	createdAt time.Time
	editedAt  *time.Time
}

// NewArticle checks a draft of authorID: a title of one line, some text
// within the limits, links of the web and the photos each once.
func NewArticle(authorID uuid.UUID, d ArticleDraft, now time.Time) (*Article, error) {
	a, err := checkArticle(d)
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	a.id, a.authorID = id, authorID
	// PostgreSQL keeps microseconds; trim now so it reads back equal.
	a.createdAt = now.UTC().Truncate(time.Microsecond)

	return a, nil
}

// Edited is a written of d anew, by its author only.
func (a *Article) Edited(requesterID uuid.UUID, d ArticleDraft, now time.Time) (*Article, error) {
	if !a.CanBeChangedBy(requesterID) {
		return nil, ErrCannotChangeArticle
	}

	edited, err := checkArticle(d)
	if err != nil {
		return nil, err
	}

	at := now.UTC().Truncate(time.Microsecond)
	edited.id, edited.authorID, edited.createdAt, edited.editedAt = a.id, a.authorID, a.createdAt, &at

	return edited, nil
}

func checkArticle(d ArticleDraft) (*Article, error) {
	title := strings.Join(strings.Fields(d.Title), " ")
	if title == "" {
		return nil, ErrArticleTitleEmpty
	}

	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > maxArticleTitle {
		return nil, ErrArticleTitleTooLong
	}

	if len(d.Blocks) > maxArticleBlocks {
		return nil, ErrArticleTooLong
	}

	blocks := make([]Block, 0, len(d.Blocks))
	length, hasText := 0, false
	for _, b := range d.Blocks {
		checked, n, err := checkBlock(b)
		if err != nil {
			return nil, err
		}

		// An empty paragraph is the editor's line between paragraphs: the
		// page spaces them itself.
		if b.Kind.hasText() && n == 0 {
			continue
		}

		length += n
		hasText = hasText || n > 0
		blocks = append(blocks, checked)
	}

	if !hasText {
		return nil, ErrArticleEmpty
	}

	if length > maxArticleText {
		return nil, ErrArticleTooLong
	}

	a := &Article{title: title, coverID: d.CoverID, blocks: blocks}
	if err := checkPhotos(a.Photos()); err != nil {
		return nil, err
	}

	return a, nil
}

// checkBlock checks a block and says how many characters of text it has.
func checkBlock(b Block) (Block, int, error) {
	switch {
	case b.Kind == BlockPhoto:
		caption := strings.Join(strings.Fields(b.Caption), " ")
		if b.PhotoID == uuid.Nil || len(b.Spans) > 0 || !utf8.ValidString(caption) || utf8.RuneCountInString(caption) > maxArticleCaption {
			return Block{}, 0, ErrArticleBlock
		}

		return Block{Kind: BlockPhoto, PhotoID: b.PhotoID, Caption: caption}, utf8.RuneCountInString(caption), nil
	case b.Kind == BlockDivider:
		if b.PhotoID != uuid.Nil || len(b.Spans) > 0 || b.Caption != "" {
			return Block{}, 0, ErrArticleBlock
		}

		return Block{Kind: BlockDivider}, 0, nil
	case b.Kind.hasText():
		if b.PhotoID != uuid.Nil || b.Caption != "" || len(b.Spans) > maxBlockSpans {
			return Block{}, 0, ErrArticleBlock
		}

		spans, n, err := checkSpans(b.Kind, b.Spans)
		if err != nil {
			return Block{}, 0, err
		}

		return Block{Kind: b.Kind, Spans: spans}, n, nil
	default:
		return Block{}, 0, ErrArticleBlock
	}
}

// checkSpans drops the empty spans and joins those written the same way;
// code is plain, and a block of text that is all spaces has none.
func checkSpans(kind BlockKind, spans []Span) ([]Span, int, error) {
	var out []Span
	n, blank := 0, true
	for _, s := range spans {
		if !utf8.ValidString(s.Text) {
			return nil, 0, ErrArticleBlock
		}

		s.Text = strings.ReplaceAll(s.Text, "\r", "")

		if s.Link != "" && !webLink(s.Link) {
			return nil, 0, ErrArticleLink
		}

		if kind == BlockCode && s.marked() {
			return nil, 0, ErrArticleBlock
		}

		if s.Text == "" {
			continue
		}

		n += utf8.RuneCountInString(s.Text)
		blank = blank && strings.TrimSpace(s.Text) == ""

		if last := len(out) - 1; last >= 0 && out[last].sameMarks(s) {
			out[last].Text += s.Text
			continue
		}

		out = append(out, s)
	}

	if blank {
		return nil, 0, nil
	}

	return out, n, nil
}

// webLink tells an http or https address with a host.
func webLink(s string) bool {
	u, err := url.Parse(s)

	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && len(s) <= maxArticleLink
}

func checkPhotos(photos []uuid.UUID) error {
	if len(photos) > maxArticlePhotos {
		return ErrArticlePhotos
	}

	seen := make(map[uuid.UUID]bool, len(photos))
	for _, id := range photos {
		if seen[id] {
			return ErrArticlePhotos
		}
		seen[id] = true
	}

	return nil
}

// StoredArticle is an article as storage keeps it.
type StoredArticle struct {
	ID        uuid.UUID
	AuthorID  uuid.UUID
	Title     string
	CoverID   uuid.UUID
	Blocks    []Block
	CreatedAt time.Time
	EditedAt  *time.Time
}

// RestoreArticle rebuilds an article from storage without validation.
func RestoreArticle(s StoredArticle) *Article {
	return &Article{
		id: s.ID, authorID: s.AuthorID, title: s.Title, coverID: s.CoverID, blocks: s.Blocks,
		createdAt: s.CreatedAt, editedAt: s.EditedAt,
	}
}

// CanBeChangedBy: only the author edits or deletes an article.
func (a *Article) CanBeChangedBy(userID uuid.UUID) bool {
	return userID == a.authorID
}

// Photos are the photos of the author the article shows: the cover and
// those of its text, in order.
func (a *Article) Photos() []uuid.UUID {
	var photos []uuid.UUID
	if a.coverID != uuid.Nil {
		photos = append(photos, a.coverID)
	}

	for _, b := range a.blocks {
		if b.Kind == BlockPhoto {
			photos = append(photos, b.PhotoID)
		}
	}

	return photos
}

// Excerpt is the start of the text of the article for its card, at most
// limit characters ending with a whole word, and whether it was cut.
func (a *Article) Excerpt(limit int) (string, bool) {
	var parts []string
	for _, b := range a.blocks {
		if b.Kind == BlockParagraph || b.Kind == BlockQuote || b.Kind == BlockBullet || b.Kind == BlockNumber {
			parts = append(parts, strings.Join(strings.Fields(blockText(b)), " "))
		}
	}

	return Text(strings.Join(parts, " ")).Excerpt(limit)
}

// Minutes is how long the article is read, at least a minute.
func (a *Article) Minutes() int {
	words := 0
	for _, b := range a.blocks {
		words += len(strings.Fields(blockText(b))) + len(strings.Fields(b.Caption))
	}

	return max(1, (words+articleWordsPerMinute-1)/articleWordsPerMinute)
}

func blockText(b Block) string {
	var sb strings.Builder
	for _, s := range b.Spans {
		sb.WriteString(s.Text)
	}

	return sb.String()
}

func (a *Article) ID() uuid.UUID        { return a.id }
func (a *Article) AuthorID() uuid.UUID  { return a.authorID }
func (a *Article) Title() string        { return a.title }
func (a *Article) CoverID() uuid.UUID   { return a.coverID }
func (a *Article) CreatedAt() time.Time { return a.createdAt }
func (a *Article) EditedAt() *time.Time { return a.editedAt }
func (a *Article) Blocks() []Block      { return a.blocks }
