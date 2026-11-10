package rest_test

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
	"net/http"
	"testing"
	"time"
)

type fakeRooms struct {
	rest.Rooms
	token, action string
	draft         model.RoomDraft
	settings      model.RoomSettings
	playback      model.RoomPlayback
	stale         bool
}

func (f *fakeRooms) view() model.Room {
	return model.Room{ID: "r1", OwnerID: "u1", RoomSettings: model.RoomSettings{Title: "Friends", Kind: "talk", Access: "everyone", Control: "owner"}, MaxMembers: 5, CreatedAt: time.Now().UTC(), Playback: f.playback}
}
func (f *fakeRooms) Get(_ context.Context, token, id string) (model.Room, error) {
	f.token = token
	return f.view(), nil
}
func (f *fakeRooms) Create(_ context.Context, token string, draft model.RoomDraft) (model.Room, error) {
	f.token = token
	f.draft = draft
	return f.view(), nil
}
func (f *fakeRooms) Update(_ context.Context, token, id string, settings model.RoomSettings) (model.Room, error) {
	f.token = token
	f.settings = settings
	return f.view(), nil
}
func (f *fakeRooms) List(_ context.Context, token string, q model.RoomQuery) (model.RoomPage, error) {
	f.token = token
	return model.RoomPage{Rooms: []model.Room{f.view()}}, nil
}
func (f *fakeRooms) Join(_ context.Context, token, id string) (model.RoomJoined, error) {
	f.token = token
	return model.RoomJoined{Room: f.view(), Media: model.RoomMedia{URL: "wss://example.com/rtc", Token: "private", ExpiresAt: time.Now().Add(time.Minute)}, ServerTime: time.Now()}, nil
}
func (f *fakeRooms) Token(_ context.Context, token, id string) (model.RoomMedia, error) {
	f.token = token
	return model.RoomMedia{Token: "renewed"}, nil
}
func (f *fakeRooms) Chat(_ context.Context, token, id, text, sticker string) (model.RoomChat, error) {
	return model.RoomChat{ID: "c1", AuthorID: "u1", Text: text, CreatedAt: time.Now()}, nil
}
func (f *fakeRooms) Playback(_ context.Context, token, id string, p model.RoomPlayback) (model.RoomPlayback, error) {
	if f.stale {
		return model.RoomPlayback{}, failure.New(failure.Conflict, "room playback has changed")
	}
	f.playback = p
	return p, nil
}
func (f *fakeRooms) Close(context.Context, string, string) error { f.action = "close"; return nil }
func (f *fakeRooms) Leave(_ context.Context, token, id string) error {
	f.token = token
	f.action = "leave"
	return nil
}
func (f *fakeRooms) Heartbeat(context.Context, string, string) error {
	f.action = "heartbeat"
	return nil
}
func (f *fakeRooms) Kick(context.Context, string, string, string) error {
	f.action = "kick"
	return nil
}
func (f *fakeRooms) Mute(context.Context, string, string, string) error {
	f.action = "mute"
	return nil
}
func (f *fakeRooms) Invite(context.Context, string, string, string) error {
	f.action = "invite"
	return nil
}
func TestRoomRoutes(t *testing.T) {
	t.Parallel()
	h := newHarness(rest.Config{})
	h.users.profile = ann
	f := &fakeRooms{}
	h.handler = mustServer(rest.Config{RequestTimeout: time.Second}, rest.Backends{Auth: h.auth, Users: h.users, Rooms: f, Clubs: h.clubs}, prometheus.NewRegistry()).Handler()
	auth := []string{"Authorization", "Bearer tok"}
	for _, call := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/v1/rooms", "", 200}, {"POST", "/api/v1/rooms", `{"title":"Friends"}`, 201},
		{"GET", "/api/v1/rooms/r1", "", 200}, {"PATCH", "/api/v1/rooms/r1", `{"title":"Other"}`, 200},
		{"POST", "/api/v1/rooms/r1/join", "", 200}, {"POST", "/api/v1/rooms/r1/token", "", 200},
		{"POST", "/api/v1/rooms/r1/chat", `{"text":"hello"}`, 201},
		{"PUT", "/api/v1/rooms/r1/playback", `{"kind":"youtube","mediaId":"abcdefghijk","playing":true,"position":0,"version":0,"queue":[]}`, 200},
		{"POST", "/api/v1/rooms/r1/heartbeat", "", 204}, {"POST", "/api/v1/rooms/r1/leave", "", 204},
		{"POST", "/api/v1/rooms/r1/kick", `{"userId":"u2"}`, 204}, {"POST", "/api/v1/rooms/r1/mute", `{"userId":"u2"}`, 204},
		{"POST", "/api/v1/rooms/r1/invite", `{"userId":"u2"}`, 204}, {"DELETE", "/api/v1/rooms/r1", "", 204},
		{"GET", "/api/v1/users/ann/rooms", "", 200}, {"GET", "/api/v1/clubs/cats/rooms", "", 200},
	} {
		w := h.do(call.method, call.path, call.body, auth...)
		if w.Code != call.code {
			t.Fatalf("%s %s: %d %s", call.method, call.path, w.Code, w.Body.String())
		}
	}
	if w := h.do("GET", "/api/v1/rooms/r1", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if f.token != "" {
		t.Fatal("guest received an identity")
	}
	if w := h.do("GET", "/api/v1/rooms", ""); w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	if w := h.do("POST", "/api/v1/rooms/r1/join", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := h.do("POST", "/api/v1/rooms/r1/leave", `{"token":"beacon"}`); w.Code != 204 || f.token != "beacon" {
		t.Fatal(w.Code, f.token)
	}
	f.stale = true
	if w := h.do("PUT", "/api/v1/rooms/r1/playback", `{"version":0}`, auth...); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}
