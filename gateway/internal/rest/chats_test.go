package rest_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

var chatTime = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

// fakeChats: ann (u1) created the chat c1 with bob (u2) and cat (u3).
type fakeChats struct {
	token string
	// calls are what was asked, as "verb args".
	calls []string
	// none: the caller has no chats.
	none bool
	err  error
	// What the last message sent, the last edit and the last page asked for were.
	draft model.MessageDraft
	edit  model.MessageEdit
	at    model.MessagesAt
}

func (f *fakeChats) did(token string, call ...string) {
	f.token = token
	f.calls = append(f.calls, strings.Join(call, " "))
}

func (f *fakeChats) chat() model.Chat {
	return model.Chat{
		ID: "c1", Title: "Поход", OwnerID: "u1", CreatedAt: chatTime, MemberCount: 3, State: model.ChatStateActive, Unread: true, PhotoID: "cp0",
		LastMessage: &model.ChatMessage{ID: "m3", ChatID: "c1", SenderID: "u2", Text: "иду", CreatedAt: chatTime, Unread: true},
	}
}

func (f *fakeChats) Create(_ context.Context, token, title string, memberIDs []string) (model.Chat, error) {
	f.did(token, "create", title, strings.Join(memberIDs, ","))
	return f.chat(), f.err
}

func (f *fakeChats) Get(_ context.Context, token, id string) (model.ChatView, error) {
	f.token = token
	if id != "c1" {
		return model.ChatView{}, failure.New(failure.NotFound, "chat not found")
	}

	return model.ChatView{
		Chat:    f.chat(),
		Members: []model.ChatMember{{UserID: "u1"}, {UserID: "u2"}, {UserID: "gone"}},
		Pinned:  &model.ChatMessage{ID: "m2", ChatID: "c1", SenderID: "u2", Text: "встречаемся в 9", CreatedAt: chatTime, DocumentIDs: []string{"d1"}},
	}, nil
}

func (f *fakeChats) List(_ context.Context, token, cursor string, _ int) (model.ChatPage, error) {
	f.did(token, "list", cursor)
	if f.none {
		return model.ChatPage{}, f.err
	}

	return model.ChatPage{Chats: []model.Chat{f.chat()}}, f.err
}

func (f *fakeChats) Rename(_ context.Context, token, id, title string) error {
	f.did(token, "rename", id, title)
	return f.err
}

func (f *fakeChats) Pin(_ context.Context, token, id, messageID string) error {
	f.did(token, "pin", id, messageID)
	return f.err
}

func (f *fakeChats) Unpin(_ context.Context, token, id string) error {
	f.did(token, "unpin", id)
	return f.err
}

func (f *fakeChats) SetPhoto(_ context.Context, token, id, photoID string) error {
	f.did(token, "photo", id, photoID)
	return f.err
}

func (f *fakeChats) Invite(_ context.Context, token, id string, userIDs []string) error {
	f.did(token, "invite", id, strings.Join(userIDs, ","))
	return f.err
}

func (f *fakeChats) Kick(_ context.Context, token, id, userID string) error {
	f.did(token, "kick", id, userID)
	return f.err
}

func (f *fakeChats) Leave(_ context.Context, token, id string) error {
	f.did(token, "leave", id)
	return f.err
}

func (f *fakeChats) Return(_ context.Context, token, id string) error {
	f.did(token, "return", id)
	return f.err
}

func (f *fakeChats) Send(_ context.Context, token, id string, d model.MessageDraft) (model.ChatMessage, error) {
	f.did(token, "send", id, d.Text, strings.Join(d.PhotoIDs, ","))
	f.draft = d

	m := model.ChatMessage{
		ID: "m9", ChatID: id, SenderID: "u1", Text: d.Text, PhotoIDs: d.PhotoIDs, CreatedAt: chatTime, DeletableUntil: chatTime, EditableUntil: chatTime,
		PostID: d.PostID, SharedPhotoID: d.SharedPhotoID, Sticker: d.Sticker, DocumentIDs: d.DocumentIDs,
	}
	for range d.ForwardIDs {
		m.Forwarded = append(m.Forwarded, model.ForwardedMessage{SenderID: "u2", Text: "дальше", CreatedAt: chatTime})
	}
	// A reply to "gone" quotes one whose author's page is gone; any other,
	// bob's words.
	if d.ReplyToID != "" {
		sender := "u2"
		if d.ReplyToID == "gone" {
			sender = "gone"
		}
		m.ReplyTo = &model.Quote{ID: d.ReplyToID, SenderID: sender, Text: "кто берёт палатку?", Found: true}
		m.Reactions = []model.Reaction{{UserID: "u2", Reaction: "fire"}}
	}

	return m, f.err
}

// History: ann created the chat, bob wrote, somebody whose page is gone
// wrote too, and ann removed cat.
func (f *fakeChats) History(_ context.Context, token, id string, at model.MessagesAt, _ int) (model.ChatMessagePage, error) {
	f.did(token, "history", id, at.Cursor, at.After)
	f.at = at

	return model.ChatMessagePage{
		Messages: []model.ChatMessage{
			{ID: "m1", ChatID: id, SenderID: "u1", Action: "create", Text: "Поход", CreatedAt: chatTime},
			{ID: "m2", ChatID: id, SenderID: "u2", Text: "иду", CreatedAt: chatTime, DeletableUntil: chatTime, EditableUntil: chatTime, Unread: true},
			{ID: "m3", ChatID: id, SenderID: "gone", Text: "и я", CreatedAt: chatTime},
			{ID: "m4", ChatID: id, SenderID: "u1", Action: "kick", TargetID: "u3", CreatedAt: chatTime},
			{ID: "m5", ChatID: id, SenderID: "u1", CreatedAt: chatTime, DeletedAt: chatTime},
		},
		NextCursor: "m1",
	}, f.err
}

func (f *fakeChats) Message(_ context.Context, token, id string) (model.ChatMessage, error) {
	f.did(token, "message", id)
	return model.ChatMessage{ID: id, ChatID: "c1", SenderID: "u2", Text: "иду, конечно", CreatedAt: chatTime}, f.err
}

func (f *fakeChats) Edit(_ context.Context, token, id string, e model.MessageEdit) (model.ChatMessage, error) {
	f.did(token, "edit", id, e.Text)
	f.edit = e

	return model.ChatMessage{ID: id, ChatID: "c1", SenderID: "u1", Text: e.Text, PhotoIDs: e.AddPhotoIDs, CreatedAt: chatTime, EditedAt: chatTime, EditableUntil: chatTime}, f.err
}

func (f *fakeChats) React(_ context.Context, token, id, reaction string) error {
	f.did(token, "react", id, reaction)
	return f.err
}

// Search: bob said it in the chat c1, carrying on what cat wrote.
func (f *fakeChats) Search(_ context.Context, token, chatID, query, cursor string, _ int) (model.FoundChatMessages, error) {
	f.did(token, "search", chatID, query, cursor)
	if f.none {
		return model.FoundChatMessages{}, f.err
	}

	return model.FoundChatMessages{Messages: []model.ChatMessage{{
		ID: "m4", ChatID: "c1", SenderID: "u2", Text: "идём в поход", CreatedAt: chatTime,
		Forwarded: []model.ForwardedMessage{{SenderID: "u3", Text: "какой поход?", CreatedAt: chatTime}},
	}}}, f.err
}

func (f *fakeChats) Delete(_ context.Context, token, id string) error {
	f.did(token, "delete", id)
	return f.err
}

func (f *fakeChats) MarkRead(_ context.Context, token, id, upTo string) error {
	f.did(token, "read", id, upTo)
	return f.err
}

func (f *fakeChats) Typing(_ context.Context, token, id string) error {
	f.did(token, "typing", id)
	return f.err
}

type chatMessageJSON struct {
	ID             string           `json:"id"`
	Author         map[string]any   `json:"author"`
	Target         map[string]any   `json:"target"`
	Out            bool             `json:"out"`
	Text           string           `json:"text"`
	Action         string           `json:"action"`
	Photos         []map[string]any `json:"photos"`
	Deleted        bool             `json:"deleted"`
	Unread         bool             `json:"unread"`
	EditedAt       *string          `json:"editedAt"`
	DeletableUntil string           `json:"deletableUntil"`
	EditableUntil  string           `json:"editableUntil"`
}

type chatJSON struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	OwnerID     string           `json:"ownerId"`
	MemberCount int              `json:"memberCount"`
	State       string           `json:"state"`
	Unread      bool             `json:"unread"`
	LastMessage *chatMessageJSON `json:"lastMessage"`
	Members     []map[string]any `json:"members"`
	Photo       *struct {
		Small string `json:"small"`
	} `json:"photo"`
	Pinned *chatMessageJSON `json:"pinned"`
	// Error is what a refusal says instead.
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

const annAuth = "Bearer tok"

func TestCreateChat(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w := h.do("POST", "/api/v1/chats", `{"title":"Поход","members":["Bob","cat"]}`, "Authorization", annAuth)
	got := decode[chatJSON](t, w)

	if w.Code != http.StatusCreated || !slices.Equal(h.chats.calls, []string{"create Поход u2,u3"}) || h.chats.token != "tok" {
		t.Fatalf("%d %+v, calls %q", w.Code, got, h.chats.calls)
	}

	// The chat comes back with who is in it; a page that is gone is not shown.
	if got.ID != "c1" || got.Title != "Поход" || got.OwnerID != "u1" || got.MemberCount != 3 || got.State != "active" || len(got.Members) != 2 ||
		got.Members[1]["nickname"] != "bob" || got.LastMessage == nil || got.LastMessage.Author["nickname"] != "bob" {
		t.Fatalf("chat = %+v", got)
	}

	tests := []struct {
		name    string
		body    string
		setup   func(h harness)
		code    int
		message string
	}{
		{"an unknown nickname", `{"members":["bob","nobody"]}`, nil, http.StatusNotFound, "user not found"},
		{"someone who does not let", `{"members":["bob","cat"]}`, func(h harness) { h.users.uninvitable = "cat" }, http.StatusForbidden, "you cannot add this user to a chat"},
		{"an unfilled profile", `{"members":["bob","cat"]}`, func(h harness) { h.users.profile = model.Profile{} }, http.StatusBadRequest, "fill in your profile before writing messages"},
		{"too many people", `{"members":["` + strings.Repeat(`bob","`, 49) + `cat"]}`, nil, http.StatusConflict, "the chat is full"},
		{"the service refusing", `{"members":["bob"]}`, func(h harness) {
			h.chats.err = failure.New(failure.InvalidArgument, "a chat needs at least two other people")
		}, http.StatusBadRequest, "a chat needs at least two other people"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
			if tt.setup != nil {
				tt.setup(h)
			}

			w := h.do("POST", "/api/v1/chats", tt.body, "Authorization", annAuth)
			if got := decode[chatJSON](t, w); w.Code != tt.code || got.Error.Message != tt.message {
				t.Fatalf("%d %q, want %d %q", w.Code, got.Error.Message, tt.code, tt.message)
			}
		})
	}

	if w = newHarness(rest.Config{}).do("POST", "/api/v1/chats", `{"members":["bob","cat"]}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestChangeChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, method, path, body string
		call                     string
	}{
		{"rename", "PATCH", "/api/v1/chats/c1", `{"title":"Горы"}`, "rename c1 Горы"},
		{"add", "POST", "/api/v1/chats/c1/members", `{"members":["cat"]}`, "invite c1 u3"},
		{"remove", "DELETE", "/api/v1/chats/c1/members/bob", "", "kick c1 u2"},
		{"leave", "DELETE", "/api/v1/chats/c1/membership", "", "leave c1"},
		{"return", "PUT", "/api/v1/chats/c1/membership", "", "return c1"},
		{"pin", "PUT", "/api/v1/chats/c1/pin", `{"messageId":"m2"}`, "pin c1 m2"},
		{"unpin", "DELETE", "/api/v1/chats/c1/pin", "", "unpin c1"},
		{"photo", "PUT", "/api/v1/chats/c1/photo", "img", "photo c1 cp1"},
		{"no photo", "DELETE", "/api/v1/chats/c1/photo", "", "photo c1 "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			w := h.do(tt.method, tt.path, tt.body, "Authorization", annAuth)

			if got := decode[chatJSON](t, w); w.Code != http.StatusOK || got.ID != "c1" || !slices.Equal(h.chats.calls, []string{tt.call}) {
				t.Fatalf("%d %+v, calls %q", w.Code, got, h.chats.calls)
			}

			// What the message service refuses reaches the client as it is.
			h = newHarness(rest.Config{})
			h.chats.err = failure.New(failure.Conflict, "you are not in this chat")

			w = h.do(tt.method, tt.path, tt.body, "Authorization", annAuth)
			if got := decode[chatJSON](t, w); w.Code != http.StatusConflict || got.Error.Message != "you are not in this chat" {
				t.Fatalf("refused: %d %+v", w.Code, got)
			}

			if w = newHarness(rest.Config{}).do(tt.method, tt.path, tt.body); w.Code != http.StatusUnauthorized {
				t.Fatalf("without token: %d", w.Code)
			}
		})
	}

	h := newHarness(rest.Config{})
	h.users.uninvitable = "cat"

	if w := h.do("POST", "/api/v1/chats/c1/members", `{"members":["cat"]}`, "Authorization", annAuth); w.Code != http.StatusForbidden || len(h.chats.calls) != 0 {
		t.Fatalf("adding someone who does not let: %d, calls %q", w.Code, h.chats.calls)
	}

	if w := h.do("GET", "/api/v1/chats/c2", "", "Authorization", annAuth); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown chat: %d", w.Code)
	}
}

func TestChatPhotoAndPin(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/chats/c1", "", "Authorization", annAuth)
	got := decode[chatJSON](t, w)
	if w.Code != http.StatusOK || got.Photo == nil || got.Photo.Small != "/media/avatars/c1/cp0/small.jpg" {
		t.Fatalf("the photo of the chat: %d %+v", w.Code, got.Photo)
	}
	if got.Pinned == nil || got.Pinned.ID != "m2" || got.Pinned.Text != "встречаемся в 9" {
		t.Fatalf("pinned = %+v", got.Pinned)
	}

	// A new photo is shown before the old one goes.
	h = newHarness(rest.Config{})
	if w = h.do("PUT", "/api/v1/chats/c1/photo", "img", "Authorization", annAuth); w.Code != http.StatusOK || string(h.media.uploaded) != "img" {
		t.Fatalf("upload: %d", w.Code)
	}
	if !slices.Equal(h.media.chatPhotos, []string{"upload c1", "keep c1 cp1"}) {
		t.Fatalf("media did %q", h.media.chatPhotos)
	}

	// Refused, the chat keeps the photo it shows.
	h = newHarness(rest.Config{})
	h.chats.err = failure.New(failure.Conflict, "you are not in this chat")
	h.do("PUT", "/api/v1/chats/c1/photo", "img", "Authorization", annAuth)
	if !slices.Equal(h.media.chatPhotos, []string{"upload c1", "keep c1 cp0"}) {
		t.Fatalf("refused, media did %q", h.media.chatPhotos)
	}

	// Removed, nothing is kept.
	h = newHarness(rest.Config{})
	if w = h.do("DELETE", "/api/v1/chats/c1/photo", "", "Authorization", annAuth); w.Code != http.StatusOK || !slices.Equal(h.media.chatPhotos, []string{"keep c1 "}) {
		t.Fatalf("delete: %d, media did %q", w.Code, h.media.chatPhotos)
	}

	// Who is not in the chat stores nothing under its ID.
	h = newHarness(rest.Config{})
	if w = h.do("PUT", "/api/v1/chats/c2/photo", "img", "Authorization", annAuth); w.Code != http.StatusNotFound || len(h.media.chatPhotos) != 0 {
		t.Fatalf("a stranger: %d, media did %q", w.Code, h.media.chatPhotos)
	}
}

func TestChatMessages(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/chats/c1/messages?cursor=m7&limit=5", "", "Authorization", annAuth)
	page := decode[struct {
		Messages   []chatMessageJSON `json:"messages"`
		NextCursor string            `json:"nextCursor"`
	}](t, w)

	if w.Code != http.StatusOK || page.NextCursor != "m1" || !slices.Equal(h.chats.calls, []string{"history c1 m7 "}) {
		t.Fatalf("%d %+v, calls %q", w.Code, page, h.chats.calls)
	}

	// The message of a page that is gone is left out.
	if len(page.Messages) != 4 {
		t.Fatalf("messages = %+v", page.Messages)
	}

	created, said, kicked, taken := page.Messages[0], page.Messages[1], page.Messages[2], page.Messages[3]
	if created.Action != "create" || created.Text != "Поход" || !created.Out || created.Author["nickname"] != "ann" {
		t.Fatalf("created = %+v", created)
	}
	if said.Out || said.Author["nickname"] != "bob" || !said.Unread || said.DeletableUntil != "" || said.EditableUntil != "" {
		t.Fatalf("bob's = %+v", said)
	}
	if kicked.Action != "kick" || kicked.Target["nickname"] != "cat" {
		t.Fatalf("kicked = %+v", kicked)
	}
	if !taken.Deleted || taken.DeletableUntil != "" {
		t.Fatalf("taken back = %+v", taken)
	}

	h = newHarness(rest.Config{})
	if w = h.do("GET", "/api/v1/chats/c1/messages?after=m2", "", "Authorization", annAuth); w.Code != http.StatusOK || h.chats.calls[0] != "history c1  m2" {
		t.Fatalf("newer: %d, calls %q", w.Code, h.chats.calls)
	}

	h = newHarness(rest.Config{})
	w = h.do("POST", "/api/v1/chats/c1/messages", `{"text":"привет","photos":["ph1"]}`, "Authorization", annAuth)
	if got := decode[chatMessageJSON](t, w); w.Code != http.StatusCreated || !got.Out || got.Text != "привет" || len(got.Photos) != 1 ||
		got.DeletableUntil == "" || got.EditableUntil == "" || !slices.Equal(h.chats.calls, []string{"send c1 привет ph1"}) {
		t.Fatalf("sent: %d %+v, calls %q", w.Code, got, h.chats.calls)
	}

	h = newHarness(rest.Config{})
	if w = h.do("POST", "/api/v1/chats/c1/messages", `{"photos":["ph7"]}`, "Authorization", annAuth); w.Code != http.StatusBadRequest || len(h.chats.calls) != 0 {
		t.Fatalf("someone else's photo: %d, calls %q", w.Code, h.chats.calls)
	}

	h = newHarness(rest.Config{})
	w = h.do("GET", "/api/v1/chat-messages/m2", "", "Authorization", annAuth)
	if got := decode[chatMessageJSON](t, w); w.Code != http.StatusOK || got.Text != "иду, конечно" || got.Out {
		t.Fatalf("whole: %d %+v", w.Code, got)
	}

	w = h.do("PATCH", "/api/v1/chat-messages/m9", `{"text":"пока"}`, "Authorization", annAuth)
	if got := decode[chatMessageJSON](t, w); w.Code != http.StatusOK || got.Text != "пока" || got.EditedAt == nil || got.EditableUntil == "" {
		t.Fatalf("edited: %d %+v", w.Code, got)
	}

	if w = h.do("DELETE", "/api/v1/chat-messages/m9", "", "Authorization", annAuth); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}

	if w = h.do("POST", "/api/v1/chats/c1/read", `{"upTo":"m2"}`, "Authorization", annAuth); w.Code != http.StatusNoContent {
		t.Fatalf("read: %d", w.Code)
	}

	if w = h.do("POST", "/api/v1/chats/c1/typing", "", "Authorization", annAuth); w.Code != http.StatusNoContent {
		t.Fatalf("typing: %d", w.Code)
	}

	if want := []string{"message m2", "edit m9 пока", "delete m9", "read c1 m2", "typing c1"}; !slices.Equal(h.chats.calls, want) {
		t.Fatalf("calls = %q, want %q", h.chats.calls, want)
	}

	t.Run("around a message", func(t *testing.T) {
		h := newHarness(rest.Config{})
		if w := h.do("GET", "/api/v1/chats/c1/messages?around=m2", "", "Authorization", annAuth); w.Code != http.StatusOK || h.chats.at.Around != "m2" {
			t.Fatalf("%d, asked for %+v", w.Code, h.chats.at)
		}
	})

	t.Run("what a message shares and carries on", func(t *testing.T) {
		type attached struct {
			chatMessageJSON
			Post           string         `json:"post"`
			Repost         map[string]any `json:"repost"`
			AlbumPhoto     string         `json:"albumPhoto"`
			SharedPhoto    map[string]any `json:"sharedPhoto"`
			ForwardedCount int            `json:"forwardedCount"`
			Forwarded      []struct {
				Text   string         `json:"text"`
				Author map[string]any `json:"author"`
			} `json:"forwarded"`
		}

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/chats/c1/messages", `{"post":"p1","forward":["m2","m3"]}`, "Authorization", annAuth)
		got := decode[attached](t, w)

		if w.Code != http.StatusCreated || h.chats.draft.PostID != "p1" || !slices.Equal(h.chats.draft.ForwardIDs, []string{"m2", "m3"}) {
			t.Fatalf("%d, sent %+v", w.Code, h.chats.draft)
		}
		if got.Post != "p1" || got.Repost == nil || got.ForwardedCount != 2 || len(got.Forwarded) != 2 || got.Forwarded[0].Author["nickname"] != "bob" || got.Forwarded[0].Text != "дальше" {
			t.Fatalf("sent = %+v", got)
		}

		h = newHarness(rest.Config{})
		w = h.do("POST", "/api/v1/chats/c1/messages", `{"text":"смотрите","albumPhoto":"ph6"}`, "Authorization", annAuth)
		if got = decode[attached](t, w); w.Code != http.StatusCreated || h.chats.draft.SharedPhotoID != "ph6" || got.AlbumPhoto != "ph6" || got.SharedPhoto == nil {
			t.Fatalf("a photo of an album: %d %+v", w.Code, got)
		}

		// What is not there for the caller is not theirs to share, and
		// messages of chats are not sealed.
		for _, body := range []string{`{"post":"p404"}`, `{"text":"вот","albumPhoto":"ph404"}`, `{"sealed":"eA=="}`} {
			h = newHarness(rest.Config{})
			if w = h.do("POST", "/api/v1/chats/c1/messages", body, "Authorization", annAuth); w.Code != http.StatusBadRequest || len(h.chats.calls) != 0 {
				t.Fatalf("%s: %d, calls %q", body, w.Code, h.chats.calls)
			}
		}
	})

	t.Run("an edit of photos", func(t *testing.T) {
		h := newHarness(rest.Config{})
		w := h.do("PATCH", "/api/v1/chat-messages/m9", `{"text":"вот","addPhotos":["ph1"],"removePhotos":["ph2"]}`, "Authorization", annAuth)

		got := decode[chatMessageJSON](t, w)
		if w.Code != http.StatusOK || len(got.Photos) != 1 || !slices.Equal(h.chats.edit.AddPhotoIDs, []string{"ph1"}) || !slices.Equal(h.chats.edit.RemovePhotoIDs, []string{"ph2"}) ||
			!slices.Equal(h.media.claimed, []string{"ph1"}) || len(h.media.released) != 0 {
			t.Fatalf("%d %+v, edit %+v, claimed %q, released %q", w.Code, got, h.chats.edit, h.media.claimed, h.media.released)
		}

		if h.chats.edit.BaseText != nil {
			t.Fatalf("an edit that names no text it started from sent %q", *h.chats.edit.BaseText)
		}

		// The text the edit started from goes with it.
		w = h.do("PATCH", "/api/v1/chat-messages/m9", `{"text":"вот так","baseText":"вот"}`, "Authorization", annAuth)
		if w.Code != http.StatusOK || h.chats.edit.BaseText == nil || *h.chats.edit.BaseText != "вот" {
			t.Fatalf("an edit from a text: %d, edit %+v", w.Code, h.chats.edit)
		}

		// A refused edit gives the photos back.
		h = newHarness(rest.Config{})
		h.chats.err = failure.New(failure.Conflict, "the message can no longer be edited")

		if w = h.do("PATCH", "/api/v1/chat-messages/m9", `{"addPhotos":["ph1"]}`, "Authorization", annAuth); w.Code != http.StatusConflict || !slices.Equal(h.media.released, []string{"ph1"}) {
			t.Fatalf("refused: %d, released %q", w.Code, h.media.released)
		}

		h = newHarness(rest.Config{})
		if w = h.do("PATCH", "/api/v1/chat-messages/m9", `{"addPhotos":["ph7"]}`, "Authorization", annAuth); w.Code != http.StatusBadRequest || len(h.chats.calls) != 0 {
			t.Fatalf("someone else's photo: %d, calls %q", w.Code, h.chats.calls)
		}
	})

	for _, req := range [][2]string{
		{"GET", "/api/v1/chats/c1/messages"}, {"POST", "/api/v1/chats/c1/messages"}, {"GET", "/api/v1/chat-messages/m2"},
		{"PATCH", "/api/v1/chat-messages/m2"}, {"DELETE", "/api/v1/chat-messages/m2"}, {"POST", "/api/v1/chats/c1/read"},
		{"POST", "/api/v1/chats/c1/typing"},
	} {
		if w = newHarness(rest.Config{}).do(req[0], req[1], `{}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: %d", req[0], req[1], w.Code)
		}
	}
}

func TestSearchChatMessages(t *testing.T) {
	t.Parallel()

	type foundJSON struct {
		Messages []struct {
			Peer    map[string]any `json:"peer"`
			Message *struct {
				ID string `json:"id"`
			} `json:"message"`
			Chat *struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"chat"`
			ChatMessage *struct {
				ID        string         `json:"id"`
				Author    map[string]any `json:"author"`
				Text      string         `json:"text"`
				Forwarded []struct {
					Text   string         `json:"text"`
					Author map[string]any `json:"author"`
				} `json:"forwarded"`
			} `json:"chatMessage"`
		} `json:"messages"`
		NextCursor string `json:"nextCursor"`
	}

	// In one chat.
	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/messages/search?q=%D0%BF%D0%BE%D1%85%D0%BE%D0%B4&chat=c1&cursor=m7", "", "Authorization", annAuth)
	got := decode[foundJSON](t, w)

	if w.Code != http.StatusOK || !slices.Equal(h.chats.calls, []string{"search c1 поход m7"}) || got.NextCursor != "" || len(got.Messages) != 1 {
		t.Fatalf("%d %+v, calls %q", w.Code, got, h.chats.calls)
	}

	row := got.Messages[0]
	if row.Peer != nil || row.Message != nil || row.Chat == nil || row.Chat.ID != "c1" || row.Chat.Title != "Поход" || row.ChatMessage == nil ||
		row.ChatMessage.ID != "m4" || row.ChatMessage.Author["nickname"] != "bob" {
		t.Fatalf("found = %+v", row)
	}
	// What it carries on, by which it may have been found.
	if carried := row.ChatMessage.Forwarded; len(carried) != 1 || carried[0].Text != "какой поход?" || carried[0].Author["nickname"] != "cat" {
		t.Fatalf("the forwarded of a found message = %+v", carried)
	}

	// Everywhere: all the caller's chats. Dialogs are sealed, the server
	// cannot search them.
	h = newHarness(rest.Config{})
	w = h.do("GET", "/api/v1/messages/search?q=x", "", "Authorization", annAuth)
	if got = decode[foundJSON](t, w); w.Code != http.StatusOK || len(got.Messages) != 1 || got.Messages[0].Message != nil || got.Messages[0].ChatMessage.ID != "m4" ||
		!slices.Equal(h.chats.calls, []string{"search  x "}) {
		t.Fatalf("everywhere: %d %+v, calls %q", w.Code, got, h.chats.calls)
	}

	h = newHarness(rest.Config{})
	if w = h.do("GET", "/api/v1/messages/search?q=x&peer=bob", "", "Authorization", annAuth); w.Code != http.StatusBadRequest || len(h.chats.calls) != 0 {
		t.Fatalf("a dialog: %d, calls %q", w.Code, h.chats.calls)
	}
	if w = h.do("GET", "/api/v1/messages/search?q=x", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	h.chats.err = failure.New(failure.NotFound, "chat not found")

	if w = h.do("GET", "/api/v1/messages/search?q=x&chat=c9", "", "Authorization", annAuth); w.Code != http.StatusNotFound {
		t.Fatalf("a chat one is not in: %d", w.Code)
	}
}
