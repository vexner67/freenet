package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/user/internal/config"
	"github.com/vexner67/freenet/user/internal/notifyclient"
	"github.com/vexner67/freenet/user/internal/purgeclient"
	"github.com/vexner67/freenet/user/internal/service"
	"google.golang.org/grpc"
)

// outboxQueue is a stream of the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	notifyclient.Recorder
}

// delivery is what user tells other services through the outbox: follows
// to notification and to post, whose news follow them, erased pages to
// every service that keeps data of users.
type delivery struct {
	notifier service.Notifier
	// gifts tells notification alone: the news do not follow gifts.
	gifts service.GiftNotifier
	// votes tells it of transfers, verified of checkmarks, prime of
	// Prime given as a present.
	votes     service.VotesNotifier
	verified  service.VerifiedNotifier
	prime     service.PrimeNotifier
	eraser    service.Eraser
	reactions service.Reactions
	// walls is nil without a post service: closed pages keep their walls
	// open there.
	walls service.Walls
	// sources is nil without a post service too: nobody to tell whom
	// everybody follows.
	sources service.Sources
}

// notifiers tells each of them, in order, about a follow, an invitation
// and the end of a follow.
type notifiers []service.Notifier

func (n notifiers) Followed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	for _, notifier := range n {
		if err := notifier.Followed(ctx, followerID, followeeID); err != nil {
			return errs.Wrap("notifier.Followed", err)
		}
	}

	return nil
}

func (n notifiers) Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	for _, notifier := range n {
		if err := notifier.Unfollowed(ctx, followerID, followeeID); err != nil {
			return errs.Wrap("notifier.Unfollowed", err)
		}
	}

	return nil
}

func (n notifiers) Invited(ctx context.Context, inviteeID, inviterID uuid.UUID) error {
	for _, notifier := range n {
		if err := notifier.Invited(ctx, inviteeID, inviterID); err != nil {
			return errs.Wrap("notifier.Invited", err)
		}
	}

	return nil
}

// newDelivery opens a stream and a relay for each service configured. The
// connections are lazy, so the services may start in any order; until one
// is up, its relay retries.
func (a *App) newDelivery(cfg config.Config, newQueue func(stream string) outboxQueue, lock purgeclient.Locker, reg prometheus.Registerer) (delivery, error) {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return delivery{}, errs.Wrap("mtls.ClientCredentials", err)
	}

	d := delivery{notifier: notifyclient.Nop{}, gifts: notifyclient.Nop{}, votes: notifyclient.Nop{}, verified: notifyclient.Nop{}, prime: notifyclient.Nop{}, reactions: purgeclient.NoReactions{}}
	var erasures []purgeclient.Recorder

	// stream relays the stream to the service at addr by deliver.
	stream := func(name, addr string, deliver func(*grpc.ClientConn) outbox.Deliver) (outboxQueue, error) {
		conn, err := grpcclient.New(addr, creds, a.tracer.DialOption())
		if err != nil {
			return nil, errs.Wrap("grpcclient.New", err)
		}
		a.conns = append(a.conns, conn)

		q := newQueue(name)

		// The streams share the metrics, told apart by a label.
		relay, err := outbox.NewRelay(q, deliver(conn), prometheus.WrapRegistererWith(prometheus.Labels{"stream": name}, reg), a.logger)
		if err != nil {
			return nil, errs.Wrap("outbox.NewRelay", err)
		}
		a.relays = append(a.relays, relay)

		return q, nil
	}

	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: follows and gifts notify nobody, erased users keep their notifications")
	} else {
		q, err := stream(notifyclient.Stream, cfg.NotificationServiceAddr, func(conn *grpc.ClientConn) outbox.Deliver {
			api := notificationv1.NewNotificationServiceClient(conn)
			return purgeclient.Notification(api, cfg.NotifyTimeout, notifyclient.NewDeliverer(api, cfg.NotifyTimeout).Deliver)
		})
		if err != nil {
			return delivery{}, err
		}

		toNotification := notifyclient.NewOutbox(q)
		d.notifier, d.gifts, d.votes, d.verified, d.prime = toNotification, toNotification, toNotification, toNotification, toNotification
		erasures = append(erasures, q)
	}

	for _, s := range []struct {
		name, addr, variable string
		deliver              func(*grpc.ClientConn) outbox.Deliver
	}{
		{purgeclient.PostStream, cfg.PostServiceAddr, "POST_SERVICE_ADDR", func(c *grpc.ClientConn) outbox.Deliver {
			return purgeclient.Post(postv1.NewPostServiceClient(c), cfg.NotifyTimeout)
		}},
		{purgeclient.MediaStream, cfg.MediaServiceAddr, "MEDIA_SERVICE_ADDR", func(c *grpc.ClientConn) outbox.Deliver {
			return purgeclient.Media(mediav1.NewMediaServiceClient(c), cfg.NotifyTimeout)
		}},
		{purgeclient.MessageStream, cfg.MessageServiceAddr, "MESSAGE_SERVICE_ADDR", func(c *grpc.ClientConn) outbox.Deliver {
			return purgeclient.Message(messagev1.NewMessageServiceClient(c), cfg.NotifyTimeout)
		}},
		{purgeclient.ClubStream, cfg.ClubServiceAddr, "CLUB_SERVICE_ADDR", func(c *grpc.ClientConn) outbox.Deliver {
			return purgeclient.Club(clubv1.NewClubServiceClient(c), cfg.NotifyTimeout)
		}},
	} {
		if s.addr == "" {
			a.logger.Warn(s.variable + " is not set: erased users keep their data in " + s.name)
			if s.name == purgeclient.PostStream {
				a.logger.Warn("POST_SERVICE_ADDR is not set: the news do not follow the follows")
			}
			continue
		}

		q, err := stream(s.name, s.addr, s.deliver)
		if err != nil {
			return delivery{}, err
		}
		erasures = append(erasures, q)

		if s.name == purgeclient.PostStream {
			d.reactions = purgeclient.NewReactions(q)
			d.walls = purgeclient.NewWalls(q)
			follows := purgeclient.NewFollows(q, lock)
			d.notifier, d.sources = notifiers{d.notifier, follows}, follows
		}
	}

	d.eraser = purgeclient.NewOutbox(erasures...)

	return d, nil
}

// runSync tells post whom everybody follows every interval until ctx
// ends; a failure is logged and the next round starts over.
func (a *App) runSync(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		told, err := a.sync.SyncAll(ctx)
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "sync of feed sources", "error", err)
		}

		a.logger.InfoContext(ctx, "follows told to post anew", "users", told)
	}
}

// runPurge erases the pages that are due every interval until ctx ends; a
// failure is logged and tried again next time.
func (a *App) runPurge(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		erased, err := a.purge.PurgeDue(ctx)
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "purge of deleted pages", "error", err)
		}
		if erased > 0 {
			a.logger.InfoContext(ctx, "deleted pages erased", "count", erased)
		}
	}
}

// runAuctions settles the auctions of gifts that ended every interval
// until ctx ends; a failure is logged and tried again next time.
func (a *App) runAuctions(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		settled, err := a.gifts.SettleDue(ctx)
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "settlement of gift auctions", "error", err)
		}
		if settled > 0 {
			a.logger.InfoContext(ctx, "gift auctions settled", "count", settled)
		}
	}
}

// runBank collects the loans past their term every interval until ctx
// ends; a failure is logged and tried again next time.
func (a *App) runBank(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		collected, err := a.banks.CollectDue(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "collection of overdue loans", "error", err)
		}
		if collected > 0 {
			a.logger.InfoContext(ctx, "overdue loans collected", "count", collected)
		}
	}
}

// runEndless gives the administrators back the votes they spent at once
// and then every interval, until ctx ends; a failure is logged and tried
// again next time.
func (a *App) runEndless(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		if _, err := a.endless.Endow(ctx); err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "endless votes of the administrators", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// runFirms counts the hours of the economy of the enterprises that ended,
// looking every interval, until ctx is done. Replicas count each hour
// once: the city is locked while counted.
func (a *App) runFirms(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		counted, err := a.firms.Tick(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "hours of the enterprises", "error", err)
		}
		if counted > 0 {
			a.logger.InfoContext(ctx, "hours of the enterprises counted", "count", counted)
		}
	}
}

// runHoldem moves on the poker tables due every interval until ctx is
// done: turns pass and hands are dealt at tables nobody reads.
func (a *App) runHoldem(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		if _, err := a.holdems.SweepDue(ctx); err != nil && ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "poker tables", "error", err)
		}
	}
}
