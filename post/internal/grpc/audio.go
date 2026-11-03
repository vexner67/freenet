package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AudioService interface {
	AddFile(ctx context.Context, id uuid.UUID, d post.AudioDraft) (*post.Audio, error)
	AddTrack(ctx context.Context, d post.AudioDraft, source post.AudioSource) (*post.Audio, error)
	Library(ctx context.Context, viewerID, ownerID, cursor uuid.UUID, size int) (service.AudioPage, error)
	Search(ctx context.Context, viewerID uuid.UUID, query string, cursor uuid.UUID, size int) (service.AudioPage, error)
	ByID(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) (service.AudioPage, error)
	Keep(ctx context.Context, userID, audioID uuid.UUID) error
	Forget(ctx context.Context, userID, audioID uuid.UUID) error
	Edit(ctx context.Context, requesterID, id uuid.UUID, artist, title post.AudioName) (*post.Audio, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	Listen(ctx context.Context, userID, audioID uuid.UUID) error
	SetPageAudio(ctx context.Context, userID, audioID uuid.UUID) error
	Music(ctx context.Context, userID uuid.UUID) (service.Music, error)
}

type PlaylistService interface {
	Create(ctx context.Context, ownerID uuid.UUID, title post.PlaylistTitle) (*post.Playlist, error)
	Rename(ctx context.Context, requesterID, id uuid.UUID, title post.PlaylistTitle) (*post.Playlist, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	List(ctx context.Context, ownerID, cursor uuid.UUID, size int) (service.PlaylistPage, error)
	Get(ctx context.Context, viewerID, id, cursor uuid.UUID, size int) (*post.Playlist, service.AudioPage, error)
	Add(ctx context.Context, requesterID, id, audioID uuid.UUID) error
	Remove(ctx context.Context, requesterID, id, audioID uuid.UUID) error
}

// AddAudio keeps an MP3 the media service has stored for the caller.
func (h *PostHandler) AddAudio(ctx context.Context, req *postv1.AddAudioRequest) (*postv1.AddAudioResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseAudioID(req.GetId())
	if err != nil {
		return nil, err
	}

	d, err := fromProtoAudioDraft(userID, req.GetArtist(), req.GetTitle(), req.GetDuration())
	if err != nil {
		return nil, err
	}

	a, err := h.audioService.AddFile(ctx, id, d)
	if err != nil {
		return nil, err
	}

	return &postv1.AddAudioResponse{Audio: toProtoAudio(a, true)}, nil
}

func (h *PostHandler) AddAudioTrack(ctx context.Context, req *postv1.AddAudioTrackRequest) (*postv1.AddAudioTrackResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := post.NewAudioKind(req.GetKind())
	if err != nil || kind == post.AudioFile {
		return nil, status.Error(codes.InvalidArgument, post.ErrAudioKind.Error())
	}

	source, err := post.NewAudioSource(kind, req.GetLink(), req.GetCover())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	d, err := fromProtoAudioDraft(userID, req.GetArtist(), req.GetTitle(), req.GetDuration())
	if err != nil {
		return nil, err
	}

	a, err := h.audioService.AddTrack(ctx, d, source)
	if err != nil {
		return nil, err
	}

	return &postv1.AddAudioTrackResponse{Audio: toProtoAudio(a, true)}, nil
}

func (h *PostHandler) ListAudios(ctx context.Context, req *postv1.ListAudiosRequest) (*postv1.ListAudiosResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.audioService.Library(ctx, viewerID(ctx), ownerID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.ListAudiosResponse{
		Audios:     toProtoAudioPage(page),
		NextCursor: idString(page.NextCursor),
		Total:      int32(page.Total),
	}, nil
}

func (h *PostHandler) SearchAudios(ctx context.Context, req *postv1.SearchAudiosRequest) (*postv1.SearchAudiosResponse, error) {
	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.audioService.Search(ctx, viewerID(ctx), req.GetQuery(), cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.SearchAudiosResponse{Audios: toProtoAudioPage(page), NextCursor: idString(page.NextCursor)}, nil
}

func (h *PostHandler) GetAudios(ctx context.Context, req *postv1.GetAudiosRequest) (*postv1.GetAudiosResponse, error) {
	ids, err := parseIDs(req.GetIds())
	if err != nil {
		return nil, err
	}

	page, err := h.audioService.ByID(ctx, viewerID(ctx), ids)
	if err != nil {
		return nil, err
	}

	return &postv1.GetAudiosResponse{Audios: toProtoAudioPage(page)}, nil
}

func (h *PostHandler) KeepAudio(ctx context.Context, req *postv1.KeepAudioRequest) (*emptypb.Empty, error) {
	return h.withAudio(ctx, req.GetAudioId(), h.audioService.Keep)
}

func (h *PostHandler) ForgetAudio(ctx context.Context, req *postv1.ForgetAudioRequest) (*emptypb.Empty, error) {
	return h.withAudio(ctx, req.GetAudioId(), h.audioService.Forget)
}

func (h *PostHandler) DeleteAudio(ctx context.Context, req *postv1.DeleteAudioRequest) (*emptypb.Empty, error) {
	return h.withAudio(ctx, req.GetId(), h.audioService.Delete)
}

// ListenAudio and SetPageAudio take an empty ID for none.
func (h *PostHandler) ListenAudio(ctx context.Context, req *postv1.ListenAudioRequest) (*emptypb.Empty, error) {
	return h.withOptionalAudio(ctx, req.GetAudioId(), h.audioService.Listen)
}

func (h *PostHandler) SetPageAudio(ctx context.Context, req *postv1.SetPageAudioRequest) (*emptypb.Empty, error) {
	return h.withOptionalAudio(ctx, req.GetAudioId(), h.audioService.SetPageAudio)
}

// withAudio calls do for the caller and the song.
func (h *PostHandler) withAudio(ctx context.Context, raw string, do func(ctx context.Context, userID, audioID uuid.UUID) error) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseAudioID(raw)
	if err != nil {
		return nil, err
	}

	if err = do(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) withOptionalAudio(ctx context.Context, raw string, do func(ctx context.Context, userID, audioID uuid.UUID) error) (*emptypb.Empty, error) {
	if raw == "" {
		userID, err := currentUserID(ctx)
		if err != nil {
			return nil, err
		}

		if err = do(ctx, userID, uuid.Nil); err != nil {
			return nil, err
		}

		return &emptypb.Empty{}, nil
	}

	return h.withAudio(ctx, raw, do)
}

func (h *PostHandler) EditAudio(ctx context.Context, req *postv1.EditAudioRequest) (*postv1.EditAudioResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseAudioID(req.GetId())
	if err != nil {
		return nil, err
	}

	artist, title, err := fromProtoAudioNames(req.GetArtist(), req.GetTitle())
	if err != nil {
		return nil, err
	}

	a, err := h.audioService.Edit(ctx, userID, id, artist, title)
	if err != nil {
		return nil, err
	}

	return &postv1.EditAudioResponse{Audio: toProtoAudio(a, true)}, nil
}

func (h *PostHandler) GetMusic(ctx context.Context, req *postv1.GetMusicRequest) (*postv1.GetMusicResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	m, err := h.audioService.Music(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := &postv1.GetMusicResponse{Count: int32(m.Count)}
	if m.Listening != nil {
		resp.Listening = toProtoAudio(m.Listening, false)
	}
	if m.PageAudio != nil {
		resp.PageAudio = toProtoAudio(m.PageAudio, false)
	}

	return resp, nil
}

func (h *PostHandler) CreatePlaylist(ctx context.Context, req *postv1.CreatePlaylistRequest) (*postv1.CreatePlaylistResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	title, err := post.NewPlaylistTitle(req.GetTitle())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.playlistService.Create(ctx, userID, title)
	if err != nil {
		return nil, err
	}

	return &postv1.CreatePlaylistResponse{Playlist: toProtoPlaylist(p)}, nil
}

func (h *PostHandler) RenamePlaylist(ctx context.Context, req *postv1.RenamePlaylistRequest) (*postv1.RenamePlaylistResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parsePlaylistID(req.GetId())
	if err != nil {
		return nil, err
	}

	title, err := post.NewPlaylistTitle(req.GetTitle())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.playlistService.Rename(ctx, userID, id, title)
	if err != nil {
		return nil, err
	}

	return &postv1.RenamePlaylistResponse{Playlist: toProtoPlaylist(p)}, nil
}

func (h *PostHandler) DeletePlaylist(ctx context.Context, req *postv1.DeletePlaylistRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parsePlaylistID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.playlistService.Delete(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) ListPlaylists(ctx context.Context, req *postv1.ListPlaylistsRequest) (*postv1.ListPlaylistsResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.playlistService.List(ctx, ownerID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	out := make([]*postv1.Playlist, len(page.Playlists))
	for i, p := range page.Playlists {
		out[i] = toProtoPlaylist(p)
	}

	return &postv1.ListPlaylistsResponse{Playlists: out, NextCursor: idString(page.NextCursor), Total: int32(page.Total)}, nil
}

func (h *PostHandler) GetPlaylist(ctx context.Context, req *postv1.GetPlaylistRequest) (*postv1.GetPlaylistResponse, error) {
	id, err := parsePlaylistID(req.GetId())
	if err != nil {
		return nil, err
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	p, page, err := h.playlistService.Get(ctx, viewerID(ctx), id, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.GetPlaylistResponse{
		Playlist:   toProtoPlaylist(p),
		Audios:     toProtoAudioPage(page),
		NextCursor: idString(page.NextCursor),
		Total:      int32(page.Total),
	}, nil
}

func (h *PostHandler) AddPlaylistAudio(ctx context.Context, req *postv1.AddPlaylistAudioRequest) (*emptypb.Empty, error) {
	return h.withPlaylistAudio(ctx, req.GetPlaylistId(), req.GetAudioId(), h.playlistService.Add)
}

func (h *PostHandler) RemovePlaylistAudio(ctx context.Context, req *postv1.RemovePlaylistAudioRequest) (*emptypb.Empty, error) {
	return h.withPlaylistAudio(ctx, req.GetPlaylistId(), req.GetAudioId(), h.playlistService.Remove)
}

func (h *PostHandler) withPlaylistAudio(ctx context.Context, rawPlaylist, rawAudio string, do func(ctx context.Context, requesterID, id, audioID uuid.UUID) error) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parsePlaylistID(rawPlaylist)
	if err != nil {
		return nil, err
	}

	audioID, err := parseAudioID(rawAudio)
	if err != nil {
		return nil, err
	}

	if err = do(ctx, userID, id, audioID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func parseAudioID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, post.ErrInvalidAudio.Error())
	}

	return id, nil
}

func parsePlaylistID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid playlist id")
	}

	return id, nil
}

func fromProtoAudioNames(rawArtist, rawTitle string) (artist, title post.AudioName, err error) {
	if artist, err = post.NewAudioName(rawArtist); err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}

	if title, err = post.NewAudioName(rawTitle); err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}

	return artist, title, nil
}

func fromProtoAudioDraft(ownerID uuid.UUID, rawArtist, rawTitle string, seconds int32) (post.AudioDraft, error) {
	artist, title, err := fromProtoAudioNames(rawArtist, rawTitle)
	if err != nil {
		return post.AudioDraft{}, err
	}

	duration, err := post.NewAudioDuration(int(seconds))
	if err != nil {
		return post.AudioDraft{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return post.AudioDraft{OwnerID: ownerID, Artist: artist, Title: title, Duration: duration}, nil
}

// fromProtoAudios reads the songs a post attaches.
func fromProtoAudios(raw []string) (post.Audios, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := parseAudioID(s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	audios, err := post.NewAudios(ids)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return audios, nil
}

func toProtoAudioPage(page service.AudioPage) []*postv1.Audio {
	out := make([]*postv1.Audio, len(page.Audios))
	for i, a := range page.Audios {
		out[i] = toProtoAudio(a, page.Added[a.ID()])
	}

	return out
}

func toProtoAudio(a *post.Audio, added bool) *postv1.Audio {
	source := a.Source()

	return &postv1.Audio{
		Id:        a.ID().String(),
		OwnerId:   a.OwnerID().String(),
		Kind:      source.Kind().String(),
		Link:      source.Link(),
		Cover:     source.Cover(),
		Artist:    a.Artist().String(),
		Title:     a.Title().String(),
		Duration:  int32(a.Duration().Seconds()),
		CreatedAt: timestamppb.New(a.CreatedAt()),
		Added:     added,
	}
}

func toProtoPlaylist(p *post.Playlist) *postv1.Playlist {
	return &postv1.Playlist{
		Id:        p.ID().String(),
		OwnerId:   p.OwnerID().String(),
		Title:     p.Title().String(),
		CreatedAt: timestamppb.New(p.CreatedAt()),
	}
}
