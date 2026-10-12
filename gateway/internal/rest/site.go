package rest

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
)

// aboutResponse is what the page "о сайте" counts, as openvk's "Об этой
// инстанции": the people, those on the site now, the groups and the posts.
type aboutResponse struct {
	Users  int `json:"users"`
	Online int `json:"online"`
	Clubs  int `json:"clubs"`
	Posts  int `json:"posts"`
}

// statistics keeps what the site counted last for ttl: every count reads
// a whole table of its service, and the page needs no number of this very
// second. A zero ttl counts every time.
type statistics struct {
	ttl time.Duration

	mu      sync.Mutex
	kept    aboutResponse
	counted time.Time
}

// get answers with the numbers kept or, once they are old, with those of
// count. Whoever asks meanwhile waits for the one count instead of
// starting another; a failed count keeps nothing.
func (s *statistics) get(count func() (aboutResponse, error)) (aboutResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.counted.IsZero() && time.Since(s.counted) < s.ttl {
		return s.kept, nil
	}

	fresh, err := count()
	if err != nil {
		return aboutResponse{}, err
	}

	s.kept, s.counted = fresh, time.Now()

	return fresh, nil
}

// about counts the site for its page "о сайте"; anyone may ask.
func (a *api) about(w http.ResponseWriter, r *http.Request) {
	resp, err := a.statistics.get(func() (aboutResponse, error) { return a.count(r.Context()) })
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *api) count(ctx context.Context) (aboutResponse, error) {
	var (
		resp aboutResponse
		err  error
	)

	if resp.Users, resp.Online, err = a.users.Statistics(ctx); err != nil {
		return aboutResponse{}, fmt.Errorf("a.users.Statistics: %w", err)
	}

	if resp.Clubs, err = a.clubs.Count(ctx); err != nil {
		return aboutResponse{}, fmt.Errorf("a.clubs.Count: %w", err)
	}

	if resp.Posts, err = a.posts.Count(ctx); err != nil {
		return aboutResponse{}, fmt.Errorf("a.posts.Count: %w", err)
	}

	return resp, nil
}

// maxKnownNicknames is how many nicknames one request asks about.
const maxKnownNicknames = 100

type nicknamesResponse struct {
	Nicknames []string `json:"nicknames"`
}

// knownNicknames tells which of the nicknames asked ("?nickname=", up to
// maxKnownNicknames) are someone's: the site links "@word" of a text to a
// page only when there is one. Pages are public, and so is this.
func (a *api) knownNicknames(w http.ResponseWriter, r *http.Request) {
	asked := r.URL.Query()["nickname"]
	if len(asked) > maxKnownNicknames {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "too many nicknames"))
		return
	}

	resp := nicknamesResponse{Nicknames: []string{}}

	if len(asked) > 0 {
		known, err := a.users.KnownNicknames(r.Context(), asked)
		if err != nil {
			a.writeError(w, r, err)
			return
		}

		resp.Nicknames = append(resp.Nicknames, known...)
	}

	writeJSON(w, http.StatusOK, resp)
}
