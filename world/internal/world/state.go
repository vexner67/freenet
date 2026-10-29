package world

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrStateNotFound = errors.New("state not found")
	ErrStateName     = errors.New("invalid name of the state")
	ErrNameTaken     = errors.New("the name of the state is taken")
	ErrAdjective     = errors.New("invalid adjective of the state")
	ErrMotto         = errors.New("invalid motto of the state")
	ErrColor         = errors.New("invalid color of the state")
	ErrGovernment    = errors.New("invalid form of government")
	ErrProvinceTaken = errors.New("the province is not free")
	ErrTooClose      = errors.New("the province is too close to another capital")
	ErrNotNeighbour  = errors.New("the province does not border the state")
	ErrNoColonists   = errors.New("all colonists are busy")
	ErrTreasury      = errors.New("not enough votes in the treasury")
	ErrDonation      = errors.New("invalid donation")
	ErrCitizen       = errors.New("already a citizen of a state")
	ErrNotCitizen    = errors.New("not a citizen of the state")
	ErrApplied       = errors.New("already applied to the state")
	ErrNoApplication = errors.New("application not found")
	ErrNotAllowed    = errors.New("the office does not allow it")
	ErrRole          = errors.New("invalid office")
	ErrRulerLeaves   = errors.New("the ruler cannot leave the state")
	ErrSelf          = errors.New("not to oneself")
	ErrNotProvince   = errors.New("not a province of land")
	ErrFreeCity      = errors.New("a free city belongs to nobody")
)

const (
	maxStateNameLen = 40
	maxAdjectiveLen = 40
	maxMottoLen     = 80
)

// StateName is the name of a state: one line of 1–40 characters, unique
// in its world whatever the case.
type StateName string

func NewStateName(s string) (StateName, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > maxStateNameLen {
		return "", ErrStateName
	}

	return StateName(s), nil
}

func (n StateName) String() string { return string(n) }

// same says whether two names are one, whatever the case.
func (n StateName) same(o StateName) bool {
	return strings.EqualFold(string(n), string(o))
}

// Adjective is what the news call the things of a state: «бореальская
// армия». It may be empty.
type Adjective string

func NewAdjective(s string) (Adjective, error) {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxAdjectiveLen {
		return "", ErrAdjective
	}

	return Adjective(s), nil
}

func (a Adjective) String() string { return string(a) }

// Motto is one line of up to 80 characters; it may be empty.
type Motto string

func NewMotto(s string) (Motto, error) {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxMottoLen {
		return "", ErrMotto
	}

	return Motto(s), nil
}

func (m Motto) String() string { return string(m) }

// Color is the number of the color of a state on the map.
type Color int

func NewColor(c int) (Color, error) {
	if !inPalette(c) {
		return 0, ErrColor
	}

	return Color(c), nil
}

// Government is the form of government. For now it only names the state;
// its rules come with the laws.
type Government string

const (
	GovernmentMonarchy     Government = "monarchy"
	GovernmentRepublic     Government = "republic"
	GovernmentDictatorship Government = "dictatorship"
	GovernmentFederation   Government = "federation"
	// GovernmentIslamism is a state ruled by sharia; its rules (the
	// council of ministers, zakat, no interest) come with the laws.
	GovernmentIslamism Government = "islamism"
)

func NewGovernment(s string) (Government, error) {
	switch g := Government(s); g {
	case GovernmentMonarchy, GovernmentRepublic, GovernmentDictatorship, GovernmentFederation, GovernmentIslamism:
		return g, nil
	}

	return "", ErrGovernment
}

// State is a state of a world. Treasury and the budget are in hundredths
// of a vote.
type State struct {
	ID             uuid.UUID
	Name           StateName
	Adjective      Adjective
	Motto          Motto
	Color          Color
	Flag           Flag
	Government     Government
	RulerID        uuid.UUID
	Capital        int
	Treasury       int64
	FoundedAt      time.Time
	ProtectedUntil time.Time
	// ClubID is its group, uuid.Nil until the club service made it.
	ClubID uuid.UUID
	// Budget is what came in and went out in the last day, hour by hour,
	// the oldest first.
	Budget []BudgetHour
}

// BudgetHour is the money of a state in an hour, in hundredths.
type BudgetHour struct {
	Hour         time.Time
	Income       int64
	Donations    int64
	Founding     int64
	Colonization int64
}

// budgetHours is how many hours the budget keeps.
const budgetHours = 24

// book adds to the hour of at what change does, keeping the last day.
func (s *State) book(at time.Time, change func(*BudgetHour)) {
	hour := at.UTC().Truncate(time.Hour)
	if n := len(s.Budget); n == 0 || !s.Budget[n-1].Hour.Equal(hour) {
		s.Budget = append(s.Budget, BudgetHour{Hour: hour})
	}
	change(&s.Budget[len(s.Budget)-1])

	if over := len(s.Budget) - budgetHours; over > 0 {
		s.Budget = append([]BudgetHour(nil), s.Budget[over:]...)
	}
}

// Role is the office of a citizen.
type Role string

const (
	RoleRuler            Role = "ruler"
	RoleMinisterEconomy  Role = "minister_economy"
	RoleMinisterForeign  Role = "minister_foreign"
	RoleMinisterDefense  Role = "minister_defense"
	RoleMinisterInterior Role = "minister_interior"
	RoleGeneral          Role = "general"
	RoleCitizen          Role = "citizen"
)

// NewRole is an office a ruler may give: any but the ruler's own.
func NewRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleMinisterEconomy, RoleMinisterForeign, RoleMinisterDefense, RoleMinisterInterior, RoleGeneral, RoleCitizen:
		return r, nil
	}

	return "", ErrRole
}

func (r Role) isMinister() bool {
	return strings.HasPrefix(string(r), "minister_")
}

// mayAnswer: whether the office takes in citizens and sends them away.
func (r Role) mayAnswer() bool {
	return r == RoleRuler || r == RoleMinisterInterior
}

// mayColonize: whether the office spends the treasury on colonies.
func (r Role) mayColonize() bool {
	return r == RoleRuler || r == RoleMinisterEconomy
}

// Citizen is a person of a state; a person is a citizen of one state at
// most.
type Citizen struct {
	UserID   uuid.UUID
	StateID  uuid.UUID
	Role     Role
	JoinedAt time.Time
}

// Application asks a state to take a person in; a person has one at most.
type Application struct {
	UserID  uuid.UUID
	StateID uuid.UUID
	At      time.Time
}

// Colony is a province a state is settling until Until.
type Colony struct {
	Province int
	StateID  uuid.UUID
	Until    time.Time
}
