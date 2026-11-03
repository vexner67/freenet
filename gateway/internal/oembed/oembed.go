// Package oembed asks SoundCloud and YouTube what a track is by its link,
// through their oEmbed endpoints, and whether its owner lets other sites
// play it in their player: a track they may not embed answers 401 or
// 403, as one that is private or gone answers 404.
//
// The reader goes only to those two fixed endpoints, so the link a person
// gives decides nothing but the query.
package oembed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	maxAnswerBytes = 64 << 10
	maxText        = 128

	userAgent = "Mozilla/5.0 (compatible; FreenetBot/1.0; oEmbed)"
)

var (
	// ErrNotTrack: the link is not one of a track of SoundCloud or YouTube.
	ErrNotTrack = failure.New(failure.InvalidArgument, "this is not a link to a track")
	// ErrNoTrack: the site knows no such track.
	ErrNoTrack = failure.New(failure.NotFound, "track not found")
)

// Endpoints are the oEmbed endpoints of the sites.
type Endpoints struct {
	SoundCloud string
	YouTube    string
}

// Sites are the real endpoints.
var Sites = Endpoints{
	SoundCloud: "https://soundcloud.com/oembed",
	YouTube:    "https://www.youtube.com/oembed",
}

// Reader asks the sites; its zero value is not usable, see New.
type Reader struct {
	client    *http.Client
	endpoints Endpoints
}

// New is a Reader that spends at most timeout on a link.
func New(timeout time.Duration, endpoints Endpoints) *Reader {
	return &Reader{
		client: &http.Client{
			Timeout: timeout,
			// The endpoints answer themselves; a redirect goes nowhere we asked.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		endpoints: endpoints,
	}
}

// Kind is the site of a link: model.AudioSoundCloud or model.AudioYouTube,
// "" for neither.
func Kind(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}

	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "soundcloud.com", "m.soundcloud.com":
		return model.AudioSoundCloud
	case "youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be":
		return model.AudioYouTube
	default:
		return ""
	}
}

type answer struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// Track tells what the track of link is.
func (r *Reader) Track(ctx context.Context, link string) (model.Track, error) {
	link = strings.TrimSpace(link)

	kind := Kind(link)
	endpoint := r.endpoints.YouTube
	switch kind {
	case model.AudioSoundCloud:
		endpoint = r.endpoints.SoundCloud
	case "":
		return model.Track{}, ErrNotTrack
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+url.Values{"format": {"json"}, "url": {link}}.Encode(), nil)
	if err != nil {
		return model.Track{}, fmt.Errorf("http.NewRequestWithContext: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return model.Track{}, fmt.Errorf("r.client.Do: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read only

	track := model.Track{Kind: kind, Link: link}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		// It is there, but its owner keeps it to their site.
		return track, nil
	case http.StatusNotFound, http.StatusBadRequest:
		return model.Track{}, ErrNoTrack
	default:
		return model.Track{}, fmt.Errorf("oembed answered %d", resp.StatusCode)
	}

	var a answer
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxAnswerBytes)).Decode(&a); err != nil {
		return model.Track{}, fmt.Errorf("json.Decode: %w", err)
	}

	track.Playable = true
	track.Artist = clip(a.AuthorName)
	// SoundCloud names its tracks "Title by Artist".
	track.Title = clip(strings.TrimSuffix(a.Title, " by "+a.AuthorName))
	// A video of a song is named "Artist - Title" on YouTube, whichever
	// channel posted it.
	if artist, title, ok := strings.Cut(track.Title, " - "); ok && kind == model.AudioYouTube && artist != "" && title != "" {
		track.Artist, track.Title = clip(artist), clip(title)
	}
	if strings.HasPrefix(a.ThumbnailURL, "https://") {
		track.Cover = a.ThumbnailURL
	}

	return track, nil
}

// clip is s in one line of at most maxText characters.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxText {
		return string(r[:maxText])
	}

	return s
}
