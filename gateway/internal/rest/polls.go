package rest

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// pollRequest is the poll a new post asks, openvk's "Новый опрос".
type pollRequest struct {
	Title string `json:"title"`
	// Options are the answers, 1 to 10, in the order shown.
	Options   []string `json:"options"`
	Anonymous bool     `json:"anonymous"`
	Multiple  bool     `json:"multiple"`
	// Locked: a vote cannot be taken back ("Запретить отменять свой
	// голос").
	Locked bool `json:"locked"`
	// Days is how long the poll takes votes, 1 to 31; 0 for ever.
	Days int `json:"days"`
}

type pollOptionJSON struct {
	Name  string `json:"name"`
	Votes int    `json:"votes"`
}

// pollJSON is the poll of a post: Voters is how many people voted, Voted
// the places of the options the viewer chose, empty for one who has not.
type pollJSON struct {
	Title     string           `json:"title"`
	Options   []pollOptionJSON `json:"options"`
	Voters    int              `json:"voters"`
	Anonymous bool             `json:"anonymous"`
	Multiple  bool             `json:"multiple"`
	Locked    bool             `json:"locked"`
	// EndsAt is null for a poll that never ends; Ended: it has passed.
	EndsAt *string `json:"endsAt"`
	Ended  bool    `json:"ended"`
	Voted  []int   `json:"voted"`
}

type voteRequest struct {
	// Options are the places of the chosen options, from 0: one, or
	// several of a poll with a multiple choice.
	Options []int `json:"options"`
}

func toNewPoll(p *pollRequest) *model.NewPoll {
	if p == nil {
		return nil
	}

	return &model.NewPoll{
		Title:     p.Title,
		Options:   p.Options,
		Anonymous: p.Anonymous,
		Multiple:  p.Multiple,
		Locked:    p.Locked,
		Days:      p.Days,
	}
}

func toPollJSON(p model.Poll) *pollJSON {
	options := make([]pollOptionJSON, 0, len(p.Options))
	for _, o := range p.Options {
		options = append(options, pollOptionJSON(o))
	}

	return &pollJSON{
		Title:     p.Title,
		Options:   options,
		Voters:    p.Voters,
		Anonymous: p.Anonymous,
		Multiple:  p.Multiple,
		Locked:    p.Locked,
		EndsAt:    optionalTime(p.EndsAt),
		Ended:     p.Ended,
		Voted:     append([]int{}, p.Chosen...),
	}
}

// maxPollsPerRequest is how many posts post tells the polls of at once.
const maxPollsPerRequest = 100

// pollsOf are the polls of those of the posts that ask one, by post, as
// the viewer of r sees them: read for all of them in one request, and
// none when no post asks a poll.
func (a *api) pollsOf(r *http.Request, posts ...model.Post) (map[string]*pollJSON, error) {
	var ids []string
	for _, p := range posts {
		if p.Polled {
			ids = append(ids, p.ID)
		}
	}

	if len(ids) == 0 {
		return nil, nil
	}

	token, err := optionalBearerToken(r)
	if err != nil {
		return nil, err
	}

	polls := make(map[string]*pollJSON, len(ids))
	for chunk := range slices.Chunk(unique(ids), maxPollsPerRequest) {
		found, err := a.posts.Polls(r.Context(), token, chunk)
		if err != nil {
			return nil, err
		}

		for _, p := range found {
			polls[p.PostID] = toPollJSON(p)
		}
	}

	return polls, nil
}

// asked is the response of a post just written, with the poll it asks.
func (a *api) asked(r *http.Request, p model.Post) (postResponse, error) {
	locked := []model.Post{p}
	if err := a.lockPaid(r, locked); err != nil {
		return postResponse{}, err
	}

	p = locked[0]

	polls, err := a.pollsOf(r, p)
	if err != nil {
		return postResponse{}, err
	}

	docs, err := a.postDocuments(r, p)
	if err != nil {
		return postResponse{}, err
	}

	resp := toPostResponse(p)
	resp.Poll = polls[p.ID]
	resp.Documents = documentsJSON(p.DocumentIDs, docs)

	return resp, nil
}

// votePoll chooses options of the poll of a post and returns the poll.
func (a *api) votePoll(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req voteRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	poll, err := a.posts.Vote(r.Context(), token, r.PathValue("id"), req.Options)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toPollJSON(poll))
}

// retractPollVote takes the caller's vote back, openvk's "Отменить голос".
func (a *api) retractPollVote(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	poll, err := a.posts.RetractVote(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toPollJSON(poll))
}

// listPollVoters is openvk's "Список проголосовавших": who chose the
// option ?option= of a poll that is not anonymous, newest first.
func (a *api) listPollVoters(w http.ResponseWriter, r *http.Request) {
	option, err := strconv.Atoi(r.URL.Query().Get("option"))
	if err != nil {
		a.writeError(w, r, failure.New(failure.InvalidArgument, "invalid option"))
		return
	}

	a.listLikers(w, r, func(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error) {
		return a.posts.PollVoters(ctx, id, option, cursor, limit)
	})
}
