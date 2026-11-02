package grpc

import (
	"context"

	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/apjson"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// maxNews is the most remote posts one ListNews gives.
const maxNews = 100

type NewsService interface {
	SiteActor(ctx context.Context) (fedi.Person, error)
	Find(ctx context.Context, userID uuid.UUID, address string) (service.Account, bool, error)
	Subscriptions(ctx context.Context, userID uuid.UUID) ([]service.Account, error)
	Subscribe(ctx context.Context, userID uuid.UUID, address string) (service.Account, error)
	Unsubscribe(ctx context.Context, userID uuid.UUID, actor fedi.URL) error
	News(ctx context.Context, q service.NewsQuery) ([]service.NewsPost, error)
	Featured(ctx context.Context) ([]service.Account, error)
	Feature(ctx context.Context, address string) (service.Account, error)
	Unfeature(ctx context.Context, actor fedi.URL) error
	Picture(ctx context.Context, postID uuid.UUID, index int) (service.Media, error)
	Avatar(ctx context.Context, actor fedi.URL) (service.Media, error)
}

func (h *FederationHandler) GetSiteActor(ctx context.Context, _ *federationv1.GetSiteActorRequest) (*federationv1.GetSiteActorResponse, error) {
	p, err := h.news.SiteActor(ctx)
	if err != nil {
		return nil, err
	}

	doc, err := apjson.Person(p)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetSiteActorResponse{Document: doc}, nil
}

func (h *FederationHandler) LookupAccount(ctx context.Context, req *federationv1.LookupAccountRequest) (*federationv1.LookupAccountResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	a, subscribed, err := h.news.Find(ctx, userID, req.GetAddress())
	if err != nil {
		return nil, err
	}

	return &federationv1.LookupAccountResponse{Account: toProtoAccount(a), Subscribed: subscribed}, nil
}

func (h *FederationHandler) ListSubscriptions(ctx context.Context, _ *federationv1.ListSubscriptionsRequest) (*federationv1.ListSubscriptionsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	accounts, err := h.news.Subscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &federationv1.ListSubscriptionsResponse{Accounts: toProtoAccounts(accounts)}, nil
}

func (h *FederationHandler) Subscribe(ctx context.Context, req *federationv1.SubscribeRequest) (*federationv1.SubscribeResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	a, err := h.news.Subscribe(ctx, userID, req.GetAddress())
	if err != nil {
		return nil, err
	}

	return &federationv1.SubscribeResponse{Account: toProtoAccount(a)}, nil
}

func (h *FederationHandler) Unsubscribe(ctx context.Context, req *federationv1.UnsubscribeRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	actor, err := fedi.NewURL(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.news.Unsubscribe(ctx, userID, actor); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *FederationHandler) ListNews(ctx context.Context, req *federationv1.ListNewsRequest) (*federationv1.ListNewsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	q, err := fromProtoNewsQuery(req)
	if err != nil {
		return nil, err
	}
	q.Reader = userID

	posts, err := h.news.News(ctx, q)
	if err != nil {
		return nil, err
	}

	out := make([]*federationv1.NewsPost, len(posts))
	for i, p := range posts {
		out[i] = toProtoNewsPost(p)
	}

	return &federationv1.ListNewsResponse{Posts: out}, nil
}

func (h *FederationHandler) ListFeatured(ctx context.Context, _ *federationv1.ListFeaturedRequest) (*federationv1.ListFeaturedResponse, error) {
	accounts, err := h.news.Featured(ctx)
	if err != nil {
		return nil, err
	}

	return &federationv1.ListFeaturedResponse{Accounts: toProtoAccounts(accounts)}, nil
}

func (h *FederationHandler) Feature(ctx context.Context, req *federationv1.FeatureRequest) (*federationv1.FeatureResponse, error) {
	a, err := h.news.Feature(ctx, req.GetAddress())
	if err != nil {
		return nil, err
	}

	return &federationv1.FeatureResponse{Account: toProtoAccount(a)}, nil
}

func (h *FederationHandler) Unfeature(ctx context.Context, req *federationv1.UnfeatureRequest) (*emptypb.Empty, error) {
	actor, err := fedi.NewURL(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.news.Unfeature(ctx, actor); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *FederationHandler) GetPicture(ctx context.Context, req *federationv1.GetPictureRequest) (*federationv1.GetPictureResponse, error) {
	m, err := h.picture(ctx, req)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetPictureResponse{Body: m.Body, ContentType: m.ContentType}, nil
}

func (h *FederationHandler) picture(ctx context.Context, req *federationv1.GetPictureRequest) (service.Media, error) {
	if req.GetAccountId() != "" {
		actor, err := fedi.NewURL(req.GetAccountId())
		if err != nil {
			return service.Media{}, err
		}

		return h.news.Avatar(ctx, actor)
	}

	postID, err := parseID(req.GetPostId(), "post id")
	if err != nil {
		return service.Media{}, err
	}

	return h.news.Picture(ctx, postID, int(req.GetIndex()))
}

var newsScopes = map[federationv1.NewsScope]bool{
	federationv1.NewsScope_NEWS_SCOPE_SUBSCRIBED: false,
	federationv1.NewsScope_NEWS_SCOPE_FEATURED:   true,
}

func fromProtoNewsQuery(req *federationv1.ListNewsRequest) (service.NewsQuery, error) {
	featured, ok := newsScopes[req.GetScope()]
	if !ok {
		return service.NewsQuery{}, status.Error(codes.InvalidArgument, "unknown news scope")
	}

	if req.GetLimit() < 1 || req.GetLimit() > maxNews {
		return service.NewsQuery{}, status.Error(codes.InvalidArgument, "invalid limit")
	}

	q := service.NewsQuery{Featured: featured, HideSensitive: req.GetHideSensitive(), Limit: int(req.GetLimit())}

	if req.GetBefore() != "" {
		before, err := parseID(req.GetBefore(), "cursor")
		if err != nil {
			return service.NewsQuery{}, err
		}
		q.Before = before
	}

	return q, nil
}
