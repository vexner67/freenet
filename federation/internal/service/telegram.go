package service

import (
	"context"
	"errors"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
)

// channel is the account of a channel of Telegram as its page tells it
// now, kept as such, and the page.
func (s *Service) channel(ctx context.Context, name string) (*fedi.Remote, fedi.ChannelDoc, error) {
	doc, err := s.channels.Channel(ctx, name)
	if err != nil {
		return nil, fedi.ChannelDoc{}, errs.Wrap("s.channels.Channel", err)
	}

	r, err := fedi.NewChannel(name, doc, s.now())
	if err != nil {
		return nil, fedi.ChannelDoc{}, errs.Wrap("fedi.NewChannel", err)
	}

	if err = s.remotes.Save(ctx, r); err != nil {
		return nil, fedi.ChannelDoc{}, errs.Wrap("s.remotes.Save", err)
	}

	return r, doc, nil
}

// followChannel makes the site read a channel: there is nobody to ask,
// so it is followed at once, and its latest posts come at once too.
func (s *Service) followChannel(ctx context.Context, f *fedi.Following) error {
	if err := s.following.Save(ctx, f.WithAccepted(true)); err != nil {
		return errs.Wrap("s.following.Save", err)
	}

	_, err := s.readChannel(ctx, f.Actor())
	return err
}

// ReadChannels reads the pages of the channels of Telegram the site
// follows and keeps their posts, telling how many are new. A channel that
// fails does not keep the others from being read.
func (s *Service) ReadChannels(ctx context.Context) (int, error) {
	actors, err := s.following.Channels(ctx)
	if err != nil {
		return 0, errs.Wrap("s.following.Channels", err)
	}

	var (
		added  int
		failed []error
	)

	for _, actor := range actors {
		if ctx.Err() != nil {
			return added, errs.Wrap("ctx.Err", ctx.Err())
		}

		n, err := s.readChannel(ctx, actor)
		if err != nil {
			failed = append(failed, errs.Wrap("s.readChannel("+actor.String()+")", err))
		}
		added += n
	}

	return added, errors.Join(failed...)
}

// readChannel keeps the posts the page of a channel shows: new ones, and
// those changed since it was read. Its page no longer showing a channel
// (deleted, made private) leaves what was kept.
func (s *Service) readChannel(ctx context.Context, actor fedi.URL) (int, error) {
	r, doc, err := s.channel(ctx, fedi.ChannelName(actor))
	if errors.Is(err, fedi.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	added := 0
	oldest := s.now().Add(-s.retention)

	for _, d := range doc.Posts {
		p, err := fedi.NewRemotePost(d, r.ID(), s.now())
		if err != nil || p.Published().Before(oldest) {
			continue // a post with nothing to show, or older than the news keep
		}

		isNew, err := s.keepChannelPost(ctx, p, d)
		if err != nil {
			return added, err
		}
		if isNew {
			added++
		}
	}

	return added, nil
}

// keepChannelPost keeps a post of a channel: p if it is new, the kept one
// as d tells it otherwise. True for a new one.
func (s *Service) keepChannelPost(ctx context.Context, p *fedi.RemotePost, d fedi.NoteDoc) (bool, error) {
	kept, err := s.news.ByURI(ctx, p.URI())
	if errors.Is(err, fedi.ErrNotFound) {
		return true, errs.Wrap("s.news.Save", s.news.Save(ctx, p))
	}
	if err != nil {
		return false, errs.Wrap("s.news.ByURI", err)
	}

	again, changed, err := kept.Reread(d, s.now())
	if errors.Is(err, fedi.ErrNotForNews) {
		return false, errs.Wrap("s.news.Delete", s.news.Delete(ctx, kept.URI()))
	}
	if err != nil {
		return false, errs.Wrap("kept.Reread", err)
	}

	if !changed {
		return false, nil
	}

	return false, errs.Wrap("s.news.Save", s.news.Save(ctx, again))
}
