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
}

func (f *fakeChats) did(token string, call ...string) {
	f.token = token
	f.calls = append(f.calls, strings.Join(call, " "))
}

func (f *fakeChats) chat() model.Chat {
	return model.Chat{
		ID: "c1", Title: "Поход", OwnerID: "u1", CreatedAt: chatTime, MemberCount: 3, State: model.ChatStateActive, Unread: true,
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

	return model.ChatView{Chat: f.chat(), Members: []model.ChatMember{{UserID: "u1"}, {UserID: "u2"}, {UserID: "gone"}}}, nil
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

func (f *fakeChats) Send(_ context.Context, token, id, text string, photoIDs []string) (model.ChatMessage, error) {
	f.did(token, "send", id, text, strings.Join(photoIDs, ","))
	return model.ChatMessage{ID: "m9", ChatID: id, SenderID: "u1", Text: text, PhotoIDs: photoIDs, CreatedAt: chatTime, DeletableUntil: chatTime, EditableUntil: chatTime}, f.err
}

// History: ann created the chat, bob wrote, somebody whose page is gone
// wrote too, and ann removed cat.
func (f *fakeChats) History(_ context.Context, token, id, cursor, after string, _ int) (model.ChatMessagePage, error) {
	f.did(token, "history", id, cursor, after)

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

func (f *fakeChats) Edit(_ context.Context, token, id, text string) (model.ChatMessage, error) {
	f.did(token, "edit", id, text)
	return model.ChatMessage{ID: id, ChatID: "c1", SenderID: "u1", Text: text, CreatedAt: chatTime, EditedAt: chatTime, EditableUntil: chatTime}, f.err
}

func (f *fakeChats) Delete(_ context.Context, token, id string) error {
	f.did(token, "delete", id)
	return f.err
}

func (f *fakeChats) MarkRead(_ context.Context, token, id, upTo string) error {
	f.did(token, "read", id, upTo)
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

	if want := []string{"message m2", "edit m9 пока", "delete m9", "read c1 m2"}; !slices.Equal(h.chats.calls, want) {
		t.Fatalf("calls = %q, want %q", h.chats.calls, want)
	}

	for _, req := range [][2]string{
		{"GET", "/api/v1/chats/c1/messages"}, {"POST", "/api/v1/chats/c1/messages"}, {"GET", "/api/v1/chat-messages/m2"},
		{"PATCH", "/api/v1/chat-messages/m2"}, {"DELETE", "/api/v1/chat-messages/m2"}, {"POST", "/api/v1/chats/c1/read"},
	} {
		if w = newHarness(rest.Config{}).do(req[0], req[1], `{}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: %d", req[0], req[1], w.Code)
		}
	}
}
