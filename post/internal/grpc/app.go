package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AppService interface {
	Get(ctx context.Context, viewerID, id uuid.UUID) (service.App, error)
	List(ctx context.Context, viewerID uuid.UUID, section post.AppSection, cursor uuid.UUID, size int) (service.AppPage, error)
	Install(ctx context.Context, userID, id uuid.UUID) error
	Uninstall(ctx context.Context, userID, id uuid.UUID) error
}

// GetApp is an HTML widget as "Приложения" show it, to anyone.
func (h *PostHandler) GetApp(ctx context.Context, req *postv1.GetAppRequest) (*postv1.GetAppResponse, error) {
	id, err := parseDocumentID(req.GetAppId())
	if err != nil {
		return nil, err
	}

	app, err := h.appService.Get(ctx, viewerID(ctx), id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetAppResponse{App: toProtoApp(app)}, nil
}

func (h *PostHandler) ListApps(ctx context.Context, req *postv1.ListAppsRequest) (*postv1.ListAppsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.appService.List(ctx, userID, fromProtoAppSection(req.GetSection()), cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListAppsResponse{Apps: make([]*postv1.App, 0, len(page.Apps))}
	for _, app := range page.Apps {
		resp.Apps = append(resp.Apps, toProtoApp(app))
	}
	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *PostHandler) InstallApp(ctx context.Context, req *postv1.InstallAppRequest) (*emptypb.Empty, error) {
	userID, id, err := gameCall(ctx, req.GetAppId())
	if err != nil {
		return nil, err
	}

	if err = h.appService.Install(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) UninstallApp(ctx context.Context, req *postv1.UninstallAppRequest) (*emptypb.Empty, error) {
	userID, id, err := gameCall(ctx, req.GetAppId())
	if err != nil {
		return nil, err
	}

	if err = h.appService.Uninstall(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func fromProtoAppSection(s postv1.AppSection) post.AppSection {
	switch s {
	case postv1.AppSection_APP_SECTION_INSTALLED:
		return post.AppsInstalled
	case postv1.AppSection_APP_SECTION_OWN:
		return post.AppsOwn
	default:
		return post.AppsAll
	}
}

func toProtoApp(app service.App) *postv1.App {
	return &postv1.App{
		Document:  toProtoDocument(app.Document),
		Users:     int32(app.Count),
		Installed: app.Installed,
	}
}
