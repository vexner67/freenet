package rest

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type Rooms interface {
	AuthorizeMedia(context.Context, string) error
	Create(context.Context, string, model.RoomDraft) (model.Room, error)
	Get(context.Context, string, string) (model.Room, error)
	List(context.Context, string, model.RoomQuery) (model.RoomPage, error)
	Update(context.Context, string, string, model.RoomSettings) (model.Room, error)
	Close(context.Context, string, string) error
	Join(context.Context, string, string) (model.RoomJoined, error)
	Leave(context.Context, string, string) error
	Heartbeat(context.Context, string, string) error
	Token(context.Context, string, string) (model.RoomMedia, error)
	Chat(context.Context, string, string, string, string) (model.RoomChat, error)
	Playback(context.Context, string, string, model.RoomPlayback) (model.RoomPlayback, error)
	Kick(context.Context, string, string, string) error
	Mute(context.Context, string, string, string) error
	Invite(context.Context, string, string, string) error
	Current(context.Context, string, []string) (map[string]model.RoomSummary, error)
	Subscribe(context.Context, string) (model.RoomEvents, error)
}

type roomSettingsJSON struct {
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Access  string `json:"access"`
	Control string `json:"control"`
}

func (s roomSettingsJSON) model() model.RoomSettings {
	return model.RoomSettings{Title: s.Title, Kind: s.Kind, Access: s.Access, Control: s.Control}
}

type roomQueueJSON struct {
	Kind    string `json:"kind"`
	MediaID string `json:"mediaId"`
	Title   string `json:"title"`
}
type roomPlaybackJSON struct {
	Kind      string          `json:"kind"`
	MediaID   string          `json:"mediaId"`
	Playing   bool            `json:"playing"`
	Position  float64         `json:"position"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
	Version   uint64          `json:"version"`
	Queue     []roomQueueJSON `json:"queue"`
}

func toRoomPlaybackJSON(p model.RoomPlayback) roomPlaybackJSON {
	result := roomPlaybackJSON{Kind: p.Kind, MediaID: p.MediaID, Playing: p.Playing, Position: p.Position, Version: p.Version, Queue: make([]roomQueueJSON, 0, len(p.Queue))}
	if !p.UpdatedAt.IsZero() {
		result.UpdatedAt = p.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	for _, q := range p.Queue {
		result.Queue = append(result.Queue, roomQueueJSON{Kind: q.Kind, MediaID: q.MediaID, Title: q.Title})
	}
	return result
}
func (p roomPlaybackJSON) model() model.RoomPlayback {
	result := model.RoomPlayback{Kind: p.Kind, MediaID: p.MediaID, Playing: p.Playing, Position: p.Position, Version: p.Version}
	for _, q := range p.Queue {
		result.Queue = append(result.Queue, model.RoomQueueItem{Kind: q.Kind, MediaID: q.MediaID, Title: q.Title})
	}
	return result
}

type roomMemberJSON struct {
	User     authorJSON `json:"user"`
	JoinedAt string     `json:"joinedAt"`
}
type roomJSON struct {
	Friends int    `json:"friends"`
	ID      string `json:"id"`
	OwnerID string `json:"ownerId"`
	ClubID  string `json:"clubId"`
	roomSettingsJSON
	Permanent  bool             `json:"permanent"`
	CreatedAt  string           `json:"createdAt"`
	Members    []roomMemberJSON `json:"members"`
	Playback   roomPlaybackJSON `json:"playback"`
	MaxMembers int              `json:"maxMembers"`
}

func toRoomJSON(room model.Room, people map[string]model.Author) roomJSON {
	result := roomJSON{Friends: room.Friends, ID: room.ID, OwnerID: room.OwnerID, ClubID: room.ClubID, roomSettingsJSON: roomSettingsJSON{Title: room.Title, Kind: room.Kind, Access: room.Access, Control: room.Control}, Permanent: room.Permanent, CreatedAt: room.CreatedAt.UTC().Format(time.RFC3339), Members: make([]roomMemberJSON, 0, len(room.Members)), Playback: toRoomPlaybackJSON(room.Playback), MaxMembers: room.MaxMembers}
	for _, m := range room.Members {
		if user, ok := people[m.UserID]; ok {
			result.Members = append(result.Members, roomMemberJSON{User: toAuthorJSON(user), JoinedAt: m.JoinedAt.UTC().Format(time.RFC3339)})
		}
	}
	return result
}

type roomChatJSON struct {
	ID        string     `json:"id"`
	Author    authorJSON `json:"author"`
	Text      string     `json:"text"`
	Sticker   string     `json:"sticker"`
	CreatedAt string     `json:"createdAt"`
}

func toRoomChatJSON(c model.RoomChat, author authorJSON) roomChatJSON {
	return roomChatJSON{ID: c.ID, Author: author, Text: c.Text, Sticker: strings.ReplaceAll(c.StickerID, "-", ""), CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339)}
}

type roomMediaJSON struct {
	URL       string `json:"url"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

func toRoomMediaJSON(m model.RoomMedia) roomMediaJSON {
	return roomMediaJSON{URL: m.URL, Token: m.Token, ExpiresAt: m.ExpiresAt.UTC().Format(time.RFC3339)}
}
func (a *api) requireRooms(w http.ResponseWriter, r *http.Request) bool {
	if a.rooms != nil {
		return true
	}
	a.writeError(w, r, failure.New(failure.Unavailable, "rooms are unavailable"))
	return false
}
func roomPeople(rows []model.Room) []string {
	ids := make([]string, 0)
	for _, r := range rows {
		for _, m := range r.Members {
			ids = append(ids, m.UserID)
		}
	}
	return ids
}
func (a *api) writeRoom(w http.ResponseWriter, r *http.Request, code int, room model.Room) {
	people, err := a.authors(r, roomPeople([]model.Room{room}))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, code, struct {
		Room roomJSON `json:"room"`
	}{toRoomJSON(room, people)})
}
func (a *api) getRoom(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	room, err := a.rooms.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	a.writeRoom(w, r, http.StatusOK, room)
}
func (a *api) listRooms(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	query := model.RoomQuery{Limit: limit, Cursor: r.URL.Query().Get("cursor")}
	switch {
	case r.PathValue("nickname") != "":
		user, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		query.OwnerID = user.ID
	case r.PathValue("ref") != "":
		club, err := a.clubs.Get(r.Context(), token, r.PathValue("ref"))
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		query.ClubID = club.Club.ID
	default:
		if token == "" {
			a.writeError(w, r, failure.New(failure.Unauthenticated, "missing access token"))
			return
		}
	}
	page, err := a.rooms.List(r.Context(), token, query)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	people, err := a.authors(r, roomPeople(page.Rooms))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := struct {
		Rooms      []roomJSON `json:"rooms"`
		NextCursor string     `json:"nextCursor"`
	}{Rooms: make([]roomJSON, 0, len(page.Rooms)), NextCursor: page.NextCursor}
	for _, room := range page.Rooms {
		response.Rooms = append(response.Rooms, toRoomJSON(room, people))
	}
	writeJSON(w, http.StatusOK, response)
}
func (a *api) createRoom(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	var request struct {
		roomSettingsJSON
		ClubID    string `json:"clubId"`
		Permanent bool   `json:"permanent"`
	}
	if err = decode(w, r, &request); err != nil {
		a.writeError(w, r, err)
		return
	}
	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !me.ProfileCompleted {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "fill in your profile before creating a room"))
		return
	}
	if request.Kind == "" {
		request.Kind = "talk"
	}
	if request.Access == "" {
		request.Access = "everyone"
	}
	if request.Control == "" {
		request.Control = "owner"
	}
	result, err := a.rooms.Create(r.Context(), token, model.RoomDraft{RoomSettings: request.model(), ClubID: request.ClubID, Permanent: request.Permanent})
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	a.writeRoom(w, r, http.StatusCreated, result)
}
func (a *api) updateRoom(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	var request struct {
		Title   *string `json:"title"`
		Kind    *string `json:"kind"`
		Access  *string `json:"access"`
		Control *string `json:"control"`
	}
	if err = decode(w, r, &request); err != nil {
		a.writeError(w, r, err)
		return
	}
	current, err := a.rooms.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	settings := current.RoomSettings
	if request.Title != nil {
		settings.Title = *request.Title
	}
	if request.Kind != nil {
		settings.Kind = *request.Kind
	}
	if request.Access != nil {
		settings.Access = *request.Access
	}
	if request.Control != nil {
		settings.Control = *request.Control
	}
	result, err := a.rooms.Update(r.Context(), token, current.ID, settings)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	a.writeRoom(w, r, http.StatusOK, result)
}
func (a *api) joinRoom(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	joined, err := a.rooms.Join(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	ids := roomPeople([]model.Room{joined.Room})
	for _, c := range joined.Chat {
		ids = append(ids, c.AuthorID)
	}
	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	response := struct {
		Room       roomJSON       `json:"room"`
		Chat       []roomChatJSON `json:"chat"`
		MutedIDs   []string       `json:"mutedIds"`
		Media      roomMediaJSON  `json:"media"`
		ServerTime string         `json:"serverTime"`
	}{Room: toRoomJSON(joined.Room, people), Chat: make([]roomChatJSON, 0, len(joined.Chat)), MutedIDs: joined.MutedIDs, Media: toRoomMediaJSON(joined.Media), ServerTime: joined.ServerTime.UTC().Format(time.RFC3339Nano)}
	if response.MutedIDs == nil {
		response.MutedIDs = []string{}
	}
	for _, c := range joined.Chat {
		if author, ok := people[c.AuthorID]; ok {
			response.Chat = append(response.Chat, toRoomChatJSON(c, toAuthorJSON(author)))
		}
	}
	w.Header().Set("Date", joined.ServerTime.UTC().Format(http.TimeFormat))
	writeJSON(w, http.StatusOK, response)
}
func (a *api) roomToken(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	media, err := a.rooms.Token(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Media roomMediaJSON `json:"media"`
	}{toRoomMediaJSON(media)})
}
func (a *api) roomChat(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	var request struct {
		Text    string `json:"text"`
		Sticker string `json:"sticker"`
	}
	if err = decode(w, r, &request); err != nil {
		a.writeError(w, r, err)
		return
	}
	if err = a.checkSticker(r, token, request.Sticker); err != nil {
		a.writeError(w, r, err)
		return
	}
	chat, err := a.rooms.Chat(r.Context(), token, r.PathValue("id"), request.Text, request.Sticker)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	people, err := a.authors(r, []string{chat.AuthorID})
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Message roomChatJSON `json:"message"`
	}{toRoomChatJSON(chat, toAuthorJSON(people[chat.AuthorID]))})
}
func (a *api) roomPlayback(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	if !a.requireRooms(w, r) {
		return
	}
	var request roomPlaybackJSON
	if err = decode(w, r, &request); err != nil {
		a.writeError(w, r, err)
		return
	}
	playback, err := a.rooms.Playback(r.Context(), token, r.PathValue("id"), request.model())
	if err != nil {
		if f := failure.From(err); f.Kind == failure.Conflict {
			if current, getErr := a.rooms.Get(r.Context(), token, r.PathValue("id")); getErr == nil {
				writeJSON(w, http.StatusConflict, struct {
					Error    errorDetails     `json:"error"`
					Playback roomPlaybackJSON `json:"playback"`
				}{errorDetails{Code: "conflict", Message: f.Message}, toRoomPlaybackJSON(current.Playback)})
				return
			}
		}
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Playback roomPlaybackJSON `json:"playback"`
	}{toRoomPlaybackJSON(playback)})
}
func (a *api) roomAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerToken(r)
		if action == "leave" && r.Header.Get("Authorization") == "" {
			// sendBeacon cannot set an Authorization header. The token is in the
			// body, never the URL, and is verified by the room service as usual.
			var request struct {
				Token string `json:"token"`
			}
			if err = decode(w, r, &request); err == nil {
				token = request.Token
				if token == "" {
					err = failure.New(failure.Unauthenticated, "missing access token")
				}
			}
		}
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		if !a.requireRooms(w, r) {
			return
		}
		id := r.PathValue("id")
		switch action {
		case "close":
			err = a.rooms.Close(r.Context(), token, id)
		case "leave":
			err = a.rooms.Leave(r.Context(), token, id)
		case "heartbeat":
			err = a.rooms.Heartbeat(r.Context(), token, id)
		default:
			var request struct {
				UserID string `json:"userId"`
			}
			if err = decode(w, r, &request); err == nil {
				switch action {
				case "kick":
					err = a.rooms.Kick(r.Context(), token, id, request.UserID)
				case "mute":
					err = a.rooms.Mute(r.Context(), token, id, request.UserID)
				case "invite":
					err = a.rooms.Invite(r.Context(), token, id, request.UserID)
				}
			}
		}
		if err != nil {
			a.writeError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
func registerRoomRoutes(mux *http.ServeMux, a *api) {
	mux.HandleFunc("GET /api/v1/rooms", a.listRooms)
	mux.HandleFunc("POST /api/v1/rooms", a.createRoom)
	mux.HandleFunc("GET /api/v1/rooms/{id}", a.getRoom)
	mux.HandleFunc("PATCH /api/v1/rooms/{id}", a.updateRoom)
	mux.HandleFunc("DELETE /api/v1/rooms/{id}", a.roomAction("close"))
	mux.HandleFunc("POST /api/v1/rooms/{id}/join", a.joinRoom)
	mux.HandleFunc("POST /api/v1/rooms/{id}/token", a.roomToken)
	mux.HandleFunc("POST /api/v1/rooms/{id}/chat", a.roomChat)
	mux.HandleFunc("PUT /api/v1/rooms/{id}/playback", a.roomPlayback)
	for _, action := range []string{"leave", "heartbeat", "kick", "mute", "invite"} {
		mux.HandleFunc("POST /api/v1/rooms/{id}/"+action, a.roomAction(action))
	}
	mux.HandleFunc("GET /api/v1/users/{nickname}/rooms", a.listRooms)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/rooms", a.listRooms)
}

// CurrentRooms is batched once per page, never once per person.
func (a *api) currentRoomsForPeople(r *http.Request, people []publicProfileResponse) error {
	if a.rooms == nil || len(people) == 0 {
		return nil
	}
	token, err := optionalBearerToken(r)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(people))
	for _, person := range people {
		if !person.Deactivated && !person.PageHidden {
			ids = append(ids, person.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rooms, err := a.rooms.Current(r.Context(), token, ids)
	if err != nil {
		return err
	}
	for i, person := range people {
		if room, ok := rooms[person.ID]; ok {
			people[i].Room = &roomSummaryJSON{ID: room.ID, Title: room.Title}
		}
	}
	return nil
}
