package app

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/reqid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/user/internal/config"
	"github.com/vexner67/freenet/user/internal/notifyclient"
	"github.com/vexner67/freenet/user/internal/purgeclient"
	"github.com/vexner67/freenet/user/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// outboxQueue is a stream of the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	notifyclient.Recorder
}

// delivery is what user tells other services through the outbox: follows
// to notification, erased pages to every service that keeps data of users.
type delivery struct {
	notifier service.Notifier
	eraser   service.Eraser
}

// newDelivery opens a stream and a relay for each service configured. The
// connections are lazy, so the services may start in any order; until one
// is up, its relay retries.
func (a *App) newDelivery(cfg config.Config, newQueue func(stream string) outboxQueue, reg prometheus.Registerer) (delivery, error) {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return delivery{}, errs.Wrap("mtls.ClientCredentials", err)
	}

	d := delivery{notifier: notifyclient.Nop{}}
	var erasures []purgeclient.Recorder

	// stream relays the stream to the service at addr by deliver.
	stream := func(name, addr string, deliver func(*grpc.ClientConn) outbox.Deliver) (outboxQueue, error) {
		conn, err := dial(addr, creds)
		if err != nil {
			return nil, errs.Wrap("dial", err)
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
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: follows notify nobody, erased users keep their notifications")
	} else {
		q, err := stream(notifyclient.Stream, cfg.NotificationServiceAddr, func(conn *grpc.ClientConn) outbox.Deliver {
			api := notificationv1.NewNotificationServiceClient(conn)
			return purgeclient.Notification(api, cfg.NotifyTimeout, notifyclient.NewDeliverer(api, cfg.NotifyTimeout).Deliver)
		})
		if err != nil {
			return delivery{}, err
		}

		d.notifier = notifyclient.NewOutbox(q)
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
			continue
		}

		q, err := stream(s.name, s.addr, s.deliver)
		if err != nil {
			return delivery{}, err
		}
		erasures = append(erasures, q)
	}

	d.eraser = purgeclient.NewOutbox(erasures...)

	return d, nil
}

// dial connects to a service; calls carry the request's ID.
func dial(addr string, creds credentials.TransportCredentials) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, append(reqid.DialOptions(), grpc.WithTransportCredentials(creds))...)
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
