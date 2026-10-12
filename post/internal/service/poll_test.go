package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// pollFixture is the services of posts and polls over one store, with a
// clock the test moves on from the time the posts are written at.
type pollFixture struct {
	posts *service.PostService
	polls *service.PollService
	purge *service.PurgeService
	now   time.Time
}

func newPollFixture(moderators fakeModerators) *pollFixture {
	store := inmem.NewStore()
	f := &pollFixture{now: time.Now()}

	f.posts = service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{},
		Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: moderators, Switches: store.Posts(), Polls: store.Polls(),
	})
	f.polls = service.NewPollService(service.PollDeps{
		Polls: store.Polls(), Posts: store.Posts(), Moderators: moderators, Tx: fakeTx{},
		Now: func() time.Time { return f.now },
	})
	f.purge = service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Votes: store.Polls(), Sources: store.Sources(), Comments: store.Comments(),
		Suggestions: inmem.NewSuggestionRepository(), Topics: inmem.NewTopicRepository(), Albums: inmem.NewAlbumRepository(), Notes: inmem.NewNoteRepository(),
		Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{},
	})

	return f
}

func draft(t *testing.T, options ...string) post.PollDraft {
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

// ask writes a post by author on their wall that asks the poll d.
func (f *pollFixture) ask(t *testing.T, author uuid.UUID, d post.PollDraft) *post.Post {
	t.Helper()

	p, err := f.posts.Create(context.Background(), service.CreateInput{
		WallID: author, AuthorID: author, Content: post.NewPolled("", nil), Poll: &d,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	return p
}

// view is the poll of a post as viewer sees it.
func (f *pollFixture) view(t *testing.T, viewer, postID uuid.UUID) service.PollView {
	t.Helper()

	views, err := f.polls.Of(context.Background(), viewer, []uuid.UUID{postID})
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("polls = %d, want 1", len(views))
	}

	return views[0]
}

func TestPostService_CreateWithPoll(t *testing.T) {
	t.Parallel()

	f := newPollFixture(fakeModerators{})
	ctx := context.Background()
	ann := uuid.New()

	p := f.ask(t, ann, draft(t, "кошки", "собаки"))
	if !p.Content().Polled() {
		t.Fatal("the post does not say it asks a poll")
	}

	v := f.view(t, uuid.Nil, p.ID())
	if v.Poll.Title() != "Кошки или собаки?" || len(v.Poll.Options()) != 2 || v.Chosen != nil || v.Ended {
		t.Fatalf("poll = %+v", v.Poll.Stored())
	}

	// A post asks a poll exactly when its content says so.
	d := draft(t, "да")
	if _, err := f.posts.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: textContent(t, "слова"), Poll: &d}); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("a poll without polled content: got %v", err)
	}
	if _, err := f.posts.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: post.NewPolled("слова", nil)}); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("polled content without a poll: got %v", err)
	}

	d.Days = 40
	if _, err := f.posts.Create(ctx, service.CreateInput{WallID: ann, AuthorID: ann, Content: post.NewPolled("", nil), Poll: &d}); !errors.Is(err, post.ErrPollDays) {
		t.Fatalf("a poll of 40 days: got %v", err)
	}

	// A post without a poll has none to give.
	plain := write(t, f.posts, ann, "просто запись")[0]
	if views, err := f.polls.Of(ctx, ann, []uuid.UUID{plain.ID(), uuid.New()}); err != nil || len(views) != 0 {
		t.Fatalf("polls of a plain post = %v, %v", views, err)
	}

	// The poll goes with its post.
	if err := f.posts.Delete(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}
	if views, err := f.polls.Of(ctx, ann, []uuid.UUID{p.ID()}); err != nil || len(views) != 0 {
		t.Fatalf("polls of a deleted post = %v, %v", views, err)
	}

	if _, err := f.polls.Of(ctx, ann, make([]uuid.UUID, service.MaxPollBatch+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("too many posts: got %v", err)
	}
}

func TestPollService_Vote(t *testing.T) {
	t.Parallel()

	f := newPollFixture(fakeModerators{})
	ctx := context.Background()
	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()

	p := f.ask(t, ann, draft(t, "кошки", "собаки", "рыбки"))

	v, err := f.polls.Vote(ctx, bob, p.ID(), []int{1})
	if err != nil {
		t.Fatalf("Vote: %v", err)
	}
	if tally := v.Poll.Tally(); !slices.Equal(tally.Votes, []int{0, 1, 0}) || tally.Voters != 1 || !slices.Equal(v.Chosen, post.PollChoice{1}) {
		t.Fatalf("after the vote: %+v chosen %v", tally, v.Chosen)
	}

	if _, err = f.polls.Vote(ctx, bob, p.ID(), []int{0}); !errors.Is(err, post.ErrPollVoted) {
		t.Fatalf("a second vote: got %v, want ErrPollVoted", err)
	}
	if _, err = f.polls.Vote(ctx, eve, p.ID(), []int{0, 1}); !errors.Is(err, post.ErrPollChoice) {
		t.Fatalf("two options of a single choice: got %v", err)
	}
	if _, err = f.polls.Vote(ctx, eve, p.ID(), []int{7}); !errors.Is(err, post.ErrPollChoice) {
		t.Fatalf("no such option: got %v", err)
	}

	if _, err = f.polls.Vote(ctx, eve, p.ID(), []int{1}); err != nil {
		t.Fatal(err)
	}

	// Everyone sees the tally, each their own choice.
	if v = f.view(t, bob, p.ID()); !slices.Equal(v.Chosen, post.PollChoice{1}) || v.Poll.Tally().Voters != 2 {
		t.Fatalf("bob sees %+v, chosen %v", v.Poll.Tally(), v.Chosen)
	}
	if v = f.view(t, ann, p.ID()); v.Chosen != nil || v.Poll.Tally().Votes[1] != 2 {
		t.Fatalf("ann sees %+v, chosen %v", v.Poll.Tally(), v.Chosen)
	}

	// A vote taken back can be cast again, otherwise.
	if v, err = f.polls.Retract(ctx, bob, p.ID()); err != nil || v.Chosen != nil || v.Poll.Tally().Voters != 1 {
		t.Fatalf("Retract: %+v, %v", v, err)
	}
	if _, err = f.polls.Retract(ctx, bob, p.ID()); !errors.Is(err, post.ErrPollNotVoted) {
		t.Fatalf("retract twice: got %v", err)
	}
	if _, err = f.polls.Vote(ctx, bob, p.ID(), []int{2}); err != nil {
		t.Fatalf("vote again: %v", err)
	}
	if v = f.view(t, bob, p.ID()); !slices.Equal(v.Poll.Tally().Votes, []int{0, 1, 1}) {
		t.Fatalf("tally = %v", v.Poll.Tally().Votes)
	}

	// No poll to vote in.
	plain := write(t, f.posts, ann, "просто запись")[0]
	if _, err = f.polls.Vote(ctx, bob, plain.ID(), []int{0}); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("a post without a poll: got %v", err)
	}
	if _, err = f.polls.Vote(ctx, bob, uuid.New(), []int{0}); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("no post: got %v", err)
	}
}

func TestPollService_Multiple(t *testing.T) {
	t.Parallel()

	f := newPollFixture(fakeModerators{})
	ctx := context.Background()
	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()

	d := draft(t, "кошки", "собаки", "рыбки")
	d.Multiple = true
	p := f.ask(t, ann, d)

	if _, err := f.polls.Vote(ctx, bob, p.ID(), []int{2, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.polls.Vote(ctx, eve, p.ID(), []int{0}); err != nil {
		t.Fatal(err)
	}

	// Two voters, three votes.
	v := f.view(t, bob, p.ID())
	if tally := v.Poll.Tally(); !slices.Equal(tally.Votes, []int{2, 0, 1}) || tally.Voters != 2 || !slices.Equal(v.Chosen, post.PollChoice{0, 2}) {
		t.Fatalf("tally = %+v, chosen %v", tally, v.Chosen)
	}

	page, err := f.polls.Voters(ctx, p.ID(), 0, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Likers) != 1 || page.Next == nil {
		t.Fatalf("the first page of voters = %+v", page)
	}

	next, err := f.polls.Voters(ctx, p.ID(), 0, page.Next, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Likers) != 1 || next.Next != nil || next.Likers[0].UserID == page.Likers[0].UserID {
		t.Fatalf("the second page of voters = %+v", next)
	}

	if page, err = f.polls.Voters(ctx, p.ID(), 1, nil, 0); err != nil || page.Total != 0 || len(page.Likers) != 0 {
		t.Fatalf("voters of an option nobody chose = %+v, %v", page, err)
	}
	if _, err = f.polls.Voters(ctx, p.ID(), 3, nil, 0); !errors.Is(err, post.ErrPollChoice) {
		t.Fatalf("voters of no option: got %v", err)
	}
	if _, err = f.polls.Voters(ctx, uuid.New(), 0, nil, 0); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("voters of no poll: got %v", err)
	}

	// The votes of an erased page are uncounted.
	if err = f.purge.PurgeUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if tally := f.view(t, eve, p.ID()).Poll.Tally(); !slices.Equal(tally.Votes, []int{1, 0, 0}) || tally.Voters != 1 {
		t.Fatalf("tally after the purge = %+v", tally)
	}
}

func TestPollService_Limits(t *testing.T) {
	t.Parallel()

	f := newPollFixture(fakeModerators{})
	ctx := context.Background()
	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()

	d := draft(t, "кошки", "собаки")
	d.Locked, d.Anonymous, d.Days = true, true, 2
	p := f.ask(t, ann, d)

	if _, err := f.polls.Vote(ctx, bob, p.ID(), []int{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.polls.Retract(ctx, bob, p.ID()); !errors.Is(err, post.ErrPollLocked) {
		t.Fatalf("retract in a locked poll: got %v", err)
	}
	if _, err := f.polls.Voters(ctx, p.ID(), 0, nil, 0); !errors.Is(err, post.ErrPollAnonymous) {
		t.Fatalf("voters of an anonymous poll: got %v", err)
	}

	if f.view(t, eve, p.ID()).Ended {
		t.Fatal("the poll has ended early")
	}

	f.now = f.now.Add(49 * time.Hour)

	if _, err := f.polls.Vote(ctx, eve, p.ID(), []int{0}); !errors.Is(err, post.ErrPollEnded) {
		t.Fatalf("a late vote: got %v", err)
	}
	if v := f.view(t, bob, p.ID()); !v.Ended || v.Poll.Tally().Voters != 1 || !slices.Equal(v.Chosen, post.PollChoice{0}) {
		t.Fatalf("the ended poll = %+v", v)
	}
}

func TestPollService_Archived(t *testing.T) {
	t.Parallel()

	club, admin := uuid.New(), uuid.New()
	f := newPollFixture(fakeModerators{{club, admin}: true})
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()

	p := f.ask(t, ann, draft(t, "кошки", "собаки"))
	if err := f.posts.Archive(ctx, ann, p.ID()); err != nil {
		t.Fatal(err)
	}

	// Only who keeps the archive is shown the poll, and votes in it.
	if views, err := f.polls.Of(ctx, bob, []uuid.UUID{p.ID()}); err != nil || len(views) != 0 {
		t.Fatalf("a stranger sees %v, %v", views, err)
	}
	if _, err := f.polls.Vote(ctx, bob, p.ID(), []int{0}); !errors.Is(err, post.ErrPollNotFound) {
		t.Fatalf("a stranger votes: got %v", err)
	}
	if _, err := f.polls.Vote(ctx, ann, p.ID(), []int{0}); err != nil {
		t.Fatalf("the owner votes: %v", err)
	}

	d := draft(t, "да", "нет")
	onClub, err := f.posts.Create(ctx, service.CreateInput{WallID: club, AuthorID: ann, Content: post.NewPolled("", nil), Poll: &d, ClubWall: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.posts.Archive(ctx, admin, onClub.ID()); err != nil {
		t.Fatal(err)
	}

	if views, err := f.polls.Of(ctx, admin, []uuid.UUID{onClub.ID(), p.ID()}); err != nil || len(views) != 1 || views[0].Poll.PostID() != onClub.ID() {
		t.Fatalf("the admin sees %v, %v", views, err)
	}
	if views, err := f.polls.Of(ctx, ann, []uuid.UUID{onClub.ID()}); err != nil || len(views) != 0 {
		t.Fatalf("the author, no admin, sees %v, %v", views, err)
	}
}
