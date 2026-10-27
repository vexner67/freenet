package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/grpcclient"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/logger"
	"github.com/vexner67/freenet/pkg/metrics"
	"github.com/vexner67/freenet/pkg/mtls"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/ratelimit"
	"github.com/vexner67/freenet/pkg/tracing"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/clubclient"
	"github.com/vexner67/freenet/post/internal/config"
	postgrpc "github.com/vexner67/freenet/post/internal/grpc"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/mediaclient"
	"github.com/vexner67/freenet/post/internal/notifyclient"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
	"github.com/vexner67/freenet/post/migrations"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// probeTimeout bounds the healthcheck.
	probeTimeout   = 3 * time.Second
	connectTimeout = 5 * time.Second
	migrateTimeout = time.Minute
)

type App struct {
	logger  *slog.Logger
	tracer  *tracing.Tracer
	pool    *pgxpool.Pool
	limiter ratelimit.Store
	// conns are the clients of notification and media, one outbox stream
	// each, and of club; queues are the PostgreSQL streams, held by the
	// relays.
	conns   []*grpc.ClientConn
	queues  []*outbox.Postgres
	relays  []*outbox.Relay
	server  *grpcserver.Server
	metrics *metrics.Server
	addr    string
	// postponed publishes the postponed posts whose time has come, every
	// postponedEvery.
	postponed      *service.PostponedService
	postponedEvery time.Duration
	// stories deletes the stories whose day is over, every storiesEvery.
	stories      *service.StoryService
	storiesEvery time.Duration
}

func New(cfg config.Config) (*App, error) {
	log, err := logger.New(logger.Config{
		LogLevel:  cfg.LogLevel,
		LogFormat: cfg.LogFormat,
	})
	if err != nil {
		return nil, errs.Wrap("logger.New", err)
	}

	a := &App{
		logger: log,
		addr:   fmt.Sprintf(":%d", cfg.GRPCPort),
	}

	// On failure Close releases whatever was opened before it.
	if err = a.build(cfg); err != nil {
		a.Close()
		return nil, errs.Wrap("a.build", err)
	}

	return a, nil
}

func (a *App) build(cfg config.Config) error {
	tracer, err := tracing.New(context.Background(), "post", cfg.Tracing, a.logger)
	if err != nil {
		return errs.Wrap("tracing.New", err)
	}
	a.tracer = tracer

	reg := metrics.NewRegistry()

	repos, err := a.newRepositories(cfg)
	if err != nil {
		return errs.Wrap("a.newRepositories", err)
	}

	notifier, err := a.newNotifier(cfg, repos.newQueue, reg)
	if err != nil {
		return errs.Wrap("a.newNotifier", err)
	}

	photos, err := a.newPhotoKeeper(cfg, repos.newQueue, reg)
	if err != nil {
		return errs.Wrap("a.newPhotoKeeper", err)
	}

	moderators, err := a.newModerators(cfg)
	if err != nil {
		return errs.Wrap("a.newModerators", err)
	}

	limiter, err := a.newLimiter(cfg)
	if err != nil {
		return errs.Wrap("a.newLimiter", err)
	}

	publicKey, err := jwtauth.LoadPublicKey(cfg.JWTPublicKeyPath)
	if err != nil {
		return errs.Wrap("jwtauth.LoadPublicKey", err)
	}

	creds, err := mtls.ServerCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ServerCredentials", err)
	}

	if !cfg.TLS.Enabled() {
		a.logger.Warn("mTLS is off: internal RPCs are open to anyone who can connect, do not use in production")
	}

	a.server, err = grpcserver.New(
		a.logger,
		reg,
		grpc.Creds(creds),
		a.tracer.ServerOption(),
		grpc.ChainUnaryInterceptor(
			postgrpc.ErrorInterceptor(a.logger),
			restrictInternal(cfg),
			jwtauth.UnaryServerInterceptor(
				jwtauth.NewVerifier(publicKey, cfg.JWTIssuer),
				jwtauth.Access{Public: postgrpc.PublicMethods()},
			),
			ratelimit.UnaryServerInterceptor(limiter, rateLimits(cfg), a.logger),
		),
	)
	if err != nil {
		return errs.Wrap("grpcserver.New", err)
	}
	albums := service.NewAlbumService(service.AlbumDeps{
		Albums: repos.albums, Photos: repos.albumPhotos, Tx: repos.tx, Media: photos, Moderators: moderators,
		Likes: repos.photoLikes, Comments: repos.photoComments, CommentLikes: repos.photoCommentLikes, Notifier: notifier,
		Pages: repos.pages,
	})

	posts := service.NewPostService(service.PostDeps{
		Posts: repos.posts, Pins: repos.posts, Comments: repos.comments, Likes: repos.likes, Tx: repos.tx, Notifier: notifier, Photos: photos,
		Moderators: moderators, AlbumPhotos: repos.albumPhotos, WallPhotos: albums, Switches: repos.posts, Walls: repos.sources,
		Polls: repos.polls, Pages: repos.pages,
	})
	a.postponed = service.NewPostponedService(service.PostponedDeps{
		Postponed: repos.postponed, Poster: posts, Tx: repos.tx, Photos: photos, Moderators: moderators,
	})
	a.postponedEvery = cfg.PostponedInterval
	a.stories = service.NewStoryService(service.StoryDeps{
		Stories: repos.stories, Views: repos.storyViews, Likes: repos.storyLikes, Comments: repos.storyComments,
		Tx: repos.tx, Photos: photos, Notifier: notifier,
	})
	a.storiesEvery = cfg.StoryCleanupInterval

	postv1.RegisterPostServiceServer(
		a.server.Registrar(),
		postgrpc.NewPostHandler(postgrpc.Services{
			Posts: posts,
			Comments: service.NewCommentService(service.CommentDeps{
				Posts: repos.posts, Comments: repos.comments, Likes: repos.commentLikes, Tx: repos.tx, Notifier: notifier, Photos: photos,
				Moderators: moderators, Walls: repos.sources, Pages: repos.pages,
			}),
			Suggestions: service.NewSuggestionService(service.SuggestionDeps{
				Suggestions: repos.suggestions, Posts: repos.posts, Tx: repos.tx, Notifier: notifier, Photos: photos, Moderators: moderators,
			}),
			Postponed: a.postponed,
			Topics: service.NewTopicService(service.TopicDeps{
				Topics: repos.topics, Messages: repos.topicMessages, Likes: repos.topicMessages, Tx: repos.tx,
				Notifier: notifier, Photos: photos, Moderators: moderators, Pages: repos.pages,
			}),
			Albums:    albums,
			Polls:     service.NewPollService(service.PollDeps{Polls: repos.polls, Posts: repos.posts, Moderators: moderators, Walls: repos.sources, Tx: repos.tx}),
			Documents: service.NewDocumentService(repos.documents, repos.tx, photos),
			Articles:  service.NewArticleService(repos.articles, repos.tx, photos, repos.pages),
			Stories:   a.stories,
			Faves: service.NewFaveService(service.FaveDeps{
				PostLikes: repos.likes, CommentLikes: repos.commentLikes, PhotoLikes: repos.photoLikes,
				Posts: repos.posts, Comments: repos.comments, Photos: repos.albumPhotos, Albums: repos.albums, Walls: repos.sources,
			}),
			Purge: service.NewPurgeService(service.PurgeDeps{
				Posts: repos.posts, Likes: repos.likes, CommentLikes: repos.commentLikes, Comments: repos.comments, Suggestions: repos.suggestions,
				Votes:  repos.polls,
				Topics: repos.topics, Albums: repos.albums, Tx: repos.tx, Notifier: notifier, Photos: photos,
				Held: repos.held, Sources: repos.sources, Postponed: repos.postponed, Documents: repos.documents,
				Stories: repos.stories, Articles: repos.articles,
			}),
			ClubPurge: service.NewClubPurgeService(service.ClubPurgeDeps{
				Posts: repos.posts, Comments: repos.comments, Suggestions: repos.suggestions, Postponed: repos.postponed,
				Topics: repos.topics, Albums: repos.albums, Sources: repos.sources, Tx: repos.tx, Notifier: notifier, Media: photos,
			}),
			Holds:   service.NewHoldService(repos.tx, repos.held...),
			Sources: service.NewSourceService(repos.sources),
		}, a.logger),
	)

	if cfg.MetricsPort == 0 {
		a.logger.Info("METRICS_PORT is not set: metrics are not served")
	} else {
		a.metrics = metrics.NewServer(fmt.Sprintf(":%d", cfg.MetricsPort), reg, a.logger)
	}

	a.logger.Info("post configured", "storage", cfg.Storage, "mtls", cfg.TLS.Enabled(),
		"notifications", cfg.NotificationServiceAddr != "", "media", cfg.MediaServiceAddr != "")

	return nil
}

// restrictInternal lets only their services call the internal methods; with
// mTLS off, which is for development, nobody can tell who calls.
func restrictInternal(cfg config.Config) grpc.UnaryServerInterceptor {
	if !cfg.TLS.Enabled() {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}

	return mtls.RestrictMethods(postgrpc.InternalMethods())
}

// rateLimits are what each user may write how often.
func rateLimits(cfg config.Config) map[string]ratelimit.Rules {
	return map[string]ratelimit.Rules{
		postv1.PostService_CreatePost_FullMethodName:         cfg.PostLimits,
		postv1.PostService_RepostPost_FullMethodName:         cfg.PostLimits,
		postv1.PostService_CreateComment_FullMethodName:      cfg.CommentLimits,
		postv1.PostService_LikePost_FullMethodName:           cfg.LikeLimits,
		postv1.PostService_UnlikePost_FullMethodName:         cfg.LikeLimits,
		postv1.PostService_VotePoll_FullMethodName:           cfg.LikeLimits,
		postv1.PostService_RetractPollVote_FullMethodName:    cfg.LikeLimits,
		postv1.PostService_LikeComment_FullMethodName:        cfg.LikeLimits,
		postv1.PostService_LikeTopicMessage_FullMethodName:   cfg.LikeLimits,
		postv1.PostService_LikeAlbumPhoto_FullMethodName:     cfg.LikeLimits,
		postv1.PostService_CreatePhotoComment_FullMethodName: cfg.CommentLimits,
		postv1.PostService_LikePhotoComment_FullMethodName:   cfg.LikeLimits,
		postv1.PostService_UnlikePhotoComment_FullMethodName: cfg.LikeLimits,
		postv1.PostService_UnlikeComment_FullMethodName:      cfg.LikeLimits,
		postv1.PostService_EditPost_FullMethodName:           cfg.EditLimits,
		// What the owner of a wall switches on a post is an edit of it.
		postv1.PostService_ArchivePost_FullMethodName:   cfg.EditLimits,
		postv1.PostService_UnarchivePost_FullMethodName: cfg.EditLimits,
		postv1.PostService_ArchiveWall_FullMethodName:   cfg.EditLimits,
		postv1.PostService_CloseComments_FullMethodName: cfg.EditLimits,
		postv1.PostService_OpenComments_FullMethodName:  cfg.EditLimits,
		// A suggestion is content for the admins to read; publishing it is
		// a post for everyone. Each has its own counter.
		postv1.PostService_SuggestPost_FullMethodName:       cfg.PostLimits,
		postv1.PostService_PublishSuggestion_FullMethodName: cfg.PostLimits,
		// A postponed post is a post to come, published at once a post
		// now; changing it is an edit.
		postv1.PostService_PostponePost_FullMethodName:     cfg.PostLimits,
		postv1.PostService_PublishPostponed_FullMethodName: cfg.PostLimits,
		postv1.PostService_EditPostponed_FullMethodName:    cfg.EditLimits,
		// A topic is a post of the discussions, a message in it a comment.
		postv1.PostService_CreateTopic_FullMethodName:        cfg.PostLimits,
		postv1.PostService_CreateTopicMessage_FullMethodName: cfg.CommentLimits,
		// An album and the photos added to it are posts of the albums.
		postv1.PostService_CreateAlbum_FullMethodName:    cfg.PostLimits,
		postv1.PostService_AddAlbumPhotos_FullMethodName: cfg.PostLimits,
		postv1.PostService_AddAvatarPhoto_FullMethodName: cfg.PostLimits,
		postv1.PostService_EditAlbum_FullMethodName:      cfg.EditLimits,
		postv1.PostService_EditAlbumPhoto_FullMethodName: cfg.EditLimits,
		postv1.PostService_SetAlbumCover_FullMethodName:  cfg.EditLimits,
		postv1.PostService_MoveAlbumPhoto_FullMethodName: cfg.EditLimits,
		// A document is added after every upload, which media counts too.
		postv1.PostService_AddDocument_FullMethodName:    cfg.PostLimits,
		postv1.PostService_RenameDocument_FullMethodName: cfg.EditLimits,
		// An article is a long post.
		postv1.PostService_CreateArticle_FullMethodName: cfg.PostLimits,
		postv1.PostService_EditArticle_FullMethodName:   cfg.EditLimits,
		// A story is told, liked and commented as a post.
		postv1.PostService_CreateStory_FullMethodName:        cfg.PostLimits,
		postv1.PostService_LikeStory_FullMethodName:          cfg.LikeLimits,
		postv1.PostService_UnlikeStory_FullMethodName:        cfg.LikeLimits,
		postv1.PostService_CreateStoryComment_FullMethodName: cfg.CommentLimits,
	}
}

// newLimiter counts in Redis when it is configured, so that all replicas
// share the limits, and in memory otherwise.
func (a *App) newLimiter(cfg config.Config) (ratelimit.Limiter, error) {
	if cfg.RedisAddr == "" {
		a.logger.Warn("REDIS_ADDR is not set: rate limits are counted per replica")
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	var err error

	a.limiter, err = ratelimit.Open(ctx, cfg.RedisAddr)
	if err != nil {
		return nil, errs.Wrap("ratelimit.Open", err)
	}

	return a.limiter, nil
}

// newNotifier records notifications in the outbox and sets up the relay
// that delivers them to the notification service.
func (a *App) newNotifier(cfg config.Config, newQueue func(stream string) outboxQueue, reg prometheus.Registerer) (service.Notifier, error) {
	if cfg.NotificationServiceAddr == "" {
		a.logger.Warn("NOTIFICATION_SERVICE_ADDR is not set: likes and comments notify nobody")
		return notifyclient.Nop{}, nil
	}

	// Most of what post tells is about one post: the likes of different
	// posts do not wait for each other.
	q, err := a.stream(cfg, notifyclient.Stream, cfg.NotificationServiceAddr, newQueue, reg, func(conn *grpc.ClientConn) outbox.Deliver {
		return notifyclient.NewDeliverer(notificationv1.NewNotificationServiceClient(conn), cfg.NotifyTimeout).Deliver
	}, outbox.Keyed(notifyclient.Key))
	if err != nil {
		return nil, err
	}

	return notifyclient.NewOutbox(q), nil
}

// newModerators asks the club service about the walls of communities. The
// connection is lazy, so the services may start in any order.
func (a *App) newModerators(cfg config.Config) (service.SuggestionModerators, error) {
	if cfg.ClubServiceAddr == "" {
		a.logger.Warn("CLUB_SERVICE_ADDR is not set: community admins cannot delete or pin others' posts and comments, nor hear about suggested posts")
		return clubclient.Nop{}, nil
	}

	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	conn, err := grpcclient.New(cfg.ClubServiceAddr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, errs.Wrap("grpcclient.New", err)
	}
	a.conns = append(a.conns, conn)

	return clubclient.New(clubv1.NewClubServiceClient(conn), cfg.NotifyTimeout), nil
}

// mediaKeeper records what media has to do with the photos of posts and
// the documents of people.
type mediaKeeper interface {
	service.MediaPurger
	service.DocumentKeeper
}

// newPhotoKeeper records in the outbox what media has to do with the
// photos of posts and the documents and sets up the relay that tells it.
func (a *App) newPhotoKeeper(cfg config.Config, newQueue func(stream string) outboxQueue, reg prometheus.Registerer) (mediaKeeper, error) {
	if cfg.MediaServiceAddr == "" {
		a.logger.Warn("MEDIA_SERVICE_ADDR is not set: media deletes the photos of posts as never published, those of deleted posts stay")
		return mediaclient.Nop{}, nil
	}

	q, err := a.stream(cfg, mediaclient.Stream, cfg.MediaServiceAddr, newQueue, reg, func(conn *grpc.ClientConn) outbox.Deliver {
		return mediaclient.NewDeliverer(mediav1.NewMediaServiceClient(conn), cfg.NotifyTimeout).Deliver
	})
	if err != nil {
		return nil, err
	}

	return mediaclient.NewOutbox(q), nil
}

// stream opens the outbox stream name and its relay, which delivers to the
// service at addr by deliver. The connection is lazy, so the services may
// start in any order; until the service is up, the relay retries.
func (a *App) stream(
	cfg config.Config, name, addr string, newQueue func(stream string) outboxQueue,
	reg prometheus.Registerer, deliver func(*grpc.ClientConn) outbox.Deliver, opts ...outbox.Option,
) (outboxQueue, error) {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return nil, errs.Wrap("mtls.ClientCredentials", err)
	}

	conn, err := grpcclient.New(addr, creds, a.tracer.DialOption())
	if err != nil {
		return nil, errs.Wrap("grpcclient.New", err)
	}
	a.conns = append(a.conns, conn)

	q := newQueue(name)

	// The streams share the metrics, told apart by a label.
	relay, err := outbox.NewRelay(q, deliver(conn), prometheus.WrapRegistererWith(prometheus.Labels{"stream": name}, reg), a.logger, opts...)
	if err != nil {
		return nil, errs.Wrap("outbox.NewRelay", err)
	}
	a.relays = append(a.relays, relay)

	return q, nil
}

// outboxQueue is a stream of the outbox of the storage in use.
type outboxQueue interface {
	outbox.Queue
	notifyclient.Recorder
}

type repositories struct {
	posts interface {
		service.PostRepository
		service.PinRepository
		service.PostSwitchRepository
		service.PostEraser
	}
	likes interface {
		service.LikeRepository
		service.ReactionEraser
		service.FaveRepository
	}
	comments interface {
		service.CommentRepository
		service.PostCommentEraser
		service.CommentEraser
	}
	commentLikes interface {
		service.CommentLikeRepository
		service.ReactionEraser
		service.FaveRepository
	}
	suggestions interface {
		service.SuggestionRepository
		service.SuggestionEraser
		service.WallSuggestionEraser
	}
	postponed interface {
		service.PostponedRepository
		service.PostponedEraser
		service.WallPostponedEraser
	}
	topics interface {
		service.TopicRepository
		service.TopicEraser
		service.ClubTopicEraser
	}
	topicMessages interface {
		service.TopicMessageRepository
		service.TopicMessageLikeRepository
	}
	albums interface {
		service.AlbumRepository
		service.AlbumEraser
		service.OwnerAlbumEraser
	}
	polls interface {
		service.PollRepository
		service.ReactionEraser
	}
	documents interface {
		service.DocumentRepository
		service.DocumentEraser
	}
	articles interface {
		service.ArticleRepository
		service.ArticleEraser
	}
	stories interface {
		service.StoryRepository
		service.StoryEraser
	}
	storyViews    service.StoryViewRepository
	storyLikes    service.StoryLikeRepository
	storyComments service.StoryCommentRepository
	// held keep the likes and votes of deleted pages aside.
	held []service.ReactionHolder
	// pages knows the deleted pages, for what is read by its ID.
	pages service.Pages
	// sources are whom each reader reads in the news.
	sources     service.SourceRepository
	albumPhotos interface {
		service.AlbumPhotoRepository
		service.FavePhotos
	}
	photoLikes interface {
		service.AlbumPhotoLikeRepository
		service.FaveRepository
	}
	// photoComments are the comments of photos of albums.
	photoComments service.PhotoCommentRepository
	// photoCommentLikes are the hearts of those comments.
	photoCommentLikes service.PhotoCommentLikeRepository
	tx                service.TxManager
	// newQueue opens a stream of the outbox.
	newQueue func(stream string) outboxQueue
}

func (a *App) newRepositories(cfg config.Config) (repositories, error) {
	if cfg.Storage == config.StorageInmem {
		store := inmem.NewStore()
		topics := inmem.NewTopicRepository()
		albums := inmem.NewAlbumRepository()
		return repositories{
			posts:        store.Posts(),
			likes:        store.Likes(),
			polls:        store.Polls(),
			comments:     store.Comments(),
			commentLikes: store.CommentLikes(),
			suggestions:  inmem.NewSuggestionRepository(),
			postponed:    inmem.NewPostponedRepository(),
			topics:       topics,
			tx:           inmem.TxManager{},

			topicMessages: topics.Messages(),
			albums:        albums,
			albumPhotos:   albums.Photos(),
			photoLikes:    albums.PhotoLikes(),
			photoComments: albums.PhotoComments(), photoCommentLikes: albums.PhotoComments(),
			documents: inmem.NewDocumentRepository(),
			articles:  inmem.NewArticleRepository(),
			stories:   store.Stories(), storyViews: store.StoryViews(),
			storyLikes: store.StoryLikes(), storyComments: store.StoryComments(),
			held: []service.ReactionHolder{store.Holds(), topics, albums},
			// Every store is told of a deleted page; one answers for all.
			pages:   store.Holds(),
			sources: store.Sources(),
			newQueue: func(string) outboxQueue {
				return outbox.NewMemory()
			},
		}, nil
	}

	if cfg.MigrateOnStart {
		ctx, cancel := context.WithTimeout(context.Background(), migrateTimeout)
		defer cancel()

		applied, err := pgpool.Migrate(ctx, cfg.DatabaseURL, migrations.FS)
		if err != nil {
			return repositories{}, errs.Wrap("pgpool.Migrate", err)
		}

		a.logger.Info("migrations applied", "count", applied)
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	var err error

	a.pool, err = pgpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return repositories{}, errs.Wrap("pgpool.New", err)
	}

	topics := postgres.NewTopicRepository(a.pool)
	albums := postgres.NewAlbumRepository(a.pool)
	held := postgres.NewHoldRepository(a.pool)

	return repositories{
		posts:        postgres.NewPostRepository(a.pool),
		likes:        postgres.NewLikeRepository(a.pool),
		polls:        postgres.NewPollRepository(a.pool),
		comments:     postgres.NewCommentRepository(a.pool),
		commentLikes: postgres.NewCommentLikeRepository(a.pool),
		suggestions:  postgres.NewSuggestionRepository(a.pool),
		postponed:    postgres.NewPostponedRepository(a.pool),
		topics:       topics,
		tx:           pgpool.NewTxManager(a.pool),

		topicMessages: topics.Messages(),
		albums:        albums,
		albumPhotos:   albums.Photos(),
		photoLikes:    albums.PhotoLikes(),
		photoComments: albums.PhotoComments(), photoCommentLikes: albums.PhotoComments(),
		documents: postgres.NewDocumentRepository(a.pool),
		articles:  postgres.NewArticleRepository(a.pool),
		stories:   postgres.NewStoryRepository(a.pool), storyViews: postgres.NewStoryViewRepository(a.pool),
		storyLikes: postgres.NewStoryLikeRepository(a.pool), storyComments: postgres.NewStoryCommentRepository(a.pool),
		held:    []service.ReactionHolder{held},
		pages:   held,
		sources: postgres.NewSourceRepository(a.pool),
		newQueue: func(stream string) outboxQueue {
			q := outbox.NewPostgres(a.pool, stream)
			a.queues = append(a.queues, q)
			return q
		},
	}, nil
}

// Run serves and delivers notifications until ctx is cancelled or the gRPC
// or metrics server fails, which stops the rest too.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return errs.Wrap("a.server.Run", a.server.Run(ctx, a.addr))
	})

	if a.metrics != nil {
		g.Go(func() error {
			return errs.Wrap("a.metrics.Run", a.metrics.Run(ctx))
		})
	}

	for _, relay := range a.relays {
		g.Go(func() error {
			return errs.Wrap("relay.Run", relay.Run(ctx))
		})
	}

	g.Go(func() error {
		a.runEvery(ctx, a.postponedEvery, a.postponed.PublishDue, "publish postponed posts", "postponed posts published")
		return nil
	})

	g.Go(func() error {
		a.runEvery(ctx, a.storiesEvery, a.stories.DeleteExpired, "delete expired stories", "expired stories deleted")
		return nil
	})

	return g.Wait()
}

// runEvery does a round of work at once and then every so often, until
// ctx is cancelled. A failed round is logged and tried again next time:
// the work stays to be done. A round reports how much it did.
func (a *App) runEvery(ctx context.Context, every time.Duration, round func(context.Context) (int, error), failed, done string) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		count, err := round(ctx)

		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			a.logger.ErrorContext(ctx, failed, "error", err)
		case count > 0:
			a.logger.InfoContext(ctx, done, "count", count)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Healthcheck asks the service running with cfg on this host whether it
// serves: the container healthcheck, as the image has no shell or curl.
func Healthcheck(ctx context.Context, cfg config.Config) error {
	creds, err := mtls.ClientCredentials(cfg.TLS)
	if err != nil {
		return errs.Wrap("mtls.ClientCredentials", err)
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return errs.Wrap("grpcserver.Probe", grpcserver.Probe(ctx, fmt.Sprintf("localhost:%d", cfg.GRPCPort), creds))
}

// Close releases everything New opened; safe after a partial New.
func (a *App) Close() {
	if a.tracer != nil {
		if err := a.tracer.Close(); err != nil {
			a.logger.Error("tracer close", "error", err)
		}
	}

	if a.limiter != nil {
		if err := a.limiter.Close(); err != nil {
			a.logger.Error("rate limiter close", "error", err)
		}
	}

	// After Run: the relays own the outbox connections while they run.
	for _, q := range a.queues {
		q.Close()
	}

	for _, conn := range a.conns {
		if err := conn.Close(); err != nil {
			a.logger.Error("connection close", "error", err, "target", conn.Target())
		}
	}

	if a.pool != nil {
		a.pool.Close()
	}
}
