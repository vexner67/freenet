package rest

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// linkCard is the card of the first page text links to, its picture
// uploaded and published as a photo of the author (authorID), whose post
// keeps it and deletes it with itself; nil when there is no link or the
// page tells nothing. A card is a nicety: whatever fails, the post goes
// on without it, or without its picture.
func (a *api) linkCard(r *http.Request, token, authorID, text string) *model.Link {
	address := model.FirstLink(text)
	if a.previews == nil || address == "" || ownArticle(r, address) {
		return nil
	}

	p, err := a.previews.Read(r.Context(), address)
	if err != nil {
		a.logger.DebugContext(r.Context(), "no link preview", "error", err)
		return nil
	}

	link := &model.Link{URL: p.URL, Title: p.Title, Description: p.Description, Site: p.Site}
	if len(p.Image) == 0 {
		return link
	}

	photoID, err := a.uploadClaimed(r, token, p.Image)
	if err != nil {
		// Not an image media takes, or the limit of uploads.
		a.logger.DebugContext(r.Context(), "link preview image", "error", err)
		return link
	}

	// Unpublished, the picture would go after a day.
	if err = a.media.PublishPhotos(r.Context(), authorID, []string{photoID}); err != nil {
		a.logger.WarnContext(r.Context(), "publish link preview image", "error", err)
		return link
	}

	link.PhotoID = photoID

	return link
}

// ownArticle tells a link to an article of this site, whose card the site
// draws itself from the article.
func ownArticle(r *http.Request, address string) bool {
	u, err := url.Parse(address)

	return err == nil && strings.EqualFold(u.Host, r.Host) && strings.HasPrefix(u.Path, "/article/")
}

// dropCard deletes the picture of a card whose post was not written.
func (a *api) dropCard(r *http.Request, authorID string, link *model.Link) {
	if link == nil || link.PhotoID == "" {
		return
	}

	if err := a.media.DeletePublishedPhotos(r.Context(), authorID, []string{link.PhotoID}); err != nil {
		a.logger.WarnContext(r.Context(), "delete link preview image", "error", err)
	}
}
