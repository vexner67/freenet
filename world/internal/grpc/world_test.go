package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	worldgrpc "github.com/vexner67/freenet/world/internal/grpc"
	"github.com/vexner67/freenet/world/internal/inmem"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

const issuer = "freenet-test"

// line is a strip of land of 10 provinces.
type lineMaker struct{}

func (lineMaker) Make(context.Context, uint64, int) (service.MadeMap, error) {
	a := &world.Atlas{Width: 64, Height: 32}
	for i := range 10 {
		p := world.Province{ID: i, Name: "P", Terrain: world.TerrainPlains, Population: 100}
		if i > 0 {
			p.Links = append(p.Links, world.Link{To: i - 1})
		}
		if i < 9 {
			p.Links = append(p.Links, world.Link{To: i + 1})
		}
		a.Provinces = append(a.Provinces, p)
	}

	return service.MadeMap{Atlas: a, Raster: []byte{1, 2}, Tiles: []service.Tile{{PNG: []byte{3}}}}, nil
}

// wallets give every page as many votes as it asks for.
type wallets struct {
	mu    sync.Mutex
	spent map[uuid.UUID]int64
}

func (w *wallets) Charge(_ context.Context, op service.VoteOp) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.spent[op.UserID] += op.Votes

	return 0, nil
}

func (w *wallets) Pay(context.Context, service.VoteOp) (int64, error) { return 0, nil }

type nopClubs struct{}

func (nopClubs) CreateStateClub(context.Context, uuid.UUID, uuid.UUID, string, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}

type testServer struct {
	client worldv1.WorldServiceClient
	key    *rsa.PrivateKey
	worlds *service.WorldService
}

// newServer wires the handler the same way app.New does, without mTLS.
func newServer(t *testing.T) testServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	lis := bufconn.Listen(1 << 20)
	verifier := jwtauth.NewVerifier(&key.PublicKey, issuer)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		worldgrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: worldgrpc.PublicMethods()}),
	))

	store := inmem.NewStore()
	worlds := service.NewWorldService(service.WorldDeps{Worlds: store, Maps: store, Games: store, Rules: store, Maker: lineMaker{}, Tx: inmem.TxManager{}})
	games := service.NewGameService(service.GameDeps{
		Worlds: worlds, Games: store, Events: store, Wallets: &wallets{spent: map[uuid.UUID]int64{}}, Clubs: nopClubs{}, Tx: inmem.TxManager{},
	})
	worldv1.RegisterWorldServiceServer(srv, worldgrpc.NewWorldHandler(worlds, games))

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return testServer{client: worldv1.NewWorldServiceClient(conn), key: key, worlds: worlds}
}

// as returns a context of the given user with a signed access token.
func (s testServer) as(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}

	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

// create makes a world, as the administrators do through the gateway;
// its map is still to be made.
func (s testServer) create(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	created, err := s.client.CreateWorld(ctx, &worldv1.CreateWorldRequest{Grid: 2048})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetWorld().GetStatus() != worldv1.WorldStatus_WORLD_STATUS_GENERATING || created.GetWorld().GetSeed() == 0 {
		t.Fatalf("CreateWorld = %v", created)
	}

	return created.GetWorld().GetId()
}

var flag = &worldv1.Flag{Pattern: "tricolor_v", Colors: []int32{1, 2, 3}, Emblem: "sun", EmblemColor: 4, EmblemAt: "center"}

func TestWorldWithoutMap(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ctx := context.Background()

	if _, err := s.client.GetWorld(ctx, &worldv1.GetWorldRequest{}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetWorld without a world: %v", err)
	}
	if _, err := s.client.CreateWorld(ctx, &worldv1.CreateWorldRequest{Grid: 100}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateWorld on a bad grid: %v", err)
	}

	id := s.create(t)
	// Nothing made the map: the world is not ready.
	if _, err := s.client.OpenWorld(ctx, &worldv1.OpenWorldRequest{WorldId: id}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("OpenWorld before the map: %v", err)
	}
	if _, err := s.client.OpenWorld(ctx, &worldv1.OpenWorldRequest{WorldId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("OpenWorld of a bad id: %v", err)
	}
	if _, err := s.client.GetTile(ctx, &worldv1.GetTileRequest{WorldId: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetTile without a map: %v", err)
	}
	if _, err := s.client.EndWorld(ctx, &emptypb.Empty{}); status.Code(err) != codes.NotFound {
		t.Fatalf("EndWorld without a world: %v", err)
	}

	rules, err := s.client.GetWorldRules(ctx, &emptypb.Empty{})
	if err != nil || rules.GetFoundCost() != world.DefaultRules.FoundCost {
		t.Fatalf("GetWorldRules = %v, %v", rules, err)
	}
	rules.ColonizeHours = 0
	if _, err = s.client.SetWorldRules(ctx, rules); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetWorldRules of bad rules: %v", err)
	}
	rules.ColonizeHours = 6
	if _, err = s.client.SetWorldRules(ctx, rules); err != nil {
		t.Fatal(err)
	}

	list, err := s.client.ListWorlds(ctx, &emptypb.Empty{})
	if err != nil || len(list.GetWorlds()) != 1 {
		t.Fatalf("ListWorlds = %v, %v", list, err)
	}
}

// open makes a world with its map and opens it.
func (s testServer) open(t *testing.T) string {
	t.Helper()

	id := s.create(t)
	if made, err := s.worlds.Generate(context.Background()); err != nil || !made {
		t.Fatalf("Generate = %v, %v", made, err)
	}
	if _, err := s.client.OpenWorld(context.Background(), &worldv1.OpenWorldRequest{WorldId: id}); err != nil {
		t.Fatal(err)
	}

	return id
}

func TestStates(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	id := s.open(t)
	ann, bob := uuid.New(), uuid.New()
	asAnn, asBob := s.as(t, ann), s.as(t, bob)
	guest := context.Background()

	if a, err := s.client.GetAtlas(guest, &worldv1.GetAtlasRequest{WorldId: id}); err != nil || len(a.GetProvinces()) != 10 || a.GetProvinces()[1].GetLinks()[0].GetTo() != 0 {
		t.Fatalf("GetAtlas = %v, %v", a, err)
	}
	if r, err := s.client.GetRaster(guest, &worldv1.GetRasterRequest{WorldId: id}); err != nil || len(r.GetRaster()) != 2 {
		t.Fatalf("GetRaster = %v, %v", r, err)
	}
	if tile, err := s.client.GetTile(guest, &worldv1.GetTileRequest{WorldId: id}); err != nil || len(tile.GetPng()) != 1 {
		t.Fatalf("GetTile = %v, %v", tile, err)
	}

	found := &worldv1.FoundStateRequest{Name: "Бореалия", Motto: "Вперёд", Color: 5, Flag: flag, Government: "republic", Capital: 1}
	if _, err := s.client.FoundState(guest, found); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("FoundState by a guest: %v", err)
	}
	for _, bad := range []*worldv1.FoundStateRequest{
		{Name: "", Flag: flag, Government: "republic"},
		{Name: "Нордия", Flag: flag, Government: "anarchy"},
		{Name: "Нордия", Flag: &worldv1.Flag{Pattern: "plain"}, Government: "republic"},
		{Name: "Нордия", Flag: flag, Government: "republic", Color: 99},
	} {
		if _, err := s.client.FoundState(asAnn, bad); status.Code(err) != codes.InvalidArgument {
			t.Errorf("FoundState(%v) = %v", bad, err)
		}
	}

	created, err := s.client.FoundState(asAnn, found)
	if err != nil {
		t.Fatal(err)
	}
	stateID := created.GetStateId()
	if _, err = s.client.FoundState(asBob, &worldv1.FoundStateRequest{Name: "бореалия", Flag: flag, Government: "republic", Capital: 8}); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("FoundState of a taken name: %v", err)
	}
	if _, err = s.client.FoundState(asBob, &worldv1.FoundStateRequest{Name: "Нордия", Flag: flag, Government: "republic", Capital: 3}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("FoundState next to a capital: %v", err)
	}

	got, err := s.client.GetWorld(asAnn, &worldv1.GetWorldRequest{})
	if err != nil || len(got.GetStates()) != 1 || got.GetOwners()[1] != 1 || got.GetOwners()[5] != 0 || got.GetViewer().GetRole() != "ruler" ||
		got.GetStates()[0].GetFlag().GetEmblem() != "sun" || got.GetStates()[0].GetLands() != 3 {
		t.Fatalf("GetWorld = %v, %v", got, err)
	}

	if _, err = s.client.ApplyCitizenship(asBob, &worldv1.ApplyCitizenshipRequest{StateId: stateID}); err != nil {
		t.Fatal(err)
	}
	if st, err := s.client.GetState(guest, &worldv1.GetStateRequest{StateId: stateID}); err != nil || len(st.GetApplications()) != 0 || len(st.GetBudget()) != 1 {
		t.Fatalf("GetState by a guest = %v, %v", st, err)
	}
	if st, err := s.client.GetState(asAnn, &worldv1.GetStateRequest{StateId: stateID}); err != nil || len(st.GetApplications()) != 1 || st.GetColonyCost() != 26000 {
		t.Fatalf("GetState by the ruler = %v, %v", st, err)
	}
	if _, err = s.client.AnswerApplication(asBob, &worldv1.AnswerApplicationRequest{UserId: bob.String(), Accept: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("AnswerApplication by the applicant: %v", err)
	}
	if _, err = s.client.AnswerApplication(asAnn, &worldv1.AnswerApplicationRequest{UserId: bob.String(), Accept: true}); err != nil {
		t.Fatal(err)
	}

	// A citizen may not give offices nor change the state.
	if _, err = s.client.AppointCitizen(asBob, &worldv1.AppointCitizenRequest{UserId: ann.String(), Role: "general"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AppointCitizen by a citizen: %v", err)
	}
	if _, err = s.client.EditState(asBob, &worldv1.EditStateRequest{Color: 1, Flag: flag}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("EditState by a citizen: %v", err)
	}
	if _, err = s.client.AppointCitizen(asAnn, &worldv1.AppointCitizenRequest{UserId: bob.String(), Role: "ruler"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("AppointCitizen of a ruler: %v", err)
	}
	if _, err = s.client.AppointCitizen(asAnn, &worldv1.AppointCitizenRequest{UserId: bob.String(), Role: "minister_economy"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.Colonize(asBob, &worldv1.ColonizeRequest{Province: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.Colonize(asBob, &worldv1.ColonizeRequest{Province: 99}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Colonize off the map: %v", err)
	}
	if _, err = s.client.Donate(asBob, &worldv1.DonateRequest{Votes: 0}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Donate of nothing: %v", err)
	}
	if _, err = s.client.Donate(asBob, &worldv1.DonateRequest{Votes: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.EditState(asAnn, &worldv1.EditStateRequest{Motto: "Вместе", Color: 9, Flag: flag}); err != nil {
		t.Fatal(err)
	}

	c, err := s.client.GetCitizenship(guest, &worldv1.GetCitizenshipRequest{UserId: bob.String()})
	if err != nil || c.GetRole() != "minister_economy" || c.GetState().GetMotto() != "Вместе" {
		t.Fatalf("GetCitizenship = %v, %v", c, err)
	}
	if c, err = s.client.GetCitizenship(guest, &worldv1.GetCitizenshipRequest{UserId: uuid.NewString()}); err != nil || c.GetState() != nil {
		t.Fatalf("GetCitizenship of a stranger = %v, %v", c, err)
	}

	if _, err = s.client.LeaveState(asAnn, &emptypb.Empty{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("LeaveState by the ruler: %v", err)
	}
	if _, err = s.client.Abdicate(asAnn, &worldv1.AbdicateRequest{HeirId: bob.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.ExpelCitizen(asBob, &worldv1.ExpelCitizenRequest{UserId: ann.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.WithdrawApplication(asAnn, &emptypb.Empty{}); status.Code(err) != codes.NotFound {
		t.Fatalf("WithdrawApplication of nothing: %v", err)
	}

	events, err := s.client.ListWorldEvents(guest, &worldv1.ListWorldEventsRequest{StateId: stateID})
	if err != nil || len(events.GetEvents()) != 5 || events.GetEvents()[0].GetKind() != "expelled" {
		t.Fatalf("ListWorldEvents = %v, %v", events, err)
	}
	if _, err = s.client.ListWorldEvents(guest, &worldv1.ListWorldEventsRequest{Limit: 51}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListWorldEvents of too many: %v", err)
	}

	if _, err = s.client.PurgeUser(guest, &worldv1.PurgeUserRequest{UserId: bob.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetState(guest, &worldv1.GetStateRequest{StateId: stateID}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetState of a dissolved state: %v", err)
	}
	if _, err = s.client.EndWorld(guest, &emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
}
