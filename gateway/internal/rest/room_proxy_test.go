package rest_test

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mediaAccess struct {
	rest.Rooms
	allowed bool
}

func (m *mediaAccess) AuthorizeMedia(context.Context, string) error {
	if !m.allowed {
		return failure.New(failure.PermissionDenied, "not in room")
	}
	return nil
}
func TestMediaProxyChecksCurrentMembership(t *testing.T) {
	t.Parallel()
	requests := 0
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if _, err := io.WriteString(w, "signal"); err != nil {
			t.Error(err)
		}
	}))
	defer media.Close()
	access := &mediaAccess{allowed: true}
	srv := mustServer(rest.Config{LiveKitURL: media.URL, RequestTimeout: time.Second}, rest.Backends{Rooms: access}, prometheus.NewRegistry())
	for _, test := range []struct {
		path    string
		allowed bool
		code    int
	}{{"/rtc", true, 401}, {"/rtc?access_token=cached", true, 200}, {"/rtc?access_token=cached", false, 403}, {"/rtc/validate?access_token=cached", false, 403}, {"/twirp/livekit.RoomService/DeleteRoom", true, 404}} {
		access.allowed = test.allowed
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
		if w.Code != test.code {
			t.Fatal(test.path, w.Code, w.Body.String())
		}
	}
	if requests != 1 {
		t.Fatalf("unauthorized request reached media: %d", requests)
	}
}
