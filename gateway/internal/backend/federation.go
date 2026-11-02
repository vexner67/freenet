package backend

import (
	"context"

	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
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
