package rest

import (
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

const (
	// friendPages is how many pages of friends the records are looked
	// among: the post service takes up to 1000 players.
	friendPages = 19
	// recordsShown is how many lines "Рекорды" shows.
	recordsShown = 10
)

type saveGameRequest struct {
	Data string `json:"data"`
}

type gameSaveResponse struct {
	Found bool   `json:"found"`
	Data  string `json:"data"`
}

type scoreGameRequest struct {
	Score int64 `json:"score"`
}

type gameScoreResponse struct {
	Best int64 `json:"best"`
}

type gameRecordJSON struct {
	User  publicProfileResponse `json:"user"`
	Score int64                 `json:"score"`
}

type gameRecordsResponse struct {
	Records []gameRecordJSON `json:"records"`
}

// loadGame is what the HTML widget of the path saved of the caller, which
// htmlapp.js hands to the widget: the widget never sees who plays.
func (a *api) loadGame(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	data, found, err := a.games.Load(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, gameSaveResponse{Found: found, Data: data})
}

// saveGame keeps what the widget asked to save, in place of the one before.
func (a *api) saveGame(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req saveGameRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.games.Save(r.Context(), token, r.PathValue("id"), req.Data); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// scoreGame keeps a result of the caller; the best one stays.
func (a *api) scoreGame(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req scoreGameRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	best, err := a.games.Score(r.Context(), token, r.PathValue("id"), req.Score)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, gameScoreResponse{Best: best})
}

// gameRecords is "Рекорды" under a widget: the best results of the
// caller's friends and the caller, the best first.
func (a *api) gameRecords(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	people, err := a.allFriends(r, token, me.ID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := []string{me.ID}
	for id := range people {
		ids = append(ids, id)
	}

	records, err := a.games.Records(r.Context(), token, r.PathValue("id"), ids, recordsShown)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.addProfiles(r, people, records); err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := gameRecordsResponse{Records: make([]gameRecordJSON, 0, len(records))}
	for _, rec := range records {
		if p, ok := people[rec.UserID]; ok && !p.Deactivated {
			resp.Records = append(resp.Records, gameRecordJSON{User: toPublicProfileResponse(p), Score: rec.Score})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// allFriends are the friends of userID by ID, as many as friendPages hold.
func (a *api) allFriends(r *http.Request, token, userID string) (map[string]model.PublicProfile, error) {
	people := make(map[string]model.PublicProfile)
	q := model.FriendQuery{UserID: userID, Section: model.FriendsAll, Limit: maxPageSize}

	for range friendPages {
		page, err := a.users.ListFriends(r.Context(), token, q)
		if err != nil {
			return nil, err
		}

		for _, p := range page.Users {
			people[p.ID] = p
		}

		if page.NextCursor == "" {
			break
		}
		q.Cursor = page.NextCursor
	}

	return people, nil
}

// addProfiles adds to people the profiles of the records' players they
// do not have yet: the caller.
func (a *api) addProfiles(r *http.Request, people map[string]model.PublicProfile, records []model.GameRecord) error {
	var missing []string
	for _, rec := range records {
		if _, ok := people[rec.UserID]; !ok {
			missing = append(missing, rec.UserID)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	profiles, err := a.users.GetProfiles(r.Context(), missing)
	if err != nil {
		return err
	}

	for _, p := range profiles {
		people[p.ID] = p
	}

	return nil
}
