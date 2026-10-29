package rest_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestTimeoutOfJSON(t *testing.T) {
	t.Parallel()

	var deadline time.Time
	h := rest.Timeout(time.Second)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, _ = r.Context().Deadline()
	}))

	r := httptest.NewRequest(http.MethodPost, "/api/v1/me/posts", strings.NewReader(`{"text":"hi"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), r)

	if left := time.Until(deadline); left <= 0 || left > time.Second {
		t.Fatalf("a JSON request has %v left", left)
	}
}

// An upload over a slow connection is not out of time before its body is
// read; once read, its own time starts.
func TestTimeoutOfUpload(t *testing.T) {
	t.Parallel()

	const d = time.Millisecond
	var (
		before time.Duration
		read   string
		ended  bool
	)
	h := rest.Timeout(d)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, _ := r.Context().Deadline()
		before = time.Until(deadline)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		read = string(body)

		select {
		case <-r.Context().Done():
			ended = true
		case <-time.After(5 * time.Second):
		}
	}))

	r := httptest.NewRequest(http.MethodPost, "/api/v1/me/photos", strings.NewReader("jpeg-bytes"))
	r.Header.Set("Content-Type", "image/jpeg")
	h.ServeHTTP(httptest.NewRecorder(), r)

	if before < rest.UploadReadTimeout-time.Second || read != "jpeg-bytes" || !ended {
		t.Fatalf("an upload had %v before its body, read %q, ended %v", before, read, ended)
	}
}
