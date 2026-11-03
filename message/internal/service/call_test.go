package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // what coturn checks.
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
)

func TestCallServers(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	now := time.Unix(1000, 0)

	none := service.NewMessageService(service.Deps{}).CallServers(id, now)
	if len(none.URLs) != 0 || none.Credential != "" {
		t.Fatalf("no TURN server = %+v", none)
	}

	s := service.NewMessageService(service.Deps{Turn: service.TurnConfig{URLs: []string{"turn:example.com:3478"}, Secret: "s3"}})
	got := s.CallServers(id, now)
	if !strings.HasSuffix(got.Username, ":"+id.String()) || strings.HasPrefix(got.Username, "1000:") {
		t.Fatalf("username = %q", got.Username)
	}

	mac := hmac.New(sha1.New, []byte("s3"))
	mac.Write([]byte(got.Username))
	if got.Credential != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("credential = %q", got.Credential)
	}
}

func TestReportCall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pushed := &fakePusher{}
	s := service.NewMessageService(service.Deps{Pusher: pushed})
	ann, bob := uuid.New(), uuid.New()

	if err := s.ReportCall(ctx, ann, bob, 95); err != nil {
		t.Fatal(err)
	}
	if err := s.ReportCall(ctx, ann, bob, 0); err != nil {
		t.Fatal(err)
	}
	want := []service.Call{{CallerID: ann, PeerID: bob, Seconds: 95}, {CallerID: ann, PeerID: bob}}
	if !slices.Equal(pushed.calls, want) {
		t.Fatalf("told %+v, want %+v", pushed.calls, want)
	}

	for _, tc := range []struct {
		name    string
		peer    uuid.UUID
		seconds int
		want    error
	}{
		{"to themselves", ann, 1, message.ErrToYourself},
		{"of negative time", bob, -1, message.ErrCallLength},
		{"over a day", bob, message.MaxCallSeconds + 1, message.ErrCallLength},
	} {
		if err := s.ReportCall(ctx, ann, tc.peer, tc.seconds); !errors.Is(err, tc.want) {
			t.Errorf("a call %s: %v, want %v", tc.name, err, tc.want)
		}
	}
	if len(pushed.calls) != 2 {
		t.Errorf("refused calls told: %+v", pushed.calls)
	}
}
