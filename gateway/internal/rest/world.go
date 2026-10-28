package rest

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// World is "Мир", which the world service keeps: the map and its states,
// what people do there, and the generation of maps by the administrators
// of the site, whom the gateway checks itself.
type World interface {
	Map(ctx context.Context, accessToken string) (model.WorldMap, error)
	Atlas(ctx context.Context, worldID string) (model.Atlas, error)
	Raster(ctx context.Context, worldID string) ([]byte, error)
	Tile(ctx context.Context, worldID string, z, x, y int) ([]byte, error)
	State(ctx context.Context, accessToken, stateID string) (model.StateView, error)
	Events(ctx context.Context, stateID, before string, limit int) ([]model.WorldEvent, error)
	Citizenship(ctx context.Context, userID string) (model.State, string, error)

	Found(ctx context.Context, accessToken string, f model.Founding) (string, error)
	Edit(ctx context.Context, accessToken string, l model.StateLook) error
	Apply(ctx context.Context, accessToken, stateID string) error
	Withdraw(ctx context.Context, accessToken string) error
	Answer(ctx context.Context, accessToken, userID string, accept bool) error
	Leave(ctx context.Context, accessToken string) error
	Expel(ctx context.Context, accessToken, userID string) error
	Appoint(ctx context.Context, accessToken, userID, role string) error
	Abdicate(ctx context.Context, accessToken, heirID string) error
	Colonize(ctx context.Context, accessToken string, province int) error
	Donate(ctx context.Context, accessToken string, votes int64) error

	Worlds(ctx context.Context) ([]model.World, error)
	Create(ctx context.Context, seed uint64, grid int) (model.World, error)
	Open(ctx context.Context, worldID string) error
	End(ctx context.Context) error
	Rules(ctx context.Context) (model.WorldRules, error)
	SetRules(ctx context.Context, r model.WorldRules) error
}

type worldJSON struct {
	ID        string     `json:"id"`
	Seed      string     `json:"seed"`
	Grid      int        `json:"grid"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	OpenedAt  *time.Time `json:"openedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

type worldRulesJSON struct {
	FoundCost       int64 `json:"foundCost"`
	FoundTreasury   int64 `json:"foundTreasury"`
	FoundLands      int   `json:"foundLands"`
	CapitalDistance int   `json:"capitalDistance"`
	ProtectionDays  int   `json:"protectionDays"`
	ColonizeCost    int64 `json:"colonizeCost"`
	ColonizeHours   int   `json:"colonizeHours"`
	Income          int64 `json:"income"`
}

type flagJSON struct {
	Pattern     string `json:"pattern"`
	Colors      []int  `json:"colors"`
	Emblem      string `json:"emblem"`
	EmblemColor int    `json:"emblemColor"`
	EmblemAt    string `json:"emblemAt"`
}

// stateJSON: treasury and income are in hundredths of a vote, population
// in thousands.
type stateJSON struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Adjective      string      `json:"adjective"`
	Motto          string      `json:"motto"`
	Color          int         `json:"color"`
	Flag           flagJSON    `json:"flag"`
	Government     string      `json:"government"`
	Ruler          *authorJSON `json:"ruler"`
	Capital        int         `json:"capital"`
	Treasury       int64       `json:"treasury"`
	FoundedAt      time.Time   `json:"foundedAt"`
	ProtectedUntil time.Time   `json:"protectedUntil"`
	ClubID         string      `json:"clubId"`
	Lands          int         `json:"lands"`
	Population     int         `json:"population"`
	Citizens       int         `json:"citizens"`
	Income         int64       `json:"income"`
}

type colonyJSON struct {
	Province int       `json:"province"`
	StateID  string    `json:"stateId"`
	Until    time.Time `json:"until"`
}

type standingJSON struct {
	StateID   string `json:"stateId"`
	Role      string `json:"role"`
	AppliedTo string `json:"appliedTo"`
}

type worldMapJSON struct {
	World    worldJSON      `json:"world"`
	Rules    worldRulesJSON `json:"rules"`
	Hour     time.Time      `json:"hour"`
	States   []stateJSON    `json:"states"`
	Owners   []int          `json:"owners"`
	Colonies []colonyJSON   `json:"colonies"`
	Viewer   standingJSON   `json:"viewer"`
}

type provinceLinkJSON struct {
	To    int  `json:"to"`
	River bool `json:"river,omitempty"`
	Sea   bool `json:"sea,omitempty"`
}

type provinceJSON struct {
	ID         int                `json:"id"`
	Sea        bool               `json:"sea,omitempty"`
	Name       string             `json:"name,omitempty"`
	Terrain    string             `json:"terrain,omitempty"`
	Biome      string             `json:"biome,omitempty"`
	Resources  []string           `json:"resources,omitempty"`
	CenterX    int                `json:"x"`
	CenterY    int                `json:"y"`
	Area       int                `json:"area"`
	Coastal    bool               `json:"coastal,omitempty"`
	Culture    int                `json:"culture,omitempty"`
	Population int                `json:"population,omitempty"`
	FreeCity   bool               `json:"freeCity,omitempty"`
	Links      []provinceLinkJSON `json:"links"`
}

type atlasJSON struct {
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	Provinces []provinceJSON `json:"provinces"`
}

type citizenJSON struct {
	User     authorJSON `json:"user"`
	Role     string     `json:"role"`
	JoinedAt time.Time  `json:"joinedAt"`
}

type applicationJSON struct {
	User authorJSON `json:"user"`
	At   time.Time  `json:"at"`
}

type budgetHourJSON struct {
	Hour         time.Time `json:"hour"`
	Income       int64     `json:"income"`
	Donations    int64     `json:"donations"`
	Founding     int64     `json:"founding"`
	Colonization int64     `json:"colonization"`
}

type stateViewJSON struct {
	State        stateJSON         `json:"state"`
	Lands        []int             `json:"lands"`
	Citizens     []citizenJSON     `json:"citizens"`
	Colonies     []colonyJSON      `json:"colonies"`
	Applications []applicationJSON `json:"applications"`
	ColonyCost   int64             `json:"colonyCost"`
	Budget       []budgetHourJSON  `json:"budget"`
	Viewer       standingJSON      `json:"viewer"`
}

type worldEventJSON struct {
	ID       string      `json:"id"`
	Kind     string      `json:"kind"`
	StateID  string      `json:"stateId"`
	User     *authorJSON `json:"user"`
	Province int         `json:"province"`
	Name     string      `json:"name"`
	Role     string      `json:"role"`
	At       time.Time   `json:"at"`
}

// world is the open world as the map shows it, with the rulers of its
// states.
func (a *api) world(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	m, err := a.worlds.Map(r.Context(), token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	rulers := make([]string, 0, len(m.States))
	for _, s := range m.States {
		rulers = append(rulers, s.RulerID)
	}

	people, err := a.people(r, rulers)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := worldMapJSON{
		World:    toWorldJSON(m.World),
		Rules:    toWorldRulesJSON(m.Rules),
		Hour:     m.Hour,
		States:   make([]stateJSON, 0, len(m.States)),
		Owners:   m.Owners,
		Colonies: toColoniesJSON(m.Colonies),
		Viewer:   toStandingJSON(m.Viewer),
	}
	for _, s := range m.States {
		resp.States = append(resp.States, toStateJSON(s, people))
	}

	writeJSON(w, http.StatusOK, resp)
}

// worldAtlas, worldRaster and worldTile are the map of a world: they never
// change, so they are cached for good under the world's ID.
func (a *api) worldAtlas(w http.ResponseWriter, r *http.Request) {
	atlas, err := a.worlds.Atlas(r.Context(), r.PathValue("world"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := atlasJSON{Width: atlas.Width, Height: atlas.Height, Provinces: make([]provinceJSON, 0, len(atlas.Provinces))}
	for _, p := range atlas.Provinces {
		links := make([]provinceLinkJSON, 0, len(p.Links))
		for _, l := range p.Links {
			links = append(links, provinceLinkJSON(l))
		}

		resp.Provinces = append(resp.Provinces, provinceJSON{
			ID:         p.ID,
			Sea:        p.Sea,
			Name:       p.Name,
			Terrain:    p.Terrain,
			Biome:      p.Biome,
			Resources:  p.Resources,
			CenterX:    p.CenterX,
			CenterY:    p.CenterY,
			Area:       p.Area,
			Coastal:    p.Coastal,
			Culture:    p.Culture,
			Population: p.Population,
			FreeCity:   p.FreeCity,
			Links:      links,
		})
	}

	w.Header().Set("Cache-Control", cacheForever)
	writeJSON(w, http.StatusOK, resp)
}

// worldRaster is the province of every pixel of the map, little-endian
// uint16, gzipped as the world service keeps it: the browser unpacks it
// by Content-Encoding.
func (a *api) worldRaster(w http.ResponseWriter, r *http.Request) {
	raster, err := a.worlds.Raster(r.Context(), r.PathValue("world"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Cache-Control", cacheForever)
	_, _ = w.Write(raster)
}

func (a *api) worldTile(w http.ResponseWriter, r *http.Request) {
	var place [3]int
	for i, name := range []string{"z", "x", "y"} {
		n, err := strconv.Atoi(r.PathValue(name))
		if err != nil || n < 0 || n > maxTileIndex {
			a.writeError(w, r, failure.New(failure.NotFound, "tile not found"))
			return
		}
		place[i] = n
	}

	png, err := a.worlds.Tile(r.Context(), r.PathValue("world"), place[0], place[1], place[2])
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", cacheForever)
	_, _ = w.Write(png)
}

// maxTileIndex bounds the zoom and the place of a tile, far above any map.
const maxTileIndex = 1 << 12

// worldState is the page of a state: its citizens, and for those who
// answer them its applications.
func (a *api) worldState(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	v, err := a.worlds.State(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := []string{v.State.RulerID}
	for _, c := range v.Citizens {
		ids = append(ids, c.UserID)
	}
	for _, ap := range v.Applications {
		ids = append(ids, ap.UserID)
	}

	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := stateViewJSON{
		State:        toStateJSON(v.State, people),
		Lands:        v.Lands,
		Citizens:     make([]citizenJSON, 0, len(v.Citizens)),
		Colonies:     toColoniesJSON(v.Colonies),
		Applications: make([]applicationJSON, 0, len(v.Applications)),
		ColonyCost:   v.ColonyCost,
		Budget:       make([]budgetHourJSON, 0, len(v.Budget)),
		Viewer:       toStandingJSON(v.Viewer),
	}
	if resp.Lands == nil {
		resp.Lands = []int{}
	}
	// Those erased are gone from the state by now; a stale answer of
	// user leaves them out rather than show nobody.
	for _, c := range v.Citizens {
		if p, ok := people[c.UserID]; ok {
			resp.Citizens = append(resp.Citizens, citizenJSON{User: toAuthorJSON(p), Role: c.Role, JoinedAt: c.JoinedAt})
		}
	}
	for _, ap := range v.Applications {
		if p, ok := people[ap.UserID]; ok {
			resp.Applications = append(resp.Applications, applicationJSON{User: toAuthorJSON(p), At: ap.At})
		}
	}
	for _, b := range v.Budget {
		resp.Budget = append(resp.Budget, budgetHourJSON(b))
	}

	writeJSON(w, http.StatusOK, resp)
}

// worldEvents is the news of the world (?state= of one state), newest
// first; ?before= is the ID of the last one seen.
func (a *api) worldEvents(w http.ResponseWriter, r *http.Request) {
	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	q := r.URL.Query()

	events, err := a.worlds.Events(r.Context(), q.Get("state"), q.Get("before"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ids := make([]string, 0, len(events))
	for _, e := range events {
		if e.UserID != "" {
			ids = append(ids, e.UserID)
		}
	}

	people, err := a.people(r, ids)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := struct {
		Events []worldEventJSON `json:"events"`
	}{Events: make([]worldEventJSON, 0, len(events))}

	for _, e := range events {
		resp.Events = append(resp.Events, worldEventJSON{
			ID:       e.ID,
			Kind:     e.Kind,
			StateID:  e.StateID,
			User:     shownPerson(people, e.UserID),
			Province: e.Province,
			Name:     e.Name,
			Role:     e.Role,
			At:       e.At,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// userCitizenship is the state of a person, for the block on their page;
// state is null for none.
func (a *api) userCitizenship(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	profile, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := struct {
		State *stateJSON `json:"state"`
		Role  string     `json:"role"`
	}{}

	state, role, err := a.worlds.Citizenship(r.Context(), profile.ID)
	if err != nil && failure.From(err).Kind != failure.NotFound {
		a.writeError(w, r, err)
		return
	}

	if state.ID != "" {
		s := toStateJSON(state, nil)
		resp.State, resp.Role = &s, role
	}

	writeJSON(w, http.StatusOK, resp)
}

type foundingJSON struct {
	Name       string   `json:"name"`
	Adjective  string   `json:"adjective"`
	Motto      string   `json:"motto"`
	Color      int      `json:"color"`
	Flag       flagJSON `json:"flag"`
	Government string   `json:"government"`
	Capital    int      `json:"capital"`
}

func (a *api) foundState(w http.ResponseWriter, r *http.Request) {
	var req foundingJSON

	token, ok := a.worldRequest(w, r, &req)
	if !ok {
		return
	}

	id, err := a.worlds.Found(r.Context(), token, model.Founding{
		Name:       req.Name,
		Adjective:  req.Adjective,
		Motto:      req.Motto,
		Color:      req.Color,
		Flag:       model.Flag(req.Flag),
		Government: req.Government,
		Capital:    req.Capital,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, struct {
		ID string `json:"id"`
	}{ID: id})
}

func (a *api) editState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Adjective string   `json:"adjective"`
		Motto     string   `json:"motto"`
		Color     int      `json:"color"`
		Flag      flagJSON `json:"flag"`
	}

	token, ok := a.worldRequest(w, r, &req)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Edit(r.Context(), token, model.StateLook{
		Adjective: req.Adjective,
		Motto:     req.Motto,
		Color:     req.Color,
		Flag:      model.Flag(req.Flag),
	}))
}

func (a *api) applyCitizenship(w http.ResponseWriter, r *http.Request) {
	token, ok := a.worldRequest(w, r, nil)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Apply(r.Context(), token, r.PathValue("id")))
}

func (a *api) withdrawApplication(w http.ResponseWriter, r *http.Request) {
	token, ok := a.worldRequest(w, r, nil)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Withdraw(r.Context(), token))
}

// answerApplication takes (PUT) or turns away (DELETE) the application
// of a user to the caller's state.
func (a *api) answerApplication(w http.ResponseWriter, r *http.Request) {
	token, ok := a.worldRequest(w, r, nil)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Answer(r.Context(), token, r.PathValue("user"), r.Method == http.MethodPut))
}

func (a *api) leaveState(w http.ResponseWriter, r *http.Request) {
	token, ok := a.worldRequest(w, r, nil)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Leave(r.Context(), token))
}

func (a *api) expelCitizen(w http.ResponseWriter, r *http.Request) {
	token, ok := a.worldRequest(w, r, nil)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Expel(r.Context(), token, r.PathValue("user")))
}

func (a *api) appointCitizen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}

	token, ok := a.worldRequest(w, r, &req)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Appoint(r.Context(), token, r.PathValue("user"), req.Role))
}

func (a *api) abdicate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HeirID string `json:"heirId"`
	}

	token, ok := a.worldRequest(w, r, &req)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Abdicate(r.Context(), token, req.HeirID))
}

func (a *api) colonize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Province int `json:"province"`
	}

	token, ok := a.worldRequest(w, r, &req)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Colonize(r.Context(), token, req.Province))
}

func (a *api) donateToState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Votes int64 `json:"votes"`
	}

	token, ok := a.worldRequest(w, r, &req)
	if !ok {
		return
	}

	a.worldDone(w, r, a.worlds.Donate(r.Context(), token, req.Votes))
}

// worldRequest reads the token and, unless body is nil, the request body
// of an action in the world; false: the error is written.
func (a *api) worldRequest(w http.ResponseWriter, r *http.Request, body any) (string, bool) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return "", false
	}

	if body != nil {
		if err = decode(w, r, body); err != nil {
			a.writeError(w, r, err)
			return "", false
		}
	}

	return token, true
}

// worldDone answers an action: 204, or its error.
func (a *api) worldDone(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// worldAdmin lets the administrators of the site alone through: the world
// service takes their calls from the gateway, not from them.
func (a *api) worldAdmin(w http.ResponseWriter, r *http.Request) bool {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return false
	}

	if _, err = a.adminID(r, token); err != nil {
		a.writeError(w, r, err)
		return false
	}

	return true
}

// listWorlds is "Мир" of the admin panel: the maps made and the rules.
func (a *api) listWorlds(w http.ResponseWriter, r *http.Request) {
	if !a.worldAdmin(w, r) {
		return
	}

	worlds, err := a.worlds.Worlds(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	rules, err := a.worlds.Rules(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := struct {
		Worlds []worldJSON    `json:"worlds"`
		Rules  worldRulesJSON `json:"rules"`
	}{Worlds: make([]worldJSON, 0, len(worlds)), Rules: toWorldRulesJSON(rules)}

	for _, world := range worlds {
		resp.Worlds = append(resp.Worlds, toWorldJSON(world))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createWorld starts making a map; seed "" takes a random one. The seed is
// a string: JSON numbers lose the digits of a large one.
func (a *api) createWorld(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed string `json:"seed"`
		Grid int    `json:"grid"`
	}
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	var seed uint64
	if req.Seed != "" {
		var err error
		if seed, err = strconv.ParseUint(req.Seed, 10, 64); err != nil {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "seed must be a number"))
			return
		}
	}

	if !a.worldAdmin(w, r) {
		return
	}

	world, err := a.worlds.Create(r.Context(), seed, req.Grid)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toWorldJSON(world))
}

// openWorld opens a ready map for the game, ending the one open now.
func (a *api) openWorld(w http.ResponseWriter, r *http.Request) {
	if !a.worldAdmin(w, r) {
		return
	}

	a.worldDone(w, r, a.worlds.Open(r.Context(), r.PathValue("id")))
}

func (a *api) endWorld(w http.ResponseWriter, r *http.Request) {
	if !a.worldAdmin(w, r) {
		return
	}

	a.worldDone(w, r, a.worlds.End(r.Context()))
}

func (a *api) setWorldRules(w http.ResponseWriter, r *http.Request) {
	var req worldRulesJSON
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if !a.worldAdmin(w, r) {
		return
	}

	a.worldDone(w, r, a.worlds.SetRules(r.Context(), model.WorldRules(req)))
}

func toWorldJSON(w model.World) worldJSON {
	return worldJSON{
		ID:        w.ID,
		Seed:      strconv.FormatUint(w.Seed, 10),
		Grid:      w.Grid,
		Status:    w.Status,
		CreatedAt: w.CreatedAt,
		OpenedAt:  worldTime(w.OpenedAt),
		EndedAt:   worldTime(w.EndedAt),
	}
}

func toWorldRulesJSON(r model.WorldRules) worldRulesJSON {
	return worldRulesJSON(r)
}

// toStateJSON names the ruler from people; without them there, ruler is
// null.
func toStateJSON(s model.State, people map[string]model.Author) stateJSON {
	colors := s.Flag.Colors
	if colors == nil {
		colors = []int{}
	}

	return stateJSON{
		ID:             s.ID,
		Name:           s.Name,
		Adjective:      s.Adjective,
		Motto:          s.Motto,
		Color:          s.Color,
		Flag:           flagJSON{Pattern: s.Flag.Pattern, Colors: colors, Emblem: s.Flag.Emblem, EmblemColor: s.Flag.EmblemColor, EmblemAt: s.Flag.EmblemAt},
		Government:     s.Government,
		Ruler:          shownPerson(people, s.RulerID),
		Capital:        s.Capital,
		Treasury:       s.Treasury,
		FoundedAt:      s.FoundedAt,
		ProtectedUntil: s.ProtectedUntil,
		ClubID:         s.ClubID,
		Lands:          s.Lands,
		Population:     s.Population,
		Citizens:       s.Citizens,
		Income:         s.Income,
	}
}

func toColoniesJSON(colonies []model.Colony) []colonyJSON {
	out := make([]colonyJSON, 0, len(colonies))
	for _, c := range colonies {
		out = append(out, colonyJSON(c))
	}

	return out
}

func toStandingJSON(s model.Standing) standingJSON {
	return standingJSON(s)
}

// shownPerson is the person of id among people, nil without them.
func shownPerson(people map[string]model.Author, id string) *authorJSON {
	p, ok := people[id]
	if !ok {
		return nil
	}

	a := toAuthorJSON(p)

	return &a
}

// worldTime is nil for the zero time.
func worldTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}
