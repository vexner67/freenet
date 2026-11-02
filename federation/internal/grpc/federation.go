package grpc

import (
	"context"
	"strings"

	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/apjson"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// maxBody bounds what another server may send in one activity.
const maxBody = 256 << 10

type FederationService interface {
	Status(ctx context.Context, userID uuid.UUID) (service.Status, error)
	Open(ctx context.Context, userID uuid.UUID) error
	Close(ctx context.Context, userID uuid.UUID) error
	Finger(ctx context.Context, resource string) (fedi.Person, error)
	Actor(ctx context.Context, userID uuid.UUID) (fedi.Person, error)
	Note(ctx context.Context, postID uuid.UUID) (fedi.Note, error)
	FollowerCount(ctx context.Context, userID uuid.UUID) (int, error)
	Receive(ctx context.Context, r service.SignedRequest, a fedi.Incoming, to uuid.UUID) error
	PostChanged(ctx context.Context, e service.PostEvent) error
}

type FederationHandler struct {
	federationv1.UnimplementedFederationServiceServer

	service FederationService
	site    fedi.Site
}

func NewFederationHandler(s FederationService, site fedi.Site) *FederationHandler {
	return &FederationHandler{
		service: s,
		site:    site,
	}
}

func (h *FederationHandler) GetFederation(ctx context.Context, _ *federationv1.GetFederationRequest) (*federationv1.GetFederationResponse, error) {
	st, err := h.status(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetFederationResponse{Status: st}, nil
}

func (h *FederationHandler) OpenFederation(ctx context.Context, _ *federationv1.OpenFederationRequest) (*federationv1.OpenFederationResponse, error) {
	st, err := h.status(ctx, h.service.Open)
	if err != nil {
		return nil, err
	}

	return &federationv1.OpenFederationResponse{Status: st}, nil
}

func (h *FederationHandler) CloseFederation(ctx context.Context, _ *federationv1.CloseFederationRequest) (*federationv1.CloseFederationResponse, error) {
	st, err := h.status(ctx, h.service.Close)
	if err != nil {
		return nil, err
	}

	return &federationv1.CloseFederationResponse{Status: st}, nil
}

// status is the caller's status after change, if any.
func (h *FederationHandler) status(ctx context.Context, change func(context.Context, uuid.UUID) error) (*federationv1.FederationStatus, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if change != nil {
		if err = change(ctx, userID); err != nil {
			return nil, err
		}
	}

	st, err := h.service.Status(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &federationv1.FederationStatus{
		Open:      st.Open,
		Followers: int32(st.Followers), //nolint:gosec // a count of rows
		Domain:    st.Domain,
	}, nil
}

func (h *FederationHandler) Finger(ctx context.Context, req *federationv1.FingerRequest) (*federationv1.FingerResponse, error) {
	p, err := h.service.Finger(ctx, req.GetResource())
	if err != nil {
		return nil, err
	}

	doc, err := apjson.Finger(p, h.site.Domain())
	if err != nil {
		return nil, err
	}

	return &federationv1.FingerResponse{Document: doc}, nil
}

func (h *FederationHandler) GetActor(ctx context.Context, req *federationv1.GetActorRequest) (*federationv1.GetActorResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	p, err := h.service.Actor(ctx, userID)
	if err != nil {
		return nil, err
	}

	doc, err := apjson.Person(p)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetActorResponse{Document: doc, PageUrl: p.Page.String()}, nil
}

func (h *FederationHandler) GetNote(ctx context.Context, req *federationv1.GetNoteRequest) (*federationv1.GetNoteResponse, error) {
	postID, err := parseID(req.GetPostId(), "post id")
	if err != nil {
		return nil, err
	}

	n, err := h.service.Note(ctx, postID)
	if err != nil {
		return nil, err
	}

	doc, err := apjson.Note(n)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetNoteResponse{Document: doc, PageUrl: n.Page.String()}, nil
}

func (h *FederationHandler) GetFollowers(ctx context.Context, req *federationv1.GetFollowersRequest) (*federationv1.GetFollowersResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	n, err := h.service.FollowerCount(ctx, userID)
	if err != nil {
		return nil, err
	}

	doc, err := apjson.Collection(h.site.Followers(userID), n)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetFollowersResponse{Document: doc}, nil
}

// GetOutbox tells no posts: the servers that follow a page get its posts
// as they come, and a page is read here.
func (h *FederationHandler) GetOutbox(ctx context.Context, req *federationv1.GetOutboxRequest) (*federationv1.GetOutboxResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	if _, err = h.service.Actor(ctx, userID); err != nil {
		return nil, err
	}

	doc, err := apjson.Collection(h.site.Outbox(userID), 0)
	if err != nil {
		return nil, err
	}

	return &federationv1.GetOutboxResponse{Document: doc}, nil
}

func (h *FederationHandler) Receive(ctx context.Context, req *federationv1.ReceiveRequest) (*emptypb.Empty, error) {
	if len(req.GetBody()) > maxBody {
		return nil, status.Error(codes.InvalidArgument, "activity too large")
	}

	to := uuid.Nil
	if req.GetUserId() != "" {
		id, err := parseID(req.GetUserId(), "user id")
		if err != nil {
			return nil, err
		}
		to = id
	}

	a, err := apjson.Incoming(req.GetBody())
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string, len(req.GetHeaders()))
	for name, value := range req.GetHeaders() {
		headers[strings.ToLower(name)] = value
	}

	r := service.SignedRequest{
		Method:  req.GetMethod(),
		Path:    req.GetPath(),
		Headers: headers,
		Body:    req.GetBody(),
	}

	if err = h.service.Receive(ctx, r, a, to); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

var postChanges = map[federationv1.PostChange]service.PostChange{
	federationv1.PostChange_POST_CHANGE_PUBLISHED: service.PostPublished,
	federationv1.PostChange_POST_CHANGE_EDITED:    service.PostEdited,
	federationv1.PostChange_POST_CHANGE_DELETED:   service.PostDeleted,
}

func (h *FederationHandler) PostChanged(ctx context.Context, req *federationv1.PostChangedRequest) (*emptypb.Empty, error) {
	change, ok := postChanges[req.GetChange()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown post change")
	}

	postID, err := parseID(req.GetPostId(), "post id")
	if err != nil {
		return nil, err
	}

	authorID, err := parseID(req.GetAuthorId(), "author id")
	if err != nil {
		return nil, err
	}

	if err = h.service.PostChanged(ctx, service.PostEvent{Change: change, PostID: postID, AuthorID: authorID}); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func parseID(s, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid "+what)
	}

	return id, nil
}

func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}
