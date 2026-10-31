package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

const (
	batchPath = "/api/v1/batch"

	// maxBatch is how many reads one batch makes: more than a page asks
	// for at once.
	maxBatch = 20
)

type batchRequest struct {
	Requests []batchRead `json:"requests"`
}

type batchRead struct {
	Path string `json:"path"`
}

type batchResponse struct {
	Responses []batchAnswer `json:"responses"`
}

// batchAnswer is what the read would have answered by itself: its status
// and its JSON body, none for 204.
type batchAnswer struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// batch makes several reads of the API at once, as the page that asks
// for them would make them one by one: each with the caller's headers, so
// with its token, through the same routes. A page then waits for one
// round trip to the server instead of one per block, and draws its blocks
// together.
func (a *api) batch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	reads, err := batchReads(req.Requests)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	answers := make([]batchAnswer, len(reads))

	var wg sync.WaitGroup

	for i, u := range reads {
		wg.Go(func() {
			answers[i] = a.batchRead(r, u)
		})
	}

	wg.Wait()

	writeJSON(w, http.StatusOK, batchResponse{Responses: answers})
}

// batchReads are the paths of the reads, each a GET of the API other than
// a batch and the event stream.
func batchReads(reads []batchRead) ([]*url.URL, error) {
	if len(reads) == 0 || len(reads) > maxBatch {
		return nil, failure.New(failure.InvalidArgument, "a batch makes 1 to 20 requests")
	}

	out := make([]*url.URL, 0, len(reads))

	for _, read := range reads {
		u, err := url.ParseRequestURI(read.Path)
		if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/api/v1/") ||
			u.Path == batchPath || isStream(u.Path) {
			return nil, failure.New(failure.InvalidArgument, "invalid path in the batch")
		}

		out = append(out, u)
	}

	return out, nil
}

// batchRead serves one read of r's batch.
func (a *api) batchRead(r *http.Request, u *url.URL) (answer batchAnswer) {
	start := time.Now()
	rec := &recorded{header: http.Header{}}

	// A panic of a read must not take the server down with the goroutine:
	// recoverPanics answers it, and this is for what it does not.
	defer func() {
		if v := recover(); v != nil {
			a.logger.ErrorContext(r.Context(), "panic", "path", u.Path, "value", v)
			answer = batchAnswer{Status: http.StatusInternalServerError}
		}
	}()

	a.recoverPanics(a.batched).ServeHTTP(rec, readOf(r, u))

	a.logger.InfoContext(r.Context(), "http request",
		"method", http.MethodGet,
		"path", u.Path,
		"status", rec.code(),
		"duration_ms", time.Since(start).Milliseconds(),
		"batch", true,
	)

	answer.Status = rec.code()
	if strings.HasPrefix(rec.header.Get("Content-Type"), "application/json") && rec.body.Len() > 0 {
		answer.Body = json.RawMessage(bytes.TrimSpace(rec.body.Bytes()))
	}

	return answer
}

// readOf is the GET of u as the batch r would have sent it.
func readOf(r *http.Request, u *url.URL) *http.Request {
	read := r.Clone(r.Context())
	read.Method = http.MethodGet
	read.URL = u
	read.RequestURI = u.RequestURI()
	read.Body = http.NoBody
	read.ContentLength = 0
	read.Pattern = ""
	read.Header.Del("Content-Type")
	read.Header.Del("Content-Length")

	return read
}

// recorded keeps what a read answers, for the batch to send.
type recorded struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorded) Header() http.Header {
	return r.header
}

func (r *recorded) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *recorded) Write(p []byte) (int, error) {
	r.WriteHeader(http.StatusOK)

	return r.body.Write(p)
}

func (r *recorded) code() int {
	if r.status == 0 {
		return http.StatusOK
	}

	return r.status
}
