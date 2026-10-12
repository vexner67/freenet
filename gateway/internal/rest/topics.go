package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

type topicRequest struct {
	Title string `json:"title"`
	// Text and Photos are the first message ("Текст"); both may be empty.
	Text   string   `json:"text"`
	Photos []string `json:"photos"`
	// FromClub: the topic speaks for the community, its admins only.
	FromClub bool `json:"fromClub"`
}

type topicSettingsRequest struct {
	Title      string `json:"title"`
	Closed     bool   `json:"closed"`
	Pinned     bool   `json:"pinned"`
	Restricted bool   `json:"restricted"`
}

type topicMessageRequest struct {
	Text   string   `json:"text"`
	Photos []string `json:"photos"`
	// FromClub: the message speaks for the community, its admins only.
	FromClub bool `json:"fromClub"`
}

// lastMessageJSON is who answered last in a topic, and when ("ответил").
type lastMessageJSON struct {
	Author    authorJSON `json:"author"`
	CreatedAt string     `json:"createdAt"`
}

type topicJSON struct {
	ID string `json:"id"`
	// Author is who started it, or the community for a topic of the
	// community (fromWall).
	Author     authorJSON       `json:"author"`
	Title      string           `json:"title"`
	FromWall   bool             `json:"fromWall"`
	Closed     bool             `json:"closed"`
	Pinned     bool             `json:"pinned"`
	Restricted bool             `json:"restricted"`
	CreatedAt  string           `json:"createdAt"`
	UpdatedAt  string           `json:"updatedAt"`
	Messages   int              `json:"messages"`
	Last       *lastMessageJSON `json:"lastMessage"`
	// Mine: the viewer started it.
	Mine bool `json:"mine"`
}

type topicPageResponse struct {
	Topics     []topicJSON `json:"topics"`
	NextCursor string      `json:"nextCursor"`
	Total      int         `json:"total"`
}

// topicResponse is a topic with its community, for the page of the topic:
// role is what the viewer is in the community.
type topicResponse struct {
	Topic topicJSON  `json:"topic"`
	Club  authorJSON `json:"club"`
	Role  string     `json:"role"`
}

// topicMessageJSON is shaped as a comment, which the site draws alike.
type topicMessageJSON struct {
	ID     string     `json:"id"`
	Author authorJSON `json:"author"`
	Text   string     `json:"text"`
	// TextCut: text is only its beginning, as lists carry it; the whole
	// is at /topic-messages/{id}/text.
	TextCut   bool        `json:"textCut,omitempty"`
	Photos    []photoJSON `json:"photos"`
	CreatedAt string      `json:"createdAt"`
	FromWall  bool        `json:"fromWall"`
	Mine      bool        `json:"mine"`
	// Likes are its hearts; Liked: the viewer put one.
	Likes int  `json:"likes"`
	Liked bool `json:"liked"`
}

type topicMessagePageResponse struct {
	Messages   []topicMessageJSON `json:"messages"`
	NextCursor string             `json:"nextCursor"`
	Total      int                `json:"total"`
}

// shownBy is who a topic or a message is shown by: its author, or the
// community it speaks for; false when that one is gone.
func shownBy(authorID string, fromWall bool, club model.Author, people map[string]model.Author) (model.Author, bool) {
	if fromWall {
		return club, true
	}

	author, ok := people[authorID]

	return author, ok
}

func toTopicJSON(t model.Topic, shown model.Author, last *lastMessageJSON) topicJSON {
	return topicJSON{
		ID:         t.ID,
		Author:     toAuthorJSON(shown),
		Title:      t.Title,
		FromWall:   t.FromWall,
		Closed:     t.Closed,
		Pinned:     t.Pinned,
		Restricted: t.Restricted,
		CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  t.UpdatedAt.UTC().Format(time.RFC3339),
		Messages:   t.Messages,
		Last:       last,
		Mine:       t.Mine,
	}
}

func toTopicMessageJSON(m model.TopicMessage, shown model.Author) topicMessageJSON {
	return topicMessageJSON{
		ID:        m.ID,
		Author:    toAuthorJSON(shown),
		Text:      m.Text,
		TextCut:   m.TextCut,
		Photos:    toPhotosJSON(m.AuthorID, m.PhotoIDs),
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		FromWall:  m.FromWall,
		Mine:      m.Mine,
		Likes:     m.Likes,
		Liked:     m.Liked,
	}
}

// listTopics is the discussions of a club ("Обсуждения"), public; ?q=
// finds topics by their titles. Topics whose authors' pages are deleted
// are left out, but counted in total.
func (a *api) listTopics(w http.ResponseWriter, r *http.Request) {
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

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	query := r.URL.Query()

	page, err := a.topics.List(r.Context(), token, c.Club.ID, query.Get("q"), query.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var ids []string
	for _, t := range page.Topics {
		ids = append(ids, t.AuthorID)
		if t.Last != nil {
			ids = append(ids, t.Last.AuthorID)
		}
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	club := clubAuthor(c.Club)
	resp := topicPageResponse{Topics: make([]topicJSON, 0, len(page.Topics)), NextCursor: page.NextCursor, Total: page.Total}

	for _, t := range page.Topics {
		shown, ok := shownBy(t.AuthorID, t.FromWall, club, people)
		if !ok {
			continue
		}

		var last *lastMessageJSON
		if t.Last != nil {
			if by, ok := shownBy(t.Last.AuthorID, t.Last.FromWall, club, people); ok {
				last = &lastMessageJSON{Author: toAuthorJSON(by), CreatedAt: t.Last.CreatedAt.UTC().Format(time.RFC3339)}
			}
		}

		resp.Topics = append(resp.Topics, toTopicJSON(t, shown, last))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createTopic starts a topic, openvk's "Новая тема": anyone the club lets
// (canStartTopic), for the club only its admins. A filled-in profile is
// needed, as to post.
func (a *api) createTopic(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req topicRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before posting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.club(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if !c.CanStartTopic {
		a.writeError(w, r, failure.New(failure.PermissionDenied, "you cannot start topics here"))
		return
	}

	if err = a.speakForClub(c, req.FromClub); err != nil {
		a.writeError(w, r, err)
		return
	}

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.claimPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	t, err := a.topics.Create(r.Context(), token, model.NewTopic{
		ClubID: c.Club.ID, Title: req.Title, Text: req.Text, PhotoIDs: req.Photos, FromWall: req.FromClub, Mentions: mentions,
	})
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	shown := authorOf(me.ID, me.AvatarID, me.Profile)
	if t.FromWall {
		shown = clubAuthor(c.Club)
	}

	writeJSON(w, http.StatusCreated, toTopicJSON(t, shown, nil))
}

// speakForClub refuses a topic or a message for the club from someone who
// does not run it.
func (a *api) speakForClub(c model.ClubView, fromClub bool) error {
	if fromClub && !manages(c.Role) {
		return failure.New(failure.PermissionDenied, "you cannot write for this community")
	}

	return nil
}

// topicAndClub are the topic of the path and its community as the viewer
// sees it.
func (a *api) topicAndClub(r *http.Request, token string) (model.Topic, model.ClubView, error) {
	t, err := a.topics.Get(r.Context(), token, r.PathValue("id"))
	if err != nil {
		return model.Topic{}, model.ClubView{}, err
	}

	c, err := a.clubs.Get(r.Context(), token, t.ClubID)
	if err != nil {
		return model.Topic{}, model.ClubView{}, err
	}

	return t, c, nil
}

// getTopic is a topic with its community, for its page; public.
func (a *api) getTopic(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	t, c, err := a.topicAndClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.authors(r, []string{t.AuthorID})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	club := clubAuthor(c.Club)

	shown, ok := shownBy(t.AuthorID, t.FromWall, club, people)
	if !ok {
		a.writeError(w, r, failure.New(failure.NotFound, "topic not found"))
		return
	}

	writeJSON(w, http.StatusOK, topicResponse{Topic: toTopicJSON(t, shown, nil), Club: toAuthorJSON(club), Role: c.Role})
}

// editTopic saves "Редактирование темы": the post service decides who
// may change what.
func (a *api) editTopic(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req topicSettingsRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.topics.Edit(r.Context(), token, r.PathValue("id"), model.TopicChanges(req)); err != nil {
		a.writeError(w, r, err)
		return
	}

	// The page of the topic again, with who started it.
	a.getTopic(w, r)
}

func (a *api) deleteTopic(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.topics.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listTopicMessages is what is written in a topic, oldest first; public.
func (a *api) listTopicMessages(w http.ResponseWriter, r *http.Request) {
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

	t, c, err := a.topicAndClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.topics.Messages(r.Context(), token, t.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(page.Messages))
	for _, m := range page.Messages {
		ids = append(ids, m.AuthorID)
	}

	people, err := a.authors(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	club := clubAuthor(c.Club)
	resp := topicMessagePageResponse{Messages: make([]topicMessageJSON, 0, len(page.Messages)), NextCursor: page.NextCursor, Total: page.Total}

	for _, m := range page.Messages {
		// An author whose page is deleted is left out, as under posts.
		if shown, ok := shownBy(m.AuthorID, m.FromWall, club, people); ok {
			resp.Messages = append(resp.Messages, toTopicMessageJSON(m, shown))
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// createTopicMessage writes in a topic; for the community only its admins.
func (a *api) createTopicMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req topicMessageRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.completedMe(r, token, "fill in your profile before posting")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	t, c, err := a.topicAndClub(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.speakForClub(c, req.FromClub); err != nil {
		a.writeError(w, r, err)
		return
	}

	mentions, err := a.mentioned(r, token, req.Text)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.claimPhotos(r, token, req.Photos); err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.topics.Write(r.Context(), token, t.ID, model.NewTopicMessage{
		Text: req.Text, PhotoIDs: req.Photos, FromWall: req.FromClub, Mentions: mentions,
	})
	if err != nil {
		a.releasePhotos(r, token, req.Photos, err)
		a.writeError(w, r, err)
		return
	}

	shown := authorOf(me.ID, me.AvatarID, me.Profile)
	if m.FromWall {
		shown = clubAuthor(c.Club)
	}

	writeJSON(w, http.StatusCreated, toTopicMessageJSON(m, shown))
}

func (a *api) deleteTopicMessage(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.topics.DeleteMessage(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// likeTopicMessage and unlikeTopicMessage are the heart of a message of
// a topic, as of a comment.
func (a *api) likeTopicMessage(w http.ResponseWriter, r *http.Request) {
	a.setTopicMessageLike(w, r, true)
}

func (a *api) unlikeTopicMessage(w http.ResponseWriter, r *http.Request) {
	a.setTopicMessageLike(w, r, false)
}

func (a *api) setTopicMessageLike(w http.ResponseWriter, r *http.Request, liked bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	set := a.topics.UnlikeMessage
	if liked {
		set = a.topics.LikeMessage
	}

	likes, err := set(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, likeResponse{Likes: likes, Liked: liked})
}

// topicMessageText is what a message of a topic says, whole: the topic
// carries its beginning. Public, as discussions are.
func (a *api) topicMessageText(w http.ResponseWriter, r *http.Request) {
	m, err := a.topics.Message(r.Context(), r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, textResponse{Text: m.Text})
}

// listTopicMessageLikers is "Понравилось N людям" of a message of a
// topic, public as discussions are.
func (a *api) listTopicMessageLikers(w http.ResponseWriter, r *http.Request) {
	a.listLikers(w, r, a.topics.MessageLikers)
}
