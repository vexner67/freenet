package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Apps is rest.Apps over the post service.
type Apps struct {
	api postv1.PostServiceClient
}

func NewApps(api postv1.PostServiceClient) *Apps {
	return &Apps{
		api: api,
	}
}

func (a *Apps) Get(ctx context.Context, accessToken, id string) (model.App, error) {
	resp, err := a.api.GetApp(withToken(ctx, accessToken), &postv1.GetAppRequest{AppId: id})
	if err != nil {
		return model.App{}, errs.Wrap("a.api.GetApp", fromStatus(err))
	}

	return toApp(resp.GetApp()), nil
}

var appSections = map[string]postv1.AppSection{
	model.AppsAll:       postv1.AppSection_APP_SECTION_UNSPECIFIED,
	model.AppsInstalled: postv1.AppSection_APP_SECTION_INSTALLED,
	model.AppsOwn:       postv1.AppSection_APP_SECTION_OWN,
}

func (a *Apps) List(ctx context.Context, accessToken, section, cursor string, limit int) (model.AppPage, error) {
	s, ok := appSections[section]
	if !ok {
		return model.AppPage{}, failure.New(failure.InvalidArgument, "invalid app section")
	}

	resp, err := a.api.ListApps(withToken(ctx, accessToken), &postv1.ListAppsRequest{Section: s, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.AppPage{}, errs.Wrap("a.api.ListApps", fromStatus(err))
	}

	page := model.AppPage{NextCursor: resp.GetNextCursor()}
	for _, app := range resp.GetApps() {
		page.Apps = append(page.Apps, toApp(app))
	}

	return page, nil
}

func (a *Apps) Install(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.InstallApp(withToken(ctx, accessToken), &postv1.InstallAppRequest{AppId: id}); err != nil {
		return errs.Wrap("a.api.InstallApp", fromStatus(err))
	}

	return nil
}

func (a *Apps) Uninstall(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.UninstallApp(withToken(ctx, accessToken), &postv1.UninstallAppRequest{AppId: id}); err != nil {
		return errs.Wrap("a.api.UninstallApp", fromStatus(err))
	}

	return nil
}

func toApp(app *postv1.App) model.App {
	return model.App{Document: toDocument(app.GetDocument()), Users: int(app.GetUsers()), Installed: app.GetInstalled()}
}
