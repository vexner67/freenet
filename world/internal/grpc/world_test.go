package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/vexner67/freenet/pkg/jwtauth"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	worldgrpc "github.com/vexner67/freenet/world/internal/grpc"
	"github.com/vexner67/freenet/world/internal/inmem"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
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

type testServer struct {
	client worldv1.WorldServiceClient
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
	worlds := service.NewWorldService(service.WorldDeps{Worlds: store, Maps: store, Maker: lineMaker{}, Tx: inmem.TxManager{}})
	worldv1.RegisterWorldServiceServer(srv, worldgrpc.NewWorldHandler(worlds))

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

	return testServer{client: worldv1.NewWorldServiceClient(conn), worlds: worlds}
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

func TestOpenWorld(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	id := s.open(t)
	guest := context.Background()

	if a, err := s.client.GetAtlas(guest, &worldv1.GetAtlasRequest{WorldId: id}); err != nil || len(a.GetProvinces()) != 10 || a.GetProvinces()[1].GetLinks()[0].GetTo() != 0 || a.GetZooms() != 1 {
		t.Fatalf("GetAtlas = %v, %v", a, err)
	}
	if r, err := s.client.GetRaster(guest, &worldv1.GetRasterRequest{WorldId: id}); err != nil || len(r.GetRaster()) != 2 {
		t.Fatalf("GetRaster = %v, %v", r, err)
	}
	if tile, err := s.client.GetTile(guest, &worldv1.GetTileRequest{WorldId: id}); err != nil || len(tile.GetPng()) != 1 {
		t.Fatalf("GetTile = %v, %v", tile, err)
	}

	got, err := s.client.GetWorld(guest, &worldv1.GetWorldRequest{})
	if err != nil || got.GetWorld().GetId() != id || got.GetWorld().GetStatus() != worldv1.WorldStatus_WORLD_STATUS_OPEN {
		t.Fatalf("GetWorld = %v, %v", got, err)
	}
	if _, err = s.client.EndWorld(guest, &emptypb.Empty{}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetWorld(guest, &worldv1.GetWorldRequest{}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetWorld after the end: %v", err)
	}
}
