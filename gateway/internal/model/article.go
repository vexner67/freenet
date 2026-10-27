package model

import "time"

// ArticleSpan is a piece of the text of a block written one way.
type ArticleSpan struct {
	Text   string
	Bold   bool
	Italic bool
	Strike bool
	// Link is an http or https address, "" for none.
	Link string
}

// ArticleBlock is a block of an article: Kind is "paragraph", "heading",
// "subheading", "quote", "bullet", "number", "code", "photo" or
// "divider".
type ArticleBlock struct {
	Kind    string
	Spans   []ArticleSpan
	PhotoID string
	Caption string
}

// ArticleDraft is what an article is written of.
type ArticleDraft struct {
	Title   string
	CoverID string
	Blocks  []ArticleBlock
}

// Photos are the photos of the draft: the cover and those of the text.
func (d ArticleDraft) Photos() []string {
	var photos []string
	if d.CoverID != "" {
		photos = append(photos, d.CoverID)
	}

	for _, b := range d.Blocks {
		if b.PhotoID != "" {
			photos = append(photos, b.PhotoID)
		}
	}

	return photos
}

// Article is a long read of the editor; a brief one has no blocks.
type Article struct {
	ID       string
	AuthorID string
	ArticleDraft
	Excerpt   string
	Minutes   int
	CreatedAt time.Time
	EditedAt  *time.Time
}

// ArticlePage is one screen of the articles of a person, brief.
type ArticlePage struct {
	Articles   []Article
	NextCursor string
	Total      int
}
