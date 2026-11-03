package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAudio(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	id := uuid.Must(uuid.NewV7()).String()

	added, err := s.client.AddAudio(s.as(t, ann), &postv1.AddAudioRequest{Id: id, Artist: " Кино ", Title: "Группа крови", Duration: 285})
	if err != nil {
		t.Fatalf("AddAudio: %v", err)
	}
	song := added.GetAudio()
	if song.GetId() != id || song.GetKind() != "file" || song.GetArtist() != "Кино" || song.GetDuration() != 285 || !song.GetAdded() {
		t.Fatalf("AddAudio = %v", song)
	}

	for name, req := range map[string]*postv1.AddAudioRequest{
		"a bad id":         {Id: "x"},
		"a minus duration": {Id: uuid.NewString(), Duration: -1},
	} {
		if _, err = s.client.AddAudio(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("AddAudio with %s: %v", name, err)
		}
	}
	if _, err = s.client.AddAudio(context.Background(), &postv1.AddAudioRequest{Id: uuid.NewString()}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("AddAudio without a token: %v", err)
	}

	// The same link is the same song for everyone.
	track, err := s.client.AddAudioTrack(s.as(t, ann), &postv1.AddAudioTrackRequest{Kind: "soundcloud", Link: "https://m.soundcloud.com/artist/song?si=1", Title: "Песня"})
	if err != nil || track.GetAudio().GetLink() != "https://soundcloud.com/artist/song" {
		t.Fatalf("AddAudioTrack = %v, %v", track, err)
	}
	again, err := s.client.AddAudioTrack(s.as(t, bob), &postv1.AddAudioTrackRequest{Kind: "soundcloud", Link: "https://soundcloud.com/artist/song"})
	if err != nil || again.GetAudio().GetId() != track.GetAudio().GetId() {
		t.Fatalf("AddAudioTrack again = %v, %v", again, err)
	}
	for name, req := range map[string]*postv1.AddAudioTrackRequest{
		"a file":       {Kind: "file", Link: "https://soundcloud.com/a/b"},
		"another site": {Kind: "soundcloud", Link: "https://example.com/a/b"},
	} {
		if _, err = s.client.AddAudioTrack(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("AddAudioTrack with %s: %v", name, err)
		}
	}

	// Anyone reads a library; added is the viewer's.
	list, err := s.client.ListAudios(s.as(t, bob), &postv1.ListAudiosRequest{OwnerId: ann.String()})
	if err != nil || list.GetTotal() != 2 || len(list.GetAudios()) != 2 {
		t.Fatalf("ListAudios = %v, %v", list, err)
	}
	for _, a := range list.GetAudios() {
		if a.GetAdded() != (a.GetId() == track.GetAudio().GetId()) {
			t.Errorf("ListAudios for bob says added = %v of %s", a.GetAdded(), a.GetId())
		}
	}
	if _, err = s.client.ListAudios(context.Background(), &postv1.ListAudiosRequest{OwnerId: ann.String()}); err != nil {
		t.Errorf("ListAudios without a token: %v", err)
	}

	if _, err = s.client.KeepAudio(s.as(t, bob), &postv1.KeepAudioRequest{AudioId: id}); err != nil {
		t.Fatalf("KeepAudio: %v", err)
	}
	if _, err = s.client.KeepAudio(s.as(t, bob), &postv1.KeepAudioRequest{AudioId: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Errorf("KeepAudio of none: %v", err)
	}
	found, err := s.client.SearchAudios(context.Background(), &postv1.SearchAudiosRequest{Query: "кров"})
	if err != nil || len(found.GetAudios()) != 1 {
		t.Fatalf("SearchAudios = %v, %v", found, err)
	}

	// Only the uploader edits a file.
	if _, err = s.client.EditAudio(s.as(t, bob), &postv1.EditAudioRequest{Id: id, Title: "Моё"}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("EditAudio by another: %v", err)
	}
	edited, err := s.client.EditAudio(s.as(t, ann), &postv1.EditAudioRequest{Id: id, Artist: "Кино", Title: "Звезда по имени Солнце"})
	if err != nil || edited.GetAudio().GetTitle() != "Звезда по имени Солнце" {
		t.Fatalf("EditAudio = %v, %v", edited, err)
	}

	// Listening, the song of the page.
	if _, err = s.client.ListenAudio(s.as(t, ann), &postv1.ListenAudioRequest{AudioId: id}); err != nil {
		t.Fatalf("ListenAudio: %v", err)
	}
	if _, err = s.client.SetPageAudio(s.as(t, ann), &postv1.SetPageAudioRequest{AudioId: track.GetAudio().GetId()}); err != nil {
		t.Fatalf("SetPageAudio: %v", err)
	}
	music, err := s.client.GetMusic(context.Background(), &postv1.GetMusicRequest{UserId: ann.String()})
	if err != nil || music.GetListening().GetId() != id || music.GetPageAudio().GetId() != track.GetAudio().GetId() || music.GetCount() != 2 {
		t.Fatalf("GetMusic = %v, %v", music, err)
	}
	if _, err = s.client.ListenAudio(s.as(t, ann), &postv1.ListenAudioRequest{}); err != nil {
		t.Fatalf("ListenAudio to stop: %v", err)
	}
	if music, err = s.client.GetMusic(context.Background(), &postv1.GetMusicRequest{UserId: ann.String()}); err != nil || music.GetListening() != nil {
		t.Fatalf("GetMusic after stop = %v, %v", music, err)
	}

	// A post of songs alone.
	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{AudioIds: []string{id}})
	if err != nil || len(created.GetPost().GetAudioIds()) != 1 {
		t.Fatalf("CreatePost with a song = %v, %v", created, err)
	}
	if _, err = s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{AudioIds: []string{id, id}}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("CreatePost with a song twice: %v", err)
	}

	// Playlists.
	pl, err := s.client.CreatePlaylist(s.as(t, ann), &postv1.CreatePlaylistRequest{Title: "Дорога"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	plID := pl.GetPlaylist().GetId()
	if _, err = s.client.CreatePlaylist(s.as(t, ann), &postv1.CreatePlaylistRequest{Title: " "}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("CreatePlaylist without a title: %v", err)
	}
	if _, err = s.client.AddPlaylistAudio(s.as(t, bob), &postv1.AddPlaylistAudioRequest{PlaylistId: plID, AudioId: id}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("AddPlaylistAudio by another: %v", err)
	}
	if _, err = s.client.AddPlaylistAudio(s.as(t, ann), &postv1.AddPlaylistAudioRequest{PlaylistId: plID, AudioId: id}); err != nil {
		t.Fatalf("AddPlaylistAudio: %v", err)
	}
	got, err := s.client.GetPlaylist(context.Background(), &postv1.GetPlaylistRequest{Id: plID})
	if err != nil || got.GetPlaylist().GetTitle() != "Дорога" || got.GetTotal() != 1 {
		t.Fatalf("GetPlaylist = %v, %v", got, err)
	}
	lists, err := s.client.ListPlaylists(context.Background(), &postv1.ListPlaylistsRequest{OwnerId: ann.String()})
	if err != nil || lists.GetTotal() != 1 {
		t.Fatalf("ListPlaylists = %v, %v", lists, err)
	}

	// Deleted, the song leaves the libraries, playlists and posts.
	if _, err = s.client.DeleteAudio(s.as(t, bob), &postv1.DeleteAudioRequest{Id: id}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("DeleteAudio by another: %v", err)
	}
	if _, err = s.client.DeleteAudio(s.as(t, ann), &postv1.DeleteAudioRequest{Id: id}); err != nil {
		t.Fatalf("DeleteAudio: %v", err)
	}
	if list, err = s.client.ListAudios(context.Background(), &postv1.ListAudiosRequest{OwnerId: bob.String()}); err != nil || list.GetTotal() != 1 {
		t.Fatalf("ListAudios of bob after delete = %v, %v", list, err)
	}
	if got, err = s.client.GetPlaylist(context.Background(), &postv1.GetPlaylistRequest{Id: plID}); err != nil || got.GetTotal() != 0 {
		t.Fatalf("GetPlaylist after delete = %v, %v", got, err)
	}
	byID, err := s.client.GetAudios(context.Background(), &postv1.GetAudiosRequest{Ids: []string{id, track.GetAudio().GetId()}})
	if err != nil || len(byID.GetAudios()) != 1 {
		t.Fatalf("GetAudios = %v, %v", byID, err)
	}
	// Added is the viewer's here too: bob keeps the track he added.
	if byID, err = s.client.GetAudios(s.as(t, bob), &postv1.GetAudiosRequest{Ids: []string{track.GetAudio().GetId()}}); err != nil || !byID.GetAudios()[0].GetAdded() {
		t.Fatalf("GetAudios of bob = %v, %v", byID, err)
	}

	if _, err = s.client.DeletePlaylist(s.as(t, ann), &postv1.DeletePlaylistRequest{Id: plID}); err != nil {
		t.Fatalf("DeletePlaylist: %v", err)
	}
	if _, err = s.client.GetPlaylist(context.Background(), &postv1.GetPlaylistRequest{Id: plID}); status.Code(err) != codes.NotFound {
		t.Errorf("GetPlaylist deleted: %v", err)
	}
}
