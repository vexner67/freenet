package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestApps(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	game, pdf := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()

	for id, name := range map[string]string{game: "snake.html", pdf: "a.pdf"} {
		if _, err := s.client.AddDocument(s.as(t, ann), &postv1.AddDocumentRequest{Id: id, FileName: name, Size: 100}); err != nil {
			t.Fatal(err)
		}
	}

	own, err := s.client.ListApps(s.as(t, ann), &postv1.ListAppsRequest{Section: postv1.AppSection_APP_SECTION_OWN})
	if err != nil || len(own.GetApps()) != 1 || own.GetApps()[0].GetDocument().GetId() != game {
		t.Fatalf("ListApps own = %v, %v", own, err)
	}

	if _, err = s.client.InstallApp(s.as(t, bob), &postv1.InstallAppRequest{AppId: game}); err != nil {
		t.Fatalf("InstallApp: %v", err)
	}
	if _, err = s.client.InstallApp(s.as(t, bob), &postv1.InstallAppRequest{AppId: pdf}); status.Code(err) != codes.NotFound {
		t.Errorf("InstallApp of a pdf: %v", err)
	}
	if _, err = s.client.InstallApp(context.Background(), &postv1.InstallAppRequest{AppId: game}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("InstallApp without a token: %v", err)
	}

	got, err := s.client.GetApp(s.as(t, bob), &postv1.GetAppRequest{AppId: game})
	if err != nil || got.GetApp().GetUsers() != 1 || !got.GetApp().GetInstalled() {
		t.Fatalf("GetApp = %v, %v", got, err)
	}
	// A guest sees it too.
	if got, err = s.client.GetApp(context.Background(), &postv1.GetAppRequest{AppId: game}); err != nil || got.GetApp().GetInstalled() {
		t.Fatalf("GetApp as a guest = %v, %v", got, err)
	}
	if _, err = s.client.GetApp(context.Background(), &postv1.GetAppRequest{AppId: pdf}); status.Code(err) != codes.NotFound {
		t.Errorf("GetApp of a pdf: %v", err)
	}

	all, err := s.client.ListApps(s.as(t, ann), &postv1.ListAppsRequest{})
	if err != nil || len(all.GetApps()) != 1 || all.GetApps()[0].GetInstalled() {
		t.Fatalf("ListApps all = %v, %v", all, err)
	}

	if _, err = s.client.UninstallApp(s.as(t, bob), &postv1.UninstallAppRequest{AppId: game}); err != nil {
		t.Fatal(err)
	}
	if mine, err := s.client.ListApps(s.as(t, bob), &postv1.ListAppsRequest{Section: postv1.AppSection_APP_SECTION_INSTALLED}); err != nil || len(mine.GetApps()) != 0 {
		t.Fatalf("ListApps installed = %v, %v", mine, err)
	}
}
