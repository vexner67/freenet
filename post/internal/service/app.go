package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// AppUsers is how many people added an app and whether the viewer did.
type AppUsers struct {
	Count     int
	Installed bool
}

// AppRepository keeps who added which HTML widget to their "Приложения".
type AppRepository interface {
	// Install adds app to userID's apps; adding it again changes nothing.
	Install(ctx context.Context, userID, app uuid.UUID, at time.Time) error
	Uninstall(ctx context.Context, userID, app uuid.UUID) error
	// List returns the widgets of section, newest first, those before
	// the cursor; userID is whose apps installed and own are.
	List(ctx context.Context, section post.AppSection, userID, before uuid.UUID, limit int) ([]*post.Document, error)
	// Users tells, of each of apps, how many added it and whether
	// viewerID did (uuid.Nil: a guest).
	Users(ctx context.Context, viewerID uuid.UUID, apps []uuid.UUID) (map[uuid.UUID]AppUsers, error)
	// DeleteByUser forgets the apps a user added.
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// App is an HTML widget as "Приложения" show it.
type App struct {
	Document *post.Document
	AppUsers
}

type AppPage struct {
	Apps       []App
	NextCursor uuid.UUID
}

// AppService is openvk's "Приложения": HTML widgets people keep at hand.
type AppService struct {
	apps      AppRepository
	documents GameDocuments
	now       func() time.Time
}

func NewAppService(apps AppRepository, documents GameDocuments) *AppService {
	return &AppService{apps: apps, documents: documents, now: time.Now}
}

// Get returns the widget id as an app; viewerID may be uuid.Nil.
func (s *AppService) Get(ctx context.Context, viewerID, id uuid.UUID) (App, error) {
	d, err := s.documents.Get(ctx, id)
	if err != nil {
		return App{}, errs.Wrap("s.documents.Get", err)
	}

	if !d.IsGame() {
		return App{}, post.ErrGameNotFound
	}

	apps, err := s.withUsers(ctx, viewerID, []*post.Document{d})
	if err != nil {
		return App{}, errs.Wrap("s.withUsers", err)
	}

	return apps[0], nil
}

// Install is "Добавить к себе".
func (s *AppService) Install(ctx context.Context, userID, id uuid.UUID) error {
	d, err := s.documents.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.documents.Get", err)
	}

	if !d.IsGame() {
		return post.ErrGameNotFound
	}

	return errs.Wrap("s.apps.Install", s.apps.Install(ctx, userID, id, s.now()))
}

// Uninstall is "Убрать из своих"; an app not added stays so.
func (s *AppService) Uninstall(ctx context.Context, userID, id uuid.UUID) error {
	return errs.Wrap("s.apps.Uninstall", s.apps.Uninstall(ctx, userID, id))
}

// List pages a section for viewerID, newest first.
func (s *AppService) List(ctx context.Context, viewerID uuid.UUID, section post.AppSection, cursor uuid.UUID, size int) (AppPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra tells whether there is a next page.
	found, err := s.apps.List(ctx, section, viewerID, cursor, size+1)
	if err != nil {
		return AppPage{}, errs.Wrap("s.apps.List", err)
	}

	var page AppPage
	if len(found) > size {
		found = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Apps, err = s.withUsers(ctx, viewerID, found); err != nil {
		return AppPage{}, errs.Wrap("s.withUsers", err)
	}

	return page, nil
}

func (s *AppService) withUsers(ctx context.Context, viewerID uuid.UUID, docs []*post.Document) ([]App, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(docs))
	for i, d := range docs {
		ids[i] = d.ID()
	}

	users, err := s.apps.Users(ctx, viewerID, ids)
	if err != nil {
		return nil, errs.Wrap("s.apps.Users", err)
	}

	apps := make([]App, len(docs))
	for i, d := range docs {
		apps[i] = App{Document: d, AppUsers: users[d.ID()]}
	}

	return apps, nil
}
