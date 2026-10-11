package post_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func pollDraft(t *testing.T, options ...string) post.PollDraft {
	t.Helper()

	title, err := post.NewPollTitle("Кошки или собаки?")
	if err != nil {
		t.Fatal(err)
	}

	opts, err := post.NewPollOptions(options)
	if err != nil {
		t.Fatal(err)
	}

	return post.PollDraft{Title: title, Options: opts}
}

func TestNewPollTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{name: "trimmed to one line", in: "  Кошки\n или  собаки? ", want: "Кошки или собаки?"},
		{name: "empty", in: " \n ", err: post.ErrPollTitleEmpty},
		{name: "at the limit", in: strings.Repeat("я", 128), want: strings.Repeat("я", 128)},
		{name: "too long", in: strings.Repeat("я", 129), err: post.ErrPollTitleTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewPollTitle(tt.in)
			if !errors.Is(err, tt.err) || got.String() != tt.want {
				t.Fatalf("got %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewPollOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		err  error
	}{
		{name: "one", in: []string{"да"}},
		{name: "ten", in: slices.Repeat([]string{"да"}, 10)},
		{name: "none", err: post.ErrPollNoOptions},
		{name: "eleven", in: slices.Repeat([]string{"да"}, 11), err: post.ErrPollTooManyOpts},
		{name: "an empty one", in: []string{"да", "  "}, err: post.ErrPollOptionEmpty},
		{name: "a long one", in: []string{strings.Repeat("я", 129)}, err: post.ErrPollOptionTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := post.NewPollOptions(tt.in); !errors.Is(err, tt.err) {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
		})
	}
}

func TestNewPoll(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 11, 12, 0, 0, 123456789, time.UTC)
	postID := uuid.New()

	p, err := post.NewPoll(postID, pollDraft(t, "кошки", "собаки"), now)
	if err != nil {
		t.Fatal(err)
	}

	if p.PostID() != postID || !p.EndsAt().IsZero() || p.Ended(now.AddDate(10, 0, 0)) {
		t.Fatalf("a poll for good: %+v", p.Stored())
	}

	if tally := p.Tally(); len(tally.Votes) != 2 || tally.Voters != 0 {
		t.Fatalf("tally = %+v", tally)
	}

	d := pollDraft(t, "кошки")
	d.Days = 3

	p, err = post.NewPoll(postID, d, now)
	if err != nil {
		t.Fatal(err)
	}

	end := now.Truncate(time.Microsecond).Add(72 * time.Hour)
	if !p.EndsAt().Equal(end) || p.Ended(end.Add(-time.Second)) || !p.Ended(end) {
		t.Fatalf("ends at %v, want %v", p.EndsAt(), end)
	}

	for _, days := range []int{-1, 32} {
		d.Days = days
		if _, err = post.NewPoll(postID, d, now); !errors.Is(err, post.ErrPollDays) {
			t.Fatalf("%d days: got %v, want ErrPollDays", days, err)
		}
	}

	if _, err = post.NewPoll(postID, post.PollDraft{Options: d.Options}, now); !errors.Is(err, post.ErrPollTitleEmpty) {
		t.Fatalf("no title: got %v", err)
	}

	if _, err = post.NewPoll(postID, post.PollDraft{Title: d.Title}, now); !errors.Is(err, post.ErrPollNoOptions) {
		t.Fatalf("no options: got %v", err)
	}
}

func TestPoll_Choice(t *testing.T) {
	t.Parallel()

	now := time.Now()
	single, _ := post.NewPoll(uuid.New(), pollDraft(t, "а", "б", "в"), now)

	d := pollDraft(t, "а", "б", "в")
	d.Multiple = true
	multiple, _ := post.NewPoll(uuid.New(), d, now)

	tests := []struct {
		name string
		poll *post.Poll
		in   []int
		want post.PollChoice
		err  error
	}{
		{name: "one", poll: single, in: []int{2}, want: post.PollChoice{2}},
		{name: "one named twice", poll: single, in: []int{1, 1}, want: post.PollChoice{1}},
		{name: "none", poll: single, err: post.ErrPollChoice},
		{name: "two of a single choice", poll: single, in: []int{0, 1}, err: post.ErrPollChoice},
		{name: "past the options", poll: single, in: []int{3}, err: post.ErrPollChoice},
		{name: "before the options", poll: multiple, in: []int{-1, 0}, err: post.ErrPollChoice},
		{name: "several, in order", poll: multiple, in: []int{2, 0, 2}, want: post.PollChoice{0, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.poll.Choice(tt.in)
			if !errors.Is(err, tt.err) || !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, %v; want %v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestPoll_Rules(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	d := pollDraft(t, "а", "б")
	d.Days = 1
	open, _ := post.NewPoll(uuid.New(), d, now)
	later := now.Add(25 * time.Hour)

	if err := open.CheckVote(false, now); err != nil {
		t.Fatalf("vote: %v", err)
	}
	if err := open.CheckVote(true, now); !errors.Is(err, post.ErrPollVoted) {
		t.Fatalf("vote twice: got %v", err)
	}
	if err := open.CheckVote(false, later); !errors.Is(err, post.ErrPollEnded) {
		t.Fatalf("vote late: got %v", err)
	}

	if err := open.CheckRetract(true, now); err != nil {
		t.Fatalf("retract: %v", err)
	}
	if err := open.CheckRetract(false, now); !errors.Is(err, post.ErrPollNotVoted) {
		t.Fatalf("retract nothing: got %v", err)
	}
	if err := open.CheckRetract(true, later); !errors.Is(err, post.ErrPollEnded) {
		t.Fatalf("retract late: got %v", err)
	}

	d.Locked, d.Anonymous = true, true
	locked, _ := post.NewPoll(uuid.New(), d, now)

	if err := locked.CheckRetract(true, now); !errors.Is(err, post.ErrPollLocked) {
		t.Fatalf("retract in a locked poll: got %v", err)
	}

	if err := open.CheckVoters(1); err != nil {
		t.Fatalf("voters: %v", err)
	}
	if err := open.CheckVoters(2); !errors.Is(err, post.ErrPollChoice) {
		t.Fatalf("voters of no option: got %v", err)
	}
	if err := locked.CheckVoters(0); !errors.Is(err, post.ErrPollAnonymous) {
		t.Fatalf("voters of an anonymous poll: got %v", err)
	}
}

func TestPoll_Counted(t *testing.T) {
	t.Parallel()

	p, _ := post.NewPoll(uuid.New(), pollDraft(t, "а", "б", "в"), time.Now())

	counted := p.Counted(post.PollChoice{0, 2}, 1).Counted(post.PollChoice{2}, 1)
	if tally := counted.Tally(); !slices.Equal(tally.Votes, []int{1, 0, 2}) || tally.Voters != 2 {
		t.Fatalf("tally = %+v", tally)
	}

	back := counted.Counted(post.PollChoice{0, 2}, -1)
	if tally := back.Tally(); !slices.Equal(tally.Votes, []int{0, 0, 1}) || tally.Voters != 1 {
		t.Fatalf("tally after a vote taken back = %+v", tally)
	}

	if tally := p.Tally(); tally.Voters != 0 || tally.Votes[0] != 0 {
		t.Fatalf("the poll itself changed: %+v", tally)
	}
}

func TestPost_EditedKeepsPoll(t *testing.T) {
	t.Parallel()

	now := time.Now()
	author := uuid.New()

	p, err := post.New(author, author, post.NewPolled("вопрос", nil), now)
	if err != nil {
		t.Fatal(err)
	}

	// The poll is content enough: the words may go.
	edited, err := p.Edited(author, "", nil, now)
	if err != nil {
		t.Fatalf("Edited: %v", err)
	}

	if !edited.Content().Polled() || edited.Content().Text() != "" {
		t.Fatalf("content = %+v", edited.Content())
	}

	if _, err = post.NewAvatarUpdate(author, author, post.NewPolled("", post.Photos{uuid.New()}), now); !errors.Is(err, post.ErrAvatarUpdate) {
		t.Fatalf("a new photo with a poll: got %v", err)
	}
}
