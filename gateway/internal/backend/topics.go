package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Topics is rest.Topics over the post service, which keeps the
// discussions of communities.
type Topics struct {
	api postv1.PostServiceClient
}

func NewTopics(api postv1.PostServiceClient) *Topics {
	return &Topics{
		api: api,
	}
}

func (t *Topics) Create(ctx context.Context, accessToken string, nt model.NewTopic) (model.Topic, error) {
	resp, err := t.api.CreateTopic(withToken(ctx, accessToken), &postv1.CreateTopicRequest{
		ClubId: nt.ClubID, Title: nt.Title, Text: nt.Text, PhotoIds: nt.PhotoIDs, FromWall: nt.FromWall,
	})
	if err != nil {
		return model.Topic{}, errs.Wrap("t.api.CreateTopic", fromStatus(err))
	}

	return toTopic(resp.GetTopic()), nil
}

func (t *Topics) Get(ctx context.Context, accessToken, id string) (model.Topic, error) {
	resp, err := t.api.GetTopic(withToken(ctx, accessToken), &postv1.GetTopicRequest{Id: id})
	if err != nil {
		return model.Topic{}, errs.Wrap("t.api.GetTopic", fromStatus(err))
	}

	return toTopic(resp.GetTopic()), nil
}

func (t *Topics) List(ctx context.Context, accessToken, clubID, query, cursor string, limit int) (model.TopicPage, error) {
	resp, err := t.api.ListTopics(withToken(ctx, accessToken), &postv1.ListTopicsRequest{
		ClubId: clubID, Query: query, Cursor: cursor, Limit: int32(limit), //nolint:gosec // limit is validated by the caller
	})
	if err != nil {
		return model.TopicPage{}, errs.Wrap("t.api.ListTopics", fromStatus(err))
	}

	page := model.TopicPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pt := range resp.GetTopics() {
		page.Topics = append(page.Topics, toTopic(pt))
	}

	return page, nil
}

func (t *Topics) Edit(ctx context.Context, accessToken, id string, changes model.TopicChanges) (model.Topic, error) {
	resp, err := t.api.EditTopic(withToken(ctx, accessToken), &postv1.EditTopicRequest{
		Id: id, Title: changes.Title, Closed: changes.Closed, Pinned: changes.Pinned, Restricted: changes.Restricted,
	})
	if err != nil {
		return model.Topic{}, errs.Wrap("t.api.EditTopic", fromStatus(err))
	}

	return toTopic(resp.GetTopic()), nil
}

func (t *Topics) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := t.api.DeleteTopic(withToken(ctx, accessToken), &postv1.DeleteTopicRequest{Id: id}); err != nil {
		return errs.Wrap("t.api.DeleteTopic", fromStatus(err))
	}

	return nil
}

func (t *Topics) Write(ctx context.Context, accessToken, topicID string, nm model.NewTopicMessage) (model.TopicMessage, error) {
	resp, err := t.api.CreateTopicMessage(withToken(ctx, accessToken), &postv1.CreateTopicMessageRequest{
		TopicId: topicID, Text: nm.Text, PhotoIds: nm.PhotoIDs, FromWall: nm.FromWall,
	})
	if err != nil {
		return model.TopicMessage{}, errs.Wrap("t.api.CreateTopicMessage", fromStatus(err))
	}

	return toTopicMessage(resp.GetMessage()), nil
}

func (t *Topics) Messages(ctx context.Context, accessToken, topicID, cursor string, limit int) (model.TopicMessagePage, error) {
	resp, err := t.api.ListTopicMessages(withToken(ctx, accessToken), &postv1.ListTopicMessagesRequest{
		TopicId: topicID, Cursor: cursor, Limit: int32(limit), //nolint:gosec // limit is validated by the caller
	})
	if err != nil {
		return model.TopicMessagePage{}, errs.Wrap("t.api.ListTopicMessages", fromStatus(err))
	}

	page := model.TopicMessagePage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pm := range resp.GetMessages() {
		page.Messages = append(page.Messages, toTopicMessage(pm))
	}

	return page, nil
}

func (t *Topics) DeleteMessage(ctx context.Context, accessToken, id string) error {
	if _, err := t.api.DeleteTopicMessage(withToken(ctx, accessToken), &postv1.DeleteTopicMessageRequest{Id: id}); err != nil {
		return errs.Wrap("t.api.DeleteTopicMessage", fromStatus(err))
	}

	return nil
}

func toTopic(pt *postv1.Topic) model.Topic {
	t := model.Topic{
		ID:         pt.GetId(),
		ClubID:     pt.GetClubId(),
		AuthorID:   pt.GetAuthorId(),
		Title:      pt.GetTitle(),
		FromWall:   pt.GetFromWall(),
		Closed:     pt.GetClosed(),
		Pinned:     pt.GetPinned(),
		Restricted: pt.GetRestricted(),
		CreatedAt:  fromTimestamp(pt.GetCreatedAt()),
		UpdatedAt:  fromTimestamp(pt.GetUpdatedAt()),
		Messages:   int(pt.GetMessageCount()),
		Mine:       pt.GetMine(),
	}
	if pt.GetLastMessage() != nil {
		last := toTopicMessage(pt.GetLastMessage())
		t.Last = &last
	}

	return t
}

func toTopicMessage(pm *postv1.TopicMessage) model.TopicMessage {
	return model.TopicMessage{
		ID:        pm.GetId(),
		TopicID:   pm.GetTopicId(),
		AuthorID:  pm.GetAuthorId(),
		Text:      pm.GetText(),
		PhotoIDs:  pm.GetPhotoIds(),
		FromWall:  pm.GetFromWall(),
		CreatedAt: fromTimestamp(pm.GetCreatedAt()),
		Mine:      pm.GetMine(),
	}
}
