package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeTopics knows t1, bob's (u2) topic in k1, and t2, a topic of the
// club, and t3 by someone whose page is gone; it records what it is asked.
type fakeTopics struct {
	clubID, query, deleted string
	liked                  string
	created                model.NewTopic
	changes                model.TopicChanges
	written                model.NewTopicMessage
	err                    error
}

var topicTime = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func (f *fakeTopics) topic(id string) (model.Topic, error) {
	switch id {
	case "t1":
		return model.Topic{ID: "t1", ClubID: "k1", AuthorID: "u2", Title: "Знакомство", CreatedAt: topicTime, UpdatedAt: topicTime, Messages: 1,
			Last: &model.TopicMessage{ID: "m1", AuthorID: "u1", CreatedAt: topicTime}}, nil
	case "t2":
		return model.Topic{ID: "t2", ClubID: "k1", AuthorID: "u1", Title: "Правила", FromWall: true, Pinned: true, CreatedAt: topicTime, UpdatedAt: topicTime}, nil
	case "t3":
		return model.Topic{ID: "t3", ClubID: "k1", AuthorID: "gone", Title: "Пропавшая", CreatedAt: topicTime, UpdatedAt: topicTime}, nil
	default:
		return model.Topic{}, failure.New(failure.NotFound, "topic not found")
	}
}

func (f *fakeTopics) Create(_ context.Context, _ string, nt model.NewTopic) (model.Topic, error) {
	f.created = nt
	return model.Topic{ID: "t9", ClubID: nt.ClubID, AuthorID: "u1", Title: nt.Title, FromWall: nt.FromWall, CreatedAt: topicTime, UpdatedAt: topicTime, Mine: true}, f.err
}

func (f *fakeTopics) Get(_ context.Context, _, id string) (model.Topic, error) {
	return f.topic(id)
}

func (f *fakeTopics) List(_ context.Context, _, clubID, query, _ string, _ int) (model.TopicPage, error) {
	f.clubID, f.query = clubID, query
	var page model.TopicPage
	for _, id := range []string{"t2", "t1", "t3"} {
		t, _ := f.topic(id)
		page.Topics = append(page.Topics, t)
	}
	page.Total = 3
	return page, f.err
}

func (f *fakeTopics) Edit(_ context.Context, _, id string, changes model.TopicChanges) (model.Topic, error) {
	f.changes = changes
	return f.topic(id)
}

func (f *fakeTopics) Delete(_ context.Context, _, id string) error {
	f.deleted = id
	return f.err
}

func (f *fakeTopics) Write(_ context.Context, _, topicID string, nm model.NewTopicMessage) (model.TopicMessage, error) {
	f.written = nm
	return model.TopicMessage{ID: "m9", TopicID: topicID, AuthorID: "u1", Text: nm.Text, PhotoIDs: nm.PhotoIDs, FromWall: nm.FromWall, CreatedAt: topicTime, Mine: true}, f.err
}

func (f *fakeTopics) Messages(_ context.Context, _, topicID, _ string, _ int) (model.TopicMessagePage, error) {
	return model.TopicMessagePage{Messages: []model.TopicMessage{
		{ID: "m1", TopicID: topicID, AuthorID: "u2", Text: "Привет", PhotoIDs: []string{"ph1"}, CreatedAt: topicTime},
		{ID: "m2", TopicID: topicID, AuthorID: "u1", Text: "Новость", FromWall: true, CreatedAt: topicTime},
		{ID: "m3", TopicID: topicID, AuthorID: "gone", Text: "?", CreatedAt: topicTime},
	}, Total: 3}, f.err
}

// Message knows m1, whose text lists cut.
func (f *fakeTopics) Message(_ context.Context, id string) (model.TopicMessage, error) {
	// m3 is of someone whose page is gone.
	if id == "m3" {
		return model.TopicMessage{ID: id, TopicID: "t1", AuthorID: "gone", Text: "?", CreatedAt: topicTime}, nil
	}

	if id != "m1" {
		return model.TopicMessage{}, failure.New(failure.NotFound, "message not found")
	}

	return model.TopicMessage{ID: id, TopicID: "t1", AuthorID: "u2", Text: "Привет, целиком", CreatedAt: topicTime}, nil
}

// MessagesByID knows m1.
func (f *fakeTopics) MessagesByID(_ context.Context, ids []string) ([]model.TopicMessage, error) {
	var found []model.TopicMessage
	for _, id := range ids {
		if id == "m1" {
			found = append(found, model.TopicMessage{ID: id, TopicID: "t1", AuthorID: "u2", Text: "Привет"})
		}
	}

	return found, f.err
}

func (f *fakeTopics) MessageLikers(_ context.Context, _, _ string, _ int) (model.LikerPage, error) {
	return model.LikerPage{UserIDs: []string{"u2"}, Total: 1}, f.err
}

func (f *fakeTopics) DeleteMessage(_ context.Context, _, id string) error {
	f.deleted = id
	return f.err
}

func (f *fakeTopics) LikeMessage(_ context.Context, _, id string) (int, error) {
	f.liked = id
	return 3, f.err
}

func (f *fakeTopics) UnlikeMessage(_ context.Context, _, id string) (int, error) {
	f.liked = ""
	return 2, f.err
}

type topicJSON struct {
	ID          string         `json:"id"`
	Author      map[string]any `json:"author"`
	Title       string         `json:"title"`
	FromWall    bool           `json:"fromWall"`
	Pinned      bool           `json:"pinned"`
	Messages    int            `json:"messages"`
	LastMessage *struct {
		Author map[string]any `json:"author"`
	} `json:"lastMessage"`
	Mine bool `json:"mine"`
}

func TestListTopics(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/clubs/cats/topics?q=зна", "")
	got := decode[struct {
		Topics []topicJSON `json:"topics"`
		Total  int         `json:"total"`
	}](t, w)

	if w.Code != http.StatusOK || h.topics.clubID != "k1" || h.topics.query != "зна" || got.Total != 3 {
		t.Fatalf("%d %+v, asked %q %q", w.Code, got, h.topics.clubID, h.topics.query)
	}
	// The club's topic shows the club; one whose author is gone is left out.
	if len(got.Topics) != 2 || got.Topics[0].Author["name"] != "Кошки" || got.Topics[0].Author["club"] != true || !got.Topics[0].Pinned {
		t.Fatalf("topics = %+v", got.Topics)
	}
	if bobs := got.Topics[1]; bobs.Author["nickname"] != "bob" || bobs.LastMessage == nil || bobs.LastMessage.Author["nickname"] != "ann" {
		t.Fatalf("bob's topic = %+v", bobs)
	}

	if w = h.do("GET", "/api/v1/clubs/nobody/topics", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown club: %d", w.Code)
	}
}

func TestCreateTopic(t *testing.T) {
	t.Parallel()

	body := `{"title":"Знакомство","text":"Привет","photos":["ph1"]}`

	h := newHarness(rest.Config{})
	if w := h.do("POST", "/api/v1/clubs/cats/topics", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
	if w := h.do("POST", "/api/v1/clubs/cats/topics", body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("without a profile: %d", w.Code)
	}

	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	if w := h.do("POST", "/api/v1/clubs/cats/topics", body, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("only the admins start topics: %d", w.Code)
	}

	h.clubs.openTopics = true
	if w := h.do("POST", "/api/v1/clubs/cats/topics", `{"title":"x","fromClub":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member for the club: %d", w.Code)
	}
	if w := h.do("POST", "/api/v1/clubs/cats/topics", `{"title":"x","photos":["ph9"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("someone else's photo: %d", w.Code)
	}

	w := h.do("POST", "/api/v1/clubs/cats/topics", body, "Authorization", "Bearer tok")
	got := decode[topicJSON](t, w)
	if w.Code != http.StatusCreated || got.Author["nickname"] != "ann" || !got.Mine || h.topics.created.ClubID != "k1" || h.topics.created.Text != "Привет" {
		t.Fatalf("%d %+v, created %+v", w.Code, got, h.topics.created)
	}

	h.clubs.role = "admin"
	w = h.do("POST", "/api/v1/clubs/cats/topics", `{"title":"Правила","fromClub":true}`, "Authorization", "Bearer tok")
	if got = decode[topicJSON](t, w); w.Code != http.StatusCreated || got.Author["club"] != true || !h.topics.created.FromWall {
		t.Fatalf("for the club: %d %+v", w.Code, got)
	}
}

func TestTopicPage(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/topics/t1", "")
	got := decode[struct {
		Topic topicJSON      `json:"topic"`
		Club  map[string]any `json:"club"`
		Role  string         `json:"role"`
	}](t, w)
	if w.Code != http.StatusOK || got.Topic.Author["nickname"] != "bob" || got.Club["address"] != "cats" || got.Role != "" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if w = h.do("GET", "/api/v1/topics/t3", ""); w.Code != http.StatusNotFound {
		t.Fatalf("a topic of a deleted page: %d", w.Code)
	}

	w = h.do("PATCH", "/api/v1/topics/t1", `{"title":"Знакомимся","closed":true}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.topics.changes.Title != "Знакомимся" || !h.topics.changes.Closed {
		t.Fatalf("edit: %d, changes %+v", w.Code, h.topics.changes)
	}
	if w = h.do("DELETE", "/api/v1/topics/t1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.topics.deleted != "t1" {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestTopicMessages(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/topics/t1/messages", "")
	got := decode[struct {
		Messages []struct {
			Author map[string]any `json:"author"`
			Photos []struct {
				Small string `json:"small"`
			} `json:"photos"`
		} `json:"messages"`
		Total int `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Messages) != 2 || got.Total != 3 || got.Messages[0].Author["nickname"] != "bob" || got.Messages[1].Author["club"] != true {
		t.Fatalf("%d %+v", w.Code, got)
	}
	// Photos stay filed under their author, also for the club.
	if got.Messages[0].Photos[0].Small != "/media/photos/u2/ph1/small.jpg" {
		t.Fatalf("photos = %+v", got.Messages[0].Photos)
	}

	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	if w = h.do("POST", "/api/v1/topics/t1/messages", `{"text":"а я","fromClub":true}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member for the club: %d", w.Code)
	}
	w = h.do("POST", "/api/v1/topics/t1/messages", `{"text":"И тебе"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusCreated || h.topics.written.Text != "И тебе" {
		t.Fatalf("write: %d %+v", w.Code, h.topics.written)
	}
	// The heart of a message, as of a comment.
	type likeJSON struct {
		Likes int  `json:"likes"`
		Liked bool `json:"liked"`
	}
	w = h.do("PUT", "/api/v1/topic-messages/m1/like", "", "Authorization", "Bearer tok")
	if like := decode[likeJSON](t, w); w.Code != http.StatusOK || like != (likeJSON{Likes: 3, Liked: true}) || h.topics.liked != "m1" {
		t.Fatalf("like: %d %s", w.Code, w.Body)
	}
	w = h.do("DELETE", "/api/v1/topic-messages/m1/like", "", "Authorization", "Bearer tok")
	if like := decode[likeJSON](t, w); w.Code != http.StatusOK || like != (likeJSON{Likes: 2}) || h.topics.liked != "" {
		t.Fatalf("unlike: %d %s", w.Code, w.Body)
	}
	if w = h.do("PUT", "/api/v1/topic-messages/m1/like", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("like without a token: %d", w.Code)
	}

	if w = h.do("DELETE", "/api/v1/topic-messages/m1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.topics.deleted != "m1" {
		t.Fatalf("delete: %d", w.Code)
	}
}
