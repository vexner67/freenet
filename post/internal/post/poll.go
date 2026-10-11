package post

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxPollLineLen = 128
	// MaxPollOptions is openvk's (polls.max-opts).
	MaxPollOptions = 10
	// maxPollDays is the longest a poll is open when it ends at all, as
	// the list of openvk's "Голосование истекает через".
	maxPollDays = 31
)

var (
	ErrPollNotFound      = errors.New("poll not found")
	ErrPollTitleEmpty    = errors.New("poll title is empty")
	ErrPollTitleTooLong  = errors.New("poll title is too long")
	ErrPollOptionEmpty   = errors.New("poll option is empty")
	ErrPollOptionTooLong = errors.New("poll option is too long")
	ErrPollNoOptions     = errors.New("a poll has at least one option")
	ErrPollTooManyOpts   = errors.New("a poll has at most 10 options")
	ErrPollDays          = errors.New("a poll is open for 1 to 31 days, or for good")
	ErrPollChoice        = errors.New("invalid choice of poll options")
	ErrPollEnded         = errors.New("the poll has ended")
	ErrPollVoted         = errors.New("you have voted in the poll already")
	ErrPollNotVoted      = errors.New("you have not voted in the poll")
	// ErrPollLocked: the poll does not let a vote be taken back ("Запретить
	// отменять свой голос" of openvk).
	ErrPollLocked    = errors.New("the poll does not let a vote be taken back")
	ErrPollAnonymous = errors.New("the poll is anonymous")
)

// PollLine is a line of a poll, its title or an option: one line, trimmed.
type PollLine string

func newPollLine(s string, errEmpty, errTooLong error) (PollLine, error) {
	s = strings.Join(strings.Fields(s), " ")

	if s == "" {
		return "", errEmpty
	}

	if utf8.RuneCountInString(s) > maxPollLineLen {
		return "", errTooLong
	}

	return PollLine(s), nil
}

func NewPollTitle(s string) (PollLine, error) {
	return newPollLine(s, ErrPollTitleEmpty, ErrPollTitleTooLong)
}

func (l PollLine) String() string {
	return string(l)
}

// PollOptions are what a poll lets choose from, in the order shown.
type PollOptions []PollLine

func NewPollOptions(raw []string) (PollOptions, error) {
	if len(raw) == 0 {
		return nil, ErrPollNoOptions
	}

	if len(raw) > MaxPollOptions {
		return nil, ErrPollTooManyOpts
	}

	options := make(PollOptions, 0, len(raw))
	for _, s := range raw {
		line, err := newPollLine(s, ErrPollOptionEmpty, ErrPollOptionTooLong)
		if err != nil {
			return nil, err
		}

		options = append(options, line)
	}

	return options, nil
}

// PollDraft is a poll as its author asks it. Days is for how long it is
// open, 0 for good.
type PollDraft struct {
	Title   PollLine
	Options PollOptions
	// Anonymous: nobody is shown who chose what.
	Anonymous bool
	// Multiple: several options may be chosen at once.
	Multiple bool
	// Locked: a vote cannot be taken back.
	Locked bool
	Days   int
}

// PollTally is how a poll stands: the votes of each option, in their
// order, and how many people voted — with several choices fewer than the
// votes. Storage counts them with the votes.
type PollTally struct {
	Votes  []int
	Voters int
}

// Poll is asked by a post, one a post at most, and is known by the ID of
// that post. Nothing of it changes once asked but its tally.
type Poll struct {
	postID    uuid.UUID
	title     PollLine
	options   PollOptions
	anonymous bool
	multiple  bool
	locked    bool
	// endsAt is zero for a poll open for good.
	endsAt time.Time
	tally  PollTally
}

// NewPoll is the poll the post postID asks from now on.
func NewPoll(postID uuid.UUID, d PollDraft, now time.Time) (*Poll, error) {
	if d.Title == "" {
		return nil, ErrPollTitleEmpty
	}

	if len(d.Options) == 0 {
		return nil, ErrPollNoOptions
	}

	if len(d.Options) > MaxPollOptions {
		return nil, ErrPollTooManyOpts
	}

	if d.Days < 0 || d.Days > maxPollDays {
		return nil, ErrPollDays
	}

	p := &Poll{
		postID:    postID,
		title:     d.Title,
		options:   d.Options,
		anonymous: d.Anonymous,
		multiple:  d.Multiple,
		locked:    d.Locked,
		tally:     PollTally{Votes: make([]int, len(d.Options))},
	}

	if d.Days > 0 {
		// PostgreSQL keeps microseconds; trim so a poll reads back equal.
		p.endsAt = now.UTC().Truncate(time.Microsecond).Add(time.Duration(d.Days) * 24 * time.Hour)
	}

	return p, nil
}

// StoredPoll is a poll as storage keeps it.
type StoredPoll struct {
	PostID    uuid.UUID
	Title     PollLine
	Options   PollOptions
	Anonymous bool
	Multiple  bool
	Locked    bool
	EndsAt    time.Time
	Tally     PollTally
}

// RestorePoll rebuilds a poll from storage without validation.
func RestorePoll(s StoredPoll) *Poll {
	return &Poll{
		postID:    s.PostID,
		title:     s.Title,
		options:   s.Options,
		anonymous: s.Anonymous,
		multiple:  s.Multiple,
		locked:    s.Locked,
		endsAt:    s.EndsAt,
		tally:     s.Tally,
	}
}

// Stored is what storage keeps of p.
func (p *Poll) Stored() StoredPoll {
	return StoredPoll{
		PostID:    p.postID,
		Title:     p.title,
		Options:   p.options,
		Anonymous: p.anonymous,
		Multiple:  p.multiple,
		Locked:    p.locked,
		EndsAt:    p.endsAt,
		Tally:     p.tally,
	}
}

func (p *Poll) PostID() uuid.UUID {
	return p.postID
}

func (p *Poll) Title() PollLine {
	return p.title
}

func (p *Poll) Options() PollOptions {
	return p.options
}

func (p *Poll) Anonymous() bool {
	return p.anonymous
}

func (p *Poll) Multiple() bool {
	return p.multiple
}

func (p *Poll) Locked() bool {
	return p.locked
}

// EndsAt is zero for a poll open for good.
func (p *Poll) EndsAt() time.Time {
	return p.endsAt
}

func (p *Poll) Tally() PollTally {
	return p.tally
}

// Ended tells whether the poll takes no votes anymore.
func (p *Poll) Ended(now time.Time) bool {
	return !p.endsAt.IsZero() && !now.Before(p.endsAt)
}

// PollChoice are the options someone chooses, by their places in the
// poll, in that order and each once.
type PollChoice []int

// Choice checks what a voter chooses in p: one option, or several where
// the poll lets, each one of its own.
func (p *Poll) Choice(places []int) (PollChoice, error) {
	choice := slices.Clone(places)
	slices.Sort(choice)
	choice = slices.Compact(choice)

	if len(choice) == 0 || (len(choice) > 1 && !p.multiple) {
		return nil, ErrPollChoice
	}

	if choice[0] < 0 || choice[len(choice)-1] >= len(p.options) {
		return nil, ErrPollChoice
	}

	return PollChoice(choice), nil
}

// CheckVote tells whether someone may vote at now: voted tells that they
// have already, and a vote is changed only by taking it back first, as in
// openvk.
func (p *Poll) CheckVote(voted bool, now time.Time) error {
	if p.Ended(now) {
		return ErrPollEnded
	}

	if voted {
		return ErrPollVoted
	}

	return nil
}

// CheckRetract tells whether someone who voted may take the vote back at
// now: not in a locked poll, and not once it has ended — the vote could
// not be cast again.
func (p *Poll) CheckRetract(voted bool, now time.Time) error {
	if !voted {
		return ErrPollNotVoted
	}

	if p.locked {
		return ErrPollLocked
	}

	if p.Ended(now) {
		return ErrPollEnded
	}

	return nil
}

// CheckVoters tells whether the poll shows who chose what, and that place
// is an option of it.
func (p *Poll) CheckVoters(place int) error {
	if p.anonymous {
		return ErrPollAnonymous
	}

	if place < 0 || place >= len(p.options) {
		return ErrPollChoice
	}

	return nil
}

// Counted is p after someone's choice was added (by +1) or taken back
// (by -1), for storage that keeps polls whole.
func (p *Poll) Counted(choice PollChoice, by int) *Poll {
	counted := *p
	counted.tally = PollTally{Votes: slices.Clone(p.tally.Votes), Voters: p.tally.Voters + by}

	for _, place := range choice {
		counted.tally.Votes[place] += by
	}

	return &counted
}
