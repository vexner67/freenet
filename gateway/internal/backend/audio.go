package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Audio is rest.Audio over the post service, which keeps what songs and
// playlists are; the bytes of uploaded songs are the media service's.
type Audio struct {
	api postv1.PostServiceClient
}

func NewAudio(api postv1.PostServiceClient) *Audio {
	return &Audio{
		api: api,
	}
}

// Add keeps a song for the caller: the MP3 media stored as FileID, or a
// track of another site.
func (a *Audio) Add(ctx context.Context, accessToken string, na model.NewAudio) (model.Audio, error) {
	ctx = withToken(ctx, accessToken)

	if na.FileID != "" {
		resp, err := a.api.AddAudio(ctx, &postv1.AddAudioRequest{Id: na.FileID, Artist: na.Artist, Title: na.Title, Duration: toDuration(na.Duration)})
		if err != nil {
			return model.Audio{}, errs.Wrap("a.api.AddAudio", fromStatus(err))
		}

		return toAudio(resp.GetAudio()), nil
	}

	resp, err := a.api.AddAudioTrack(ctx, &postv1.AddAudioTrackRequest{
		Kind: na.Kind, Link: na.Link, Cover: na.Cover, Artist: na.Artist, Title: na.Title, Duration: toDuration(na.Duration),
	})
	if err != nil {
		return model.Audio{}, errs.Wrap("a.api.AddAudioTrack", fromStatus(err))
	}

	return toAudio(resp.GetAudio()), nil
}

// List pages the songs ownerID keeps, as the caller sees them.
func (a *Audio) List(ctx context.Context, accessToken, ownerID, cursor string, limit int) (model.AudioPage, error) {
	resp, err := a.api.ListAudios(withToken(ctx, accessToken), &postv1.ListAudiosRequest{OwnerId: ownerID, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.AudioPage{}, errs.Wrap("a.api.ListAudios", fromStatus(err))
	}

	return model.AudioPage{Audios: toAudios(resp.GetAudios()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// Search pages everyone's songs with query, newest first; "" lists all.
func (a *Audio) Search(ctx context.Context, accessToken, query, cursor string, limit int) (model.AudioPage, error) {
	resp, err := a.api.SearchAudios(withToken(ctx, accessToken), &postv1.SearchAudiosRequest{Query: query, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.AudioPage{}, errs.Wrap("a.api.SearchAudios", fromStatus(err))
	}

	return model.AudioPage{Audios: toAudios(resp.GetAudios()), NextCursor: resp.GetNextCursor()}, nil
}

// GetMany returns the songs of ids that exist, at most 50, in no
// particular order; Added is of the one accessToken names, if any.
func (a *Audio) GetMany(ctx context.Context, accessToken string, ids []string) ([]model.Audio, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	resp, err := a.api.GetAudios(withToken(ctx, accessToken), &postv1.GetAudiosRequest{Ids: ids})
	if err != nil {
		return nil, errs.Wrap("a.api.GetAudios", fromStatus(err))
	}

	return toAudios(resp.GetAudios()), nil
}

func (a *Audio) Keep(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.KeepAudio(withToken(ctx, accessToken), &postv1.KeepAudioRequest{AudioId: id}); err != nil {
		return errs.Wrap("a.api.KeepAudio", fromStatus(err))
	}

	return nil
}

func (a *Audio) Forget(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.ForgetAudio(withToken(ctx, accessToken), &postv1.ForgetAudioRequest{AudioId: id}); err != nil {
		return errs.Wrap("a.api.ForgetAudio", fromStatus(err))
	}

	return nil
}

func (a *Audio) Edit(ctx context.Context, accessToken, id, artist, title string) (model.Audio, error) {
	resp, err := a.api.EditAudio(withToken(ctx, accessToken), &postv1.EditAudioRequest{Id: id, Artist: artist, Title: title})
	if err != nil {
		return model.Audio{}, errs.Wrap("a.api.EditAudio", fromStatus(err))
	}

	return toAudio(resp.GetAudio()), nil
}

func (a *Audio) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.DeleteAudio(withToken(ctx, accessToken), &postv1.DeleteAudioRequest{Id: id}); err != nil {
		return errs.Wrap("a.api.DeleteAudio", fromStatus(err))
	}

	return nil
}

// Listen says what the caller plays now; "" for nothing.
func (a *Audio) Listen(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.ListenAudio(withToken(ctx, accessToken), &postv1.ListenAudioRequest{AudioId: id}); err != nil {
		return errs.Wrap("a.api.ListenAudio", fromStatus(err))
	}

	return nil
}

// SetPageAudio puts a song on the caller's page; "" takes it off.
func (a *Audio) SetPageAudio(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.SetPageAudio(withToken(ctx, accessToken), &postv1.SetPageAudioRequest{AudioId: id}); err != nil {
		return errs.Wrap("a.api.SetPageAudio", fromStatus(err))
	}

	return nil
}

// Music is what the page of userID says of music.
func (a *Audio) Music(ctx context.Context, userID string) (model.Music, error) {
	resp, err := a.api.GetMusic(ctx, &postv1.GetMusicRequest{UserId: userID})
	if err != nil {
		return model.Music{}, errs.Wrap("a.api.GetMusic", fromStatus(err))
	}

	m := model.Music{Count: int(resp.GetCount())}
	if resp.GetListening() != nil {
		listening := toAudio(resp.GetListening())
		m.Listening = &listening
	}
	if resp.GetPageAudio() != nil {
		page := toAudio(resp.GetPageAudio())
		m.PageAudio = &page
	}

	return m, nil
}

func (a *Audio) CreatePlaylist(ctx context.Context, accessToken, title string) (model.Playlist, error) {
	resp, err := a.api.CreatePlaylist(withToken(ctx, accessToken), &postv1.CreatePlaylistRequest{Title: title})
	if err != nil {
		return model.Playlist{}, errs.Wrap("a.api.CreatePlaylist", fromStatus(err))
	}

	return toPlaylist(resp.GetPlaylist()), nil
}

func (a *Audio) RenamePlaylist(ctx context.Context, accessToken, id, title string) (model.Playlist, error) {
	resp, err := a.api.RenamePlaylist(withToken(ctx, accessToken), &postv1.RenamePlaylistRequest{Id: id, Title: title})
	if err != nil {
		return model.Playlist{}, errs.Wrap("a.api.RenamePlaylist", fromStatus(err))
	}

	return toPlaylist(resp.GetPlaylist()), nil
}

func (a *Audio) DeletePlaylist(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.DeletePlaylist(withToken(ctx, accessToken), &postv1.DeletePlaylistRequest{Id: id}); err != nil {
		return errs.Wrap("a.api.DeletePlaylist", fromStatus(err))
	}

	return nil
}

func (a *Audio) Playlists(ctx context.Context, ownerID, cursor string, limit int) (model.PlaylistPage, error) {
	resp, err := a.api.ListPlaylists(ctx, &postv1.ListPlaylistsRequest{OwnerId: ownerID, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.PlaylistPage{}, errs.Wrap("a.api.ListPlaylists", fromStatus(err))
	}

	page := model.PlaylistPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pp := range resp.GetPlaylists() {
		page.Playlists = append(page.Playlists, toPlaylist(pp))
	}

	return page, nil
}

// Playlist is a playlist with a page of its songs, as the caller sees them.
func (a *Audio) Playlist(ctx context.Context, accessToken, id, cursor string, limit int) (model.Playlist, model.AudioPage, error) {
	resp, err := a.api.GetPlaylist(withToken(ctx, accessToken), &postv1.GetPlaylistRequest{Id: id, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.Playlist{}, model.AudioPage{}, errs.Wrap("a.api.GetPlaylist", fromStatus(err))
	}

	page := model.AudioPage{Audios: toAudios(resp.GetAudios()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}

	return toPlaylist(resp.GetPlaylist()), page, nil
}

func (a *Audio) AddToPlaylist(ctx context.Context, accessToken, playlistID, audioID string) error {
	if _, err := a.api.AddPlaylistAudio(withToken(ctx, accessToken), &postv1.AddPlaylistAudioRequest{PlaylistId: playlistID, AudioId: audioID}); err != nil {
		return errs.Wrap("a.api.AddPlaylistAudio", fromStatus(err))
	}

	return nil
}

func (a *Audio) RemoveFromPlaylist(ctx context.Context, accessToken, playlistID, audioID string) error {
	if _, err := a.api.RemovePlaylistAudio(withToken(ctx, accessToken), &postv1.RemovePlaylistAudioRequest{PlaylistId: playlistID, AudioId: audioID}); err != nil {
		return errs.Wrap("a.api.RemovePlaylistAudio", fromStatus(err))
	}

	return nil
}

// toDuration keeps a duration within int32; the post service refuses
// what is too long.
func toDuration(seconds int) int32 {
	return int32(min(max(seconds, -1), 1<<30)) //nolint:gosec // clamped
}

func toAudios(pas []*postv1.Audio) []model.Audio {
	out := make([]model.Audio, 0, len(pas))
	for _, pa := range pas {
		out = append(out, toAudio(pa))
	}

	return out
}

func toAudio(pa *postv1.Audio) model.Audio {
	return model.Audio{
		ID:        pa.GetId(),
		OwnerID:   pa.GetOwnerId(),
		Kind:      pa.GetKind(),
		Link:      pa.GetLink(),
		Cover:     pa.GetCover(),
		Artist:    pa.GetArtist(),
		Title:     pa.GetTitle(),
		Duration:  int(pa.GetDuration()),
		CreatedAt: pa.GetCreatedAt().AsTime(),
		Added:     pa.GetAdded(),
	}
}

func toPlaylist(pp *postv1.Playlist) model.Playlist {
	return model.Playlist{
		ID:        pp.GetId(),
		OwnerID:   pp.GetOwnerId(),
		Title:     pp.GetTitle(),
		CreatedAt: pp.GetCreatedAt().AsTime(),
	}
}
