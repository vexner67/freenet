package backend

import (
	"context"

	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	"google.golang.org/grpc"
)

// Federation opens pages to the Fediverse and serves what other servers
// read of them.
type Federation struct {
	api federationv1.FederationServiceClient
}

func NewFederation(api federationv1.FederationServiceClient) *Federation {
	return &Federation{api: api}
}

func (f *Federation) Federation(ctx context.Context, accessToken string) (model.Federation, error) {
	resp, err := f.api.GetFederation(withToken(ctx, accessToken), &federationv1.GetFederationRequest{})
	if err != nil {
		return model.Federation{}, errs.Wrap("f.api.GetFederation", fromStatus(err))
	}

	return fromProtoFederation(resp.GetStatus()), nil
}

func (f *Federation) Open(ctx context.Context, accessToken string) (model.Federation, error) {
	resp, err := f.api.OpenFederation(withToken(ctx, accessToken), &federationv1.OpenFederationRequest{})
	if err != nil {
		return model.Federation{}, errs.Wrap("f.api.OpenFederation", fromStatus(err))
	}

	return fromProtoFederation(resp.GetStatus()), nil
}

func (f *Federation) Close(ctx context.Context, accessToken string) (model.Federation, error) {
	resp, err := f.api.CloseFederation(withToken(ctx, accessToken), &federationv1.CloseFederationRequest{})
	if err != nil {
		return model.Federation{}, errs.Wrap("f.api.CloseFederation", fromStatus(err))
	}

	return fromProtoFederation(resp.GetStatus()), nil
}

func (f *Federation) Finger(ctx context.Context, resource string) ([]byte, error) {
	resp, err := f.api.Finger(ctx, &federationv1.FingerRequest{Resource: resource})
	if err != nil {
		return nil, errs.Wrap("f.api.Finger", fromStatus(err))
	}

	return resp.GetDocument(), nil
}

func (f *Federation) Actor(ctx context.Context, userID string) (model.FederationDocument, error) {
	resp, err := f.api.GetActor(ctx, &federationv1.GetActorRequest{UserId: userID})
	if err != nil {
		return model.FederationDocument{}, errs.Wrap("f.api.GetActor", fromStatus(err))
	}

	return model.FederationDocument{Body: resp.GetDocument(), PageURL: resp.GetPageUrl()}, nil
}

func (f *Federation) Note(ctx context.Context, postID string) (model.FederationDocument, error) {
	resp, err := f.api.GetNote(ctx, &federationv1.GetNoteRequest{PostId: postID})
	if err != nil {
		return model.FederationDocument{}, errs.Wrap("f.api.GetNote", fromStatus(err))
	}

	return model.FederationDocument{Body: resp.GetDocument(), PageURL: resp.GetPageUrl()}, nil
}

func (f *Federation) Followers(ctx context.Context, userID string) ([]byte, error) {
	resp, err := f.api.GetFollowers(ctx, &federationv1.GetFollowersRequest{UserId: userID})
	if err != nil {
		return nil, errs.Wrap("f.api.GetFollowers", fromStatus(err))
	}

	return resp.GetDocument(), nil
}

func (f *Federation) Outbox(ctx context.Context, userID string) ([]byte, error) {
	resp, err := f.api.GetOutbox(ctx, &federationv1.GetOutboxRequest{UserId: userID})
	if err != nil {
		return nil, errs.Wrap("f.api.GetOutbox", fromStatus(err))
	}

	return resp.GetDocument(), nil
}

func (f *Federation) Receive(ctx context.Context, in model.InboxRequest) error {
	_, err := f.api.Receive(ctx, &federationv1.ReceiveRequest{
		Method: in.Method, Path: in.Path, Headers: in.Headers, Body: in.Body, UserId: in.UserID,
	})
	if err != nil {
		return errs.Wrap("f.api.Receive", fromStatus(err))
	}

	return nil
}

func fromProtoFederation(s *federationv1.FederationStatus) model.Federation {
	return model.Federation{Open: s.GetOpen(), Followers: int(s.GetFollowers()), Domain: s.GetDomain()}
}

// maxPictureBytes is above the largest picture federation reads from
// another server (8 MB), with room for the message around it.
const maxPictureBytes = 9 << 20

func (f *Federation) SiteActor(ctx context.Context) ([]byte, error) {
	resp, err := f.api.GetSiteActor(ctx, &federationv1.GetSiteActorRequest{})
	if err != nil {
		return nil, errs.Wrap("f.api.GetSiteActor", fromStatus(err))
	}

	return resp.GetDocument(), nil
}

func (f *Federation) Lookup(ctx context.Context, accessToken, address string) (model.FediverseLookup, error) {
	resp, err := f.api.LookupAccount(withToken(ctx, accessToken), &federationv1.LookupAccountRequest{Address: address})
	if err != nil {
		return model.FediverseLookup{}, errs.Wrap("f.api.LookupAccount", fromStatus(err))
	}

	return model.FediverseLookup{Account: fromProtoAccount(resp.GetAccount()), Subscribed: resp.GetSubscribed()}, nil
}

func (f *Federation) Subscriptions(ctx context.Context, accessToken string) ([]model.FediverseAccount, error) {
	resp, err := f.api.ListSubscriptions(withToken(ctx, accessToken), &federationv1.ListSubscriptionsRequest{})
	if err != nil {
		return nil, errs.Wrap("f.api.ListSubscriptions", fromStatus(err))
	}

	return fromProtoAccounts(resp.GetAccounts()), nil
}

func (f *Federation) Subscribe(ctx context.Context, accessToken, address string) (model.FediverseAccount, error) {
	resp, err := f.api.Subscribe(withToken(ctx, accessToken), &federationv1.SubscribeRequest{Address: address})
	if err != nil {
		return model.FediverseAccount{}, errs.Wrap("f.api.Subscribe", fromStatus(err))
	}

	return fromProtoAccount(resp.GetAccount()), nil
}

func (f *Federation) Unsubscribe(ctx context.Context, accessToken, id string) error {
	if _, err := f.api.Unsubscribe(withToken(ctx, accessToken), &federationv1.UnsubscribeRequest{Id: id}); err != nil {
		return errs.Wrap("f.api.Unsubscribe", fromStatus(err))
	}

	return nil
}

func (f *Federation) News(ctx context.Context, accessToken string, q model.FediverseNews) ([]model.FediversePost, error) {
	scope := federationv1.NewsScope_NEWS_SCOPE_SUBSCRIBED
	if q.Featured {
		scope = federationv1.NewsScope_NEWS_SCOPE_FEATURED
	}

	resp, err := f.api.ListNews(withToken(ctx, accessToken), &federationv1.ListNewsRequest{
		Scope: scope, Before: q.Before, Limit: int32(q.Limit), HideSensitive: q.HideSensitive, //nolint:gosec // a page size
	})
	if err != nil {
		return nil, errs.Wrap("f.api.ListNews", fromStatus(err))
	}

	posts := make([]model.FediversePost, len(resp.GetPosts()))
	for i, p := range resp.GetPosts() {
		posts[i] = fromProtoNewsPost(p)
	}

	return posts, nil
}

func (f *Federation) Featured(ctx context.Context) ([]model.FediverseAccount, error) {
	resp, err := f.api.ListFeatured(ctx, &federationv1.ListFeaturedRequest{})
	if err != nil {
		return nil, errs.Wrap("f.api.ListFeatured", fromStatus(err))
	}

	return fromProtoAccounts(resp.GetAccounts()), nil
}

func (f *Federation) Feature(ctx context.Context, address string) (model.FediverseAccount, error) {
	resp, err := f.api.Feature(ctx, &federationv1.FeatureRequest{Address: address})
	if err != nil {
		return model.FediverseAccount{}, errs.Wrap("f.api.Feature", fromStatus(err))
	}

	return fromProtoAccount(resp.GetAccount()), nil
}

func (f *Federation) Unfeature(ctx context.Context, id string) error {
	if _, err := f.api.Unfeature(ctx, &federationv1.UnfeatureRequest{Id: id}); err != nil {
		return errs.Wrap("f.api.Unfeature", fromStatus(err))
	}

	return nil
}

func (f *Federation) PostPicture(ctx context.Context, postID string, index int) (model.Picture, error) {
	return f.picture(ctx, &federationv1.GetPictureRequest{PostId: postID, Index: int32(index)}) //nolint:gosec // an index of at most four
}

func (f *Federation) AccountPicture(ctx context.Context, accountID string) (model.Picture, error) {
	return f.picture(ctx, &federationv1.GetPictureRequest{AccountId: accountID})
}

func (f *Federation) picture(ctx context.Context, req *federationv1.GetPictureRequest) (model.Picture, error) {
	resp, err := f.api.GetPicture(ctx, req, grpc.MaxCallRecvMsgSize(maxPictureBytes))
	if err != nil {
		return model.Picture{}, errs.Wrap("f.api.GetPicture", fromStatus(err))
	}

	return model.Picture{Body: resp.GetBody(), ContentType: resp.GetContentType()}, nil
}

func fromProtoAccount(a *federationv1.Account) model.FediverseAccount {
	return model.FediverseAccount{
		ID:        a.GetId(),
		Handle:    a.GetHandle(),
		Name:      a.GetName(),
		PageURL:   a.GetPageUrl(),
		HasAvatar: a.GetHasAvatar(),
		Accepted:  a.GetAccepted(),
	}
}

func fromProtoAccounts(accounts []*federationv1.Account) []model.FediverseAccount {
	out := make([]model.FediverseAccount, len(accounts))
	for i, a := range accounts {
		out[i] = fromProtoAccount(a)
	}

	return out
}

func fromProtoNewsPost(p *federationv1.NewsPost) model.FediversePost {
	descriptions := make([]string, len(p.GetImages()))
	for i, img := range p.GetImages() {
		descriptions[i] = img.GetDescription()
	}

	post := model.FediversePost{
		ID:                p.GetId(),
		URL:               p.GetUrl(),
		PageURL:           p.GetPageUrl(),
		Author:            fromProtoAccount(p.GetAuthor()),
		Text:              p.GetText(),
		Summary:           p.GetSummary(),
		Sensitive:         p.GetSensitive(),
		ImageDescriptions: descriptions,
		PublishedAt:       p.GetPublishedAt().AsTime(),
	}

	if p.GetUpdatedAt() != nil {
		post.UpdatedAt = p.GetUpdatedAt().AsTime()
	}

	return post
}
