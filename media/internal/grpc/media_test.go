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

func TestAvatars(t *testing.T) {
	t.Parallel()

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
	mediav1.RegisterMediaServiceServer(srv, mediagrpc.NewMediaHandler(service.NewAvatarService(inmem.NewAvatarStore()), log))

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

	client := mediav1.NewMediaServiceClient(conn)
	userID := uuid.New()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(), Issuer: issuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	me := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)

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
