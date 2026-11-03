package rest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	// maxAudioBytes matches what the media service accepts.
	maxAudioBytes = 20 << 20
	// maxAttachedAudios is how many songs a post or a message attaches.
	maxAttachedAudios = 10
	// audioBatch is how many songs the post service tells at once.
	audioBatch = 50
)

// Audio is "Аудиозаписи" of openvk, which the post service keeps: songs
// people keep in their libraries and playlists, and posts and messages
// attach. Methods that read take an optional access token ("" for
// anonymous viewers), with which songs tell whether the viewer keeps
// them.
type Audio interface {
	Add(ctx context.Context, accessToken string, na model.NewAudio) (model.Audio, error)
	List(ctx context.Context, accessToken, ownerID, cursor string, limit int) (model.AudioPage, error)
	Search(ctx context.Context, accessToken, query, cursor string, limit int) (model.AudioPage, error)
	// GetMany tells the songs of up to 50 ids that exist, and which of
	// them the one accessToken names keeps.
	GetMany(ctx context.Context, accessToken string, ids []string) ([]model.Audio, error)
	Keep(ctx context.Context, accessToken, id string) error
	Forget(ctx context.Context, accessToken, id string) error
	Edit(ctx context.Context, accessToken, id, artist, title string) (model.Audio, error)
	Delete(ctx context.Context, accessToken, id string) error
	Listen(ctx context.Context, accessToken, id string) error
	SetPageAudio(ctx context.Context, accessToken, id string) error
	Music(ctx context.Context, userID string) (model.Music, error)

	CreatePlaylist(ctx context.Context, accessToken, title string) (model.Playlist, error)
	RenamePlaylist(ctx context.Context, accessToken, id, title string) (model.Playlist, error)
	DeletePlaylist(ctx context.Context, accessToken, id string) error
	Playlists(ctx context.Context, ownerID, cursor string, limit int) (model.PlaylistPage, error)
	Playlist(ctx context.Context, accessToken, id, cursor string, limit int) (model.Playlist, model.AudioPage, error)
	AddToPlaylist(ctx context.Context, accessToken, playlistID, audioID string) error
	RemoveFromPlaylist(ctx context.Context, accessToken, playlistID, audioID string) error
}

// Tracks tell what a track of SoundCloud or YouTube is by its link.
type Tracks interface {
	Track(ctx context.Context, link string) (model.Track, error)
}

// audioJSON is a song as a list, a post or a message shows it: url plays
// an uploaded one, link opens a track on its site.
type audioJSON struct {
	ID        string `json:"id"`
	OwnerID   string `json:"ownerId"`
	Kind      string `json:"kind"`
	URL       string `json:"url,omitempty"`
	Link      string `json:"link,omitempty"`
	Cover     string `json:"cover,omitempty"`
	Artist    string `json:"artist"`
	Title     string `json:"title"`
	Duration  int    `json:"duration"`
	Added     bool   `json:"added"`
	CreatedAt string `json:"createdAt"`
}

type audioPageResponse struct {
	Audios     []audioJSON `json:"audios"`
	NextCursor string      `json:"nextCursor"`
	Total      int         `json:"total"`
}

type addTrackRequest struct {
	Link string `json:"link"`
}

type editAudioRequest struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

type audioRefRequest struct {
	Audio string `json:"audio"`
}

type musicResponse struct {
	// Listening is what the person plays now, nil for nothing.
	Listening *audioJSON `json:"listening"`
	PageAudio *audioJSON `json:"pageAudio"`
	Count     int        `json:"count"`
}

type playlistJSON struct {
	ID        string `json:"id"`
	OwnerID   string `json:"ownerId"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
}

type playlistPageResponse struct {
	Playlists  []playlistJSON `json:"playlists"`
	NextCursor string         `json:"nextCursor"`
	Total      int            `json:"total"`
}

type playlistResponse struct {
	playlistJSON
	audioPageResponse
}

type playlistTitleRequest struct {
	Title string `json:"title"`
}

func toAudioJSON(a model.Audio) audioJSON {
	j := audioJSON{
		ID: a.ID, OwnerID: a.OwnerID, Kind: a.Kind, Link: a.Link, Cover: a.Cover, Artist: a.Artist, Title: a.Title,
		Duration: a.Duration, Added: a.Added, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
	if a.Kind == model.AudioFile {
		j.URL = "/media/audio/" + url.PathEscape(a.ID)
	}

	return j
}

func toAudioPageResponse(page model.AudioPage) audioPageResponse {
	resp := audioPageResponse{Audios: make([]audioJSON, 0, len(page.Audios)), NextCursor: page.NextCursor, Total: page.Total}
	for _, s := range page.Audios {
		resp.Audios = append(resp.Audios, toAudioJSON(s))
	}

	return resp
}

func toPlaylistJSON(p model.Playlist) playlistJSON {
	return playlistJSON{ID: p.ID, OwnerID: p.OwnerID, Title: p.Title, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339)}
}

// audiosJSON are the songs of ids that are still there, in order.
func audiosJSON(ids []string, known map[string]model.Audio) []audioJSON {
	if len(ids) == 0 {
		return nil
	}

	songs := make([]audioJSON, 0, len(ids))
	for _, id := range ids {
		if s, ok := known[id]; ok {
			songs = append(songs, toAudioJSON(s))
		}
	}

	return songs
}

// audiosOf tells the songs of ids by ID, a batch at a time; a deleted one
// is not there.
func (a *api) audiosOf(ctx context.Context, token string, ids []string) (map[string]model.Audio, error) {
	known := make(map[string]model.Audio)
	if a.audio == nil {
		return known, nil
	}

	wanted := uniqueIDs(ids)
	for len(wanted) > 0 {
		n := min(len(wanted), audioBatch)

		found, err := a.audio.GetMany(ctx, token, wanted[:n])
		if err != nil {
			return nil, err
		}

		for _, s := range found {
			known[s.ID] = s
		}

		wanted = wanted[n:]
	}

	return known, nil
}

// checkAudios makes sure the songs a post or a message is about to attach
// exist: anyone's song may be shared, as in openvk.
func (a *api) checkAudios(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	if len(ids) > maxAttachedAudios {
		return failure.New(failure.InvalidArgument, "too many audios")
	}

	known, err := a.audiosOf(ctx, "", ids)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return failure.New(failure.InvalidArgument, "audio not found")
		}
	}

	return nil
}

// postAudios are the songs the posts attach, by ID; none asked when no
// post attaches one.
func (a *api) postAudios(r *http.Request, posts ...model.Post) (map[string]model.Audio, error) {
	var ids []string
	for _, p := range posts {
		ids = append(ids, p.AudioIDs...)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	return a.audiosOf(r.Context(), viewerToken(r), ids)
}

// listAudios is "Аудиозаписи" of a person: the songs they keep, the last
// added first, to whoever sees their page.
func (a *api) listAudios(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.visiblePage(r, token, pagePart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.audio.List(r.Context(), token, owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAudioPageResponse(page))
}

// searchAudios is "Поиск" of the audio page; without ?q= it is "Новое":
// every song, newest first.
func (a *api) searchAudios(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.audio.Search(r.Context(), token, r.URL.Query().Get("q"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAudioPageResponse(page))
}

// uploadAudio takes the raw MP3 as the request body, and ?artist=,
// ?title= and ?duration= (seconds) as the browser read them: the media
// service keeps the bytes, the post service what the song is. A file the
// post service refuses is deleted from media again.
func (a *api) uploadAudio(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	q := r.URL.Query()
	duration := 0
	if s := q.Get("duration"); s != "" {
		if duration, err = strconv.Atoi(s); err != nil {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid audio duration"))
			return
		}
	}

	data, err := readAudio(w, r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	id, err := a.media.UploadAudio(r.Context(), token, data)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	song, err := a.audio.Add(r.Context(), token, model.NewAudio{FileID: id, Artist: q.Get("artist"), Title: q.Get("title"), Duration: duration})
	if err != nil {
		a.dropAudio(r, me.ID, id)
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAudioJSON(song))
}

func readAudio(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAudioBytes))

	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return nil, failure.New(failure.InvalidArgument, "audio is larger than 20 MB")
	}

	if err == nil && len(data) == 0 {
		return nil, failure.New(failure.InvalidArgument, "audio is empty")
	}

	return data, err
}

// dropAudio deletes an MP3 media kept for a song the post service
// refused; the client may be gone already.
func (a *api) dropAudio(r *http.Request, ownerID, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), releaseTimeout)
	defer cancel()

	if err := a.media.DeleteAudio(ctx, ownerID, []string{id}); err != nil {
		a.logger.WarnContext(ctx, "refused audio not deleted", "error", err)
	}
}

// addTrack adds a track of SoundCloud or YouTube by its link, as the site
// tells of it. A track its owner keeps to their site cannot play here:
// that is a conflict the page shows with the link to listen there.
func (a *api) addTrack(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req addTrackRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if a.tracks == nil {
		a.writeError(w, r, failure.New(failure.Unavailable, "tracks of other sites are off"))
		return
	}

	track, err := a.tracks.Track(r.Context(), req.Link)
	if err != nil {
		if failure.From(err).Kind == failure.Internal {
			a.logger.WarnContext(r.Context(), "track not read", "error", err)
			err = failure.New(failure.Unavailable, "the site of the track does not answer")
		}
		a.writeError(w, r, err)
		return
	}

	if !track.Playable {
		a.writeError(w, r, failure.New(failure.Conflict, "this track cannot be played on other sites"))
		return
	}

	song, err := a.audio.Add(r.Context(), token, model.NewAudio{
		Kind: track.Kind, Link: track.Link, Cover: track.Cover, Artist: track.Artist, Title: track.Title,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAudioJSON(song))
}

func (a *api) keepAudio(w http.ResponseWriter, r *http.Request) {
	a.audioAction(w, r, a.audio.Keep)
}

func (a *api) forgetAudio(w http.ResponseWriter, r *http.Request) {
	a.audioAction(w, r, a.audio.Forget)
}

func (a *api) deleteAudio(w http.ResponseWriter, r *http.Request) {
	a.audioAction(w, r, a.audio.Delete)
}

// audioAction does what the caller asked with the song of the path.
func (a *api) audioAction(w http.ResponseWriter, r *http.Request, do func(ctx context.Context, accessToken, id string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = do(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) editAudio(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req editAudioRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	song, err := a.audio.Edit(r.Context(), token, r.PathValue("id"), req.Artist, req.Title)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAudioJSON(song))
}

// setListening and setPageAudio take {"audio": id}; the DELETE of the
// same path takes it off.
func (a *api) setListening(w http.ResponseWriter, r *http.Request) {
	a.audioRef(w, r, a.audio.Listen)
}

func (a *api) stopListening(w http.ResponseWriter, r *http.Request) {
	a.audioRefNone(w, r, a.audio.Listen)
}

func (a *api) setPageAudio(w http.ResponseWriter, r *http.Request) {
	a.audioRef(w, r, a.audio.SetPageAudio)
}

func (a *api) removePageAudio(w http.ResponseWriter, r *http.Request) {
	a.audioRefNone(w, r, a.audio.SetPageAudio)
}

func (a *api) audioRef(w http.ResponseWriter, r *http.Request, set func(ctx context.Context, accessToken, id string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req audioRefRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if req.Audio == "" {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid audio id"))
		return
	}

	if err = set(r.Context(), token, req.Audio); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) audioRefNone(w http.ResponseWriter, r *http.Request, set func(ctx context.Context, accessToken, id string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = set(r.Context(), token, ""); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getMusic is what a page says of music: "слушает" under the name, the
// song of the page, how many songs it keeps.
func (a *api) getMusic(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.visiblePage(r, token, pagePart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.audio.Music(r.Context(), owner.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := musicResponse{Count: m.Count}
	if m.Listening != nil {
		j := toAudioJSON(*m.Listening)
		resp.Listening = &j
	}
	if m.PageAudio != nil {
		j := toAudioJSON(*m.PageAudio)
		resp.PageAudio = &j
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *api) listPlaylists(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.visiblePage(r, token, pagePart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.audio.Playlists(r.Context(), owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := playlistPageResponse{Playlists: make([]playlistJSON, 0, len(page.Playlists)), NextCursor: page.NextCursor, Total: page.Total}
	for _, p := range page.Playlists {
		resp.Playlists = append(resp.Playlists, toPlaylistJSON(p))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *api) getPlaylist(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	p, page, err := a.audio.Playlist(r.Context(), token, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, playlistResponse{playlistJSON: toPlaylistJSON(p), audioPageResponse: toAudioPageResponse(page)})
}

func (a *api) createPlaylist(w http.ResponseWriter, r *http.Request) {
	a.playlistTitle(w, r, http.StatusCreated, func(ctx context.Context, token, title string) (model.Playlist, error) {
		return a.audio.CreatePlaylist(ctx, token, title)
	})
}

func (a *api) renamePlaylist(w http.ResponseWriter, r *http.Request) {
	a.playlistTitle(w, r, http.StatusOK, func(ctx context.Context, token, title string) (model.Playlist, error) {
		return a.audio.RenamePlaylist(ctx, token, r.PathValue("id"), title)
	})
}

func (a *api) playlistTitle(w http.ResponseWriter, r *http.Request, code int, do func(ctx context.Context, token, title string) (model.Playlist, error)) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req playlistTitleRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	p, err := do(r.Context(), token, req.Title)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, code, toPlaylistJSON(p))
}

func (a *api) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	a.audioAction(w, r, a.audio.DeletePlaylist)
}

func (a *api) addToPlaylist(w http.ResponseWriter, r *http.Request) {
	a.playlistAudio(w, r, a.audio.AddToPlaylist)
}

func (a *api) removeFromPlaylist(w http.ResponseWriter, r *http.Request) {
	a.playlistAudio(w, r, a.audio.RemoveFromPlaylist)
}

func (a *api) playlistAudio(w http.ResponseWriter, r *http.Request, do func(ctx context.Context, accessToken, playlistID, audioID string) error) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = do(r.Context(), token, r.PathValue("id"), r.PathValue("audioID")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// serveAudio plays an uploaded song by its ID, to whoever sees what
// attaches it: the ID is the secret, as with photos. It answers ranges,
// so the player seeks without loading the whole file.
func (a *api) serveAudio(w http.ResponseWriter, r *http.Request) {
	known, err := a.audiosOf(r.Context(), "", []string{r.PathValue("id")})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	song, ok := known[r.PathValue("id")]
	if !ok || song.Kind != model.AudioFile {
		a.writeError(w, r, failure.New(failure.NotFound, "audio not found"))
		return
	}

	data, err := a.media.Audio(r.Context(), song.OwnerID, song.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The bytes never change, but a deleted song goes away.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", song.CreatedAt, bytes.NewReader(data))
}
