package grpc_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	mediagrpc "github.com/vexner67/freenet/media/internal/grpc"
	"github.com/vexner67/freenet/media/internal/inmem"
	"github.com/vexner67/freenet/media/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const issuer = "freenet-test"

type testServer struct {
	client mediav1.MediaServiceClient
	key    *rsa.PrivateKey
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
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		mediagrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer), mediagrpc.PublicMethods()...),
	))
	mediav1.RegisterMediaServiceServer(srv, mediagrpc.NewMediaHandler(
		service.NewAvatarService(inmem.NewAvatarStore()),
		service.NewPhotoService(inmem.NewPhotoStore()),
		log,
	))

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

	return testServer{client: mediav1.NewMediaServiceClient(conn), key: key}
}

// as returns a context of the given user with a signed access token.
func (s testServer) as(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(), Issuer: issuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}

	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestAvatars(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	client := s.client
	userID := uuid.New()
	me := s.as(t, userID)
	var err error

	var picture bytes.Buffer
	if err = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 120, 80))); err != nil {
		t.Fatal(err)
	}

	if _, err = client.UploadAvatar(context.Background(), &mediav1.UploadAvatarRequest{Image: picture.Bytes()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("upload without token: %v", err)
	}
	if _, err = client.UploadAvatar(me, &mediav1.UploadAvatarRequest{Image: []byte("text")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("not an image: %v", err)
	}

	first, err := client.UploadAvatar(me, &mediav1.UploadAvatarRequest{Image: picture.Bytes()})
	if err != nil {
		t.Fatalf("UploadAvatar: %v", err)
	}
	second, err := client.UploadAvatar(me, &mediav1.UploadAvatarRequest{Image: picture.Bytes()})
	if err != nil {
		t.Fatal(err)
	}

	get := func(avatarID string, size mediav1.AvatarSize) (*mediav1.GetAvatarResponse, error) {
		return client.GetAvatar(context.Background(), &mediav1.GetAvatarRequest{UserId: userID.String(), AvatarId: avatarID, Size: size})
	}

	small, err := get(second.GetAvatarId(), mediav1.AvatarSize_AVATAR_SIZE_SMALL)
	if err != nil {
		t.Fatalf("GetAvatar without token: %v", err)
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(small.GetImage())); err != nil || format != "jpeg" || cfg.Width != 100 {
		t.Fatalf("small avatar: %v %s %v", cfg, format, err)
	}

	if _, err = client.DeleteAvatars(me, &mediav1.DeleteAvatarsRequest{KeepAvatarId: second.GetAvatarId()}); err != nil {
		t.Fatal(err)
	}
	if _, err = get(first.GetAvatarId(), mediav1.AvatarSize_AVATAR_SIZE_LARGE); status.Code(err) != codes.NotFound {
		t.Fatalf("replaced avatar must be gone: %v", err)
	}
	if _, err = get(second.GetAvatarId(), mediav1.AvatarSize_AVATAR_SIZE_LARGE); err != nil {
		t.Fatalf("kept avatar: %v", err)
	}
	if _, err = get("x", mediav1.AvatarSize_AVATAR_SIZE_LARGE); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad id: %v", err)
	}
}

func TestPhotos(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 1600, 900))); err != nil {
		t.Fatal(err)
	}

	if _, err := s.client.UploadPhoto(context.Background(), &mediav1.UploadPhotoRequest{Image: picture.Bytes()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("upload without token: %v", err)
	}
	if _, err := s.client.UploadPhoto(s.as(t, ann), &mediav1.UploadPhotoRequest{Image: []byte("text")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("not an image: %v", err)
	}

	var ids []string
	for range 2 {
		resp, err := s.client.UploadPhoto(s.as(t, ann), &mediav1.UploadPhotoRequest{Image: picture.Bytes()})
		if err != nil {
			t.Fatalf("UploadPhoto: %v", err)
		}
		ids = append(ids, resp.GetPhotoId())
	}

	small, err := s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: ids[0], Size: mediav1.PhotoSize_PHOTO_SIZE_SMALL})
	if err != nil {
		t.Fatalf("GetPhoto without token: %v", err)
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(small.GetImage())); err != nil || format != "jpeg" || cfg.Width != 640 || cfg.Height != 360 {
		t.Fatalf("small photo: %v %s %v; a photo keeps its proportions", cfg, format, err)
	}

	// The same IDs under bob are not his.
	stranger := uuid.NewString()
	own, err := s.client.ListOwnPhotos(s.as(t, ann), &mediav1.ListOwnPhotosRequest{PhotoIds: append([]string{stranger}, ids...)})
	if err != nil || len(own.GetPhotoIds()) != 2 {
		t.Fatalf("ann's photos = %v, %v", own, err)
	}
	if bobs, err := s.client.ListOwnPhotos(s.as(t, bob), &mediav1.ListOwnPhotosRequest{PhotoIds: ids}); err != nil || len(bobs.GetPhotoIds()) != 0 {
		t.Fatalf("bob has none of ann's photos: %v, %v", bobs, err)
	}

	// Bob cannot delete ann's photos: he deletes only his own.
	if _, err = s.client.DeletePhotos(s.as(t, bob), &mediav1.DeletePhotosRequest{PhotoIds: ids}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: ids[0]}); err != nil {
		t.Fatalf("ann's photo after bob's delete: %v", err)
	}

	if _, err = s.client.DeletePhotos(s.as(t, ann), &mediav1.DeletePhotosRequest{PhotoIds: ids[:1]}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: ids[0]}); status.Code(err) != codes.NotFound {
		t.Fatalf("deleted photo: %v", err)
	}

	if _, err = s.client.ListOwnPhotos(s.as(t, ann), &mediav1.ListOwnPhotosRequest{PhotoIds: make([]string, 11)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("too many ids: %v", err)
	}
}
