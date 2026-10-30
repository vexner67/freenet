package service_test

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // what coturn checks.
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
