package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/service"
)

const meduza = fedi.URL("https://t.me/meduzalive")

// channelPost is post number of meduzalive with text, published at.
func channelPost(number, text string, at time.Time) fedi.NoteDoc {
	id := meduza.String() + "/" + number

	return fedi.NoteDoc{ID: id, AttributedTo: meduza.String(), URL: id, Text: text, Public: true, Published: at}
}

func (f *fixture) meduzaPage(posts ...fedi.NoteDoc) {
	f.channels.pages["meduzalive"] = fedi.ChannelDoc{
		Title: "Медуза — LIVE", Username: "meduzalive", Photo: "https://cdn4.telesco.pe/file/m.jpg", Posts: posts,
	}
}

func TestSubscribeChannel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	f.meduzaPage(channelPost("1", "first", f.now.Add(-time.Hour)), channelPost("2", "second", f.now.Add(-time.Minute)))

	a, err := f.svc.Subscribe(ctx, f.user, "https://t.me/s/MeduzaLive")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if a.Remote.ID() != meduza || !a.Accepted || a.Remote.Handle() != "@meduzalive" || a.Remote.Profile().Name != "Медуза — LIVE" {
		t.Fatalf("account = %+v, want meduzalive followed at once", a)
	}

	if len(f.sender.sent) != 0 {
		t.Fatalf("sent %+v: a channel has nobody to send to", f.sender.sent)
	}

	if texts := f.newsTexts(t, service.NewsQuery{Reader: f.user}); len(texts) != 2 || texts[0] != "second" || texts[1] != "first" {
		t.Fatalf("news = %v, want the posts of the channel at once", texts)
	}

	if err = f.svc.Unsubscribe(ctx, f.user, meduza); err != nil || len(f.sender.sent) != 0 {
		t.Fatalf("Unsubscribe: %v, sent %d", err, len(f.sender.sent))
	}

	if texts := f.newsTexts(t, service.NewsQuery{Reader: f.user}); len(texts) != 0 {
		t.Fatalf("news after Unsubscribe = %v", texts)
	}
}

func TestReadChannels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)
	f.meduzaPage(channelPost("1", "first", f.now.Add(-time.Hour)))

	if _, err := f.svc.Feature(ctx, "@meduzalive"); err != nil {
		t.Fatalf("Feature: %v", err)
	}

	f.now = f.now.Add(5 * time.Minute)
	f.meduzaPage(
		channelPost("1", "first, corrected", f.now.Add(-time.Hour)),
		channelPost("2", "second", f.now.Add(-time.Minute)),
		channelPost("3", "", f.now),                              // nothing to show
		channelPost("4", "too old", f.now.Add(-91*24*time.Hour)), // older than the news keep
	)

	n, err := f.svc.ReadChannels(ctx)
	if err != nil || n != 1 {
		t.Fatalf("ReadChannels = %d, %v; want one new post", n, err)
	}

	posts, err := f.svc.News(ctx, service.NewsQuery{Featured: true, Limit: 20})
	if err != nil || len(posts) != 2 {
		t.Fatalf("News = %d, %v", len(posts), err)
	}

	if posts[0].Post.Text() != "second" || posts[1].Post.Text() != "first, corrected" || !posts[1].Post.Updated().Equal(f.now) {
		t.Fatalf("news = %q, %q edited %v", posts[0].Post.Text(), posts[1].Post.Text(), posts[1].Post.Updated())
	}

	// Read again unchanged: nothing new, nothing edited.
	if n, err = f.svc.ReadChannels(ctx); err != nil || n != 0 {
		t.Fatalf("ReadChannels again = %d, %v", n, err)
	}

	// A channel its page no longer shows keeps what was kept.
	delete(f.channels.pages, "meduzalive")
	if n, err = f.svc.ReadChannels(ctx); err != nil || n != 0 {
		t.Fatalf("ReadChannels of a channel gone = %d, %v", n, err)
	}

	f.channels.fail = errors.New("t.me is down")
	if _, err = f.svc.ReadChannels(ctx); err == nil {
		t.Fatal("ReadChannels with t.me down: no error")
	}
}

func TestLookupChannel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.svc.Lookup(ctx, "t.me/nobody_here"); !errors.Is(err, fedi.ErrAccountNotFound) {
		t.Fatalf("Lookup of no channel: %v, want fedi.ErrAccountNotFound", err)
	}

	f.channels.pages["nobody_here"] = fedi.ChannelDoc{Username: "somebody_else"}
	if _, err := f.svc.Lookup(ctx, "t.me/nobody_here"); !errors.Is(err, fedi.ErrAccountNotFound) {
		t.Fatalf("Lookup of a page of another channel: %v", err)
	}
}
