package rest

import (
	"context"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// Dating is "Знакомства": cards people show each other, likes and matches.
type Dating interface {
	Dating(ctx context.Context, accessToken string) (model.Dating, error)
	SaveCard(ctx context.Context, accessToken string, in model.DatingCardInput) (model.DatingCard, error)
	Answer(ctx context.Context, accessToken, userID string, like bool) (bool, error)
}

// datingCardJSON is a card; user is the public profile of its owner.
type datingCardJSON struct {
	User  *authorJSON `json:"user"`
	Goal  string      `json:"goal"`
	Seek  string      `json:"seek"`
	About string      `json:"about"`
	On    bool        `json:"on"`
}

type datingMatchJSON struct {
	User      authorJSON `json:"user"`
	MatchedAt string     `json:"matchedAt"`
}

// datingJSON is "Знакомства" of the caller: card is null before they made
// one; the cards and matches of erased pages are left out.
type datingJSON struct {
	Card    *datingCardJSON   `json:"card"`
	Deck    []datingCardJSON  `json:"deck"`
	Matches []datingMatchJSON `json:"matches"`
}

func toDatingCardJSON(c model.DatingCard, user *authorJSON) datingCardJSON {
	return datingCardJSON{User: user, Goal: c.Goal, Seek: c.Seek, About: c.About, On: c.On}
}

func (a *api) getDating(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	d, err := a.dating.Dating(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(d.Deck)+len(d.Matches))
	for _, c := range d.Deck {
		ids = append(ids, c.UserID)
	}
	for _, m := range d.Matches {
		ids = append(ids, m.UserID)
	}
	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	out := datingJSON{Deck: []datingCardJSON{}, Matches: []datingMatchJSON{}}
	if d.Card != nil {
		card := toDatingCardJSON(*d.Card, nil)
		out.Card = &card
	}
	for _, c := range d.Deck {
		if user := petOwner(c.UserID, people); user != nil {
			out.Deck = append(out.Deck, toDatingCardJSON(c, user))
		}
	}
	for _, m := range d.Matches {
		if user := petOwner(m.UserID, people); user != nil {
			out.Matches = append(out.Matches, datingMatchJSON{User: *user, MatchedAt: timeString(m.At)})
		}
	}

	writeJSON(w, http.StatusOK, out)
}

type datingCardRequest struct {
	Goal  string `json:"goal"`
	Seek  string `json:"seek"`
	About string `json:"about"`
	On    bool   `json:"on"`
}

func (a *api) putDating(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req datingCardRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	card, err := a.dating.SaveCard(r.Context(), token, model.DatingCardInput(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toDatingCardJSON(card, nil))
}

type datingAnswerRequest struct {
	UserID string `json:"userId"`
	Like   bool   `json:"like"`
}

// answerDating likes or skips a card; matched tells that the like was
// answered.
func (a *api) answerDating(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req datingAnswerRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	matched, err := a.dating.Answer(r.Context(), token, req.UserID, req.Like)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Matched bool `json:"matched"`
	}{matched})
}
