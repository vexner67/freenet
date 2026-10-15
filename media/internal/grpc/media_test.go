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
	avatars, photos, documents := inmem.NewAvatarStore(), inmem.NewPhotoStore(), inmem.NewDocumentStore()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		mediagrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer), jwtauth.Access{Public: mediagrpc.PublicMethods()}),
	))
	mediav1.RegisterMediaServiceServer(srv, mediagrpc.NewMediaHandler(mediagrpc.Services{
		Avatars:   service.NewAvatarService(avatars),
		Photos:    service.NewPhotoService(photos, time.Hour, 24*time.Hour, 0),
		Documents: service.NewDocumentService(documents),
		Purges:    service.NewPurgeService(avatars, photos, documents),
	}, log))

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
	if got, err := s.client.ClaimPhotos(s.as(t, ann), &mediav1.ClaimPhotosRequest{PhotoIds: append(ids[:2:2], stranger)}); err != nil || got.GetClaimed() {
		t.Fatalf("a claim with a photo that is not there = %v, %v", got, err)
	}
	if bobs, err := s.client.ClaimPhotos(s.as(t, bob), &mediav1.ClaimPhotosRequest{PhotoIds: ids}); err != nil || bobs.GetClaimed() {
		t.Fatalf("bob claimed ann's photos: %v, %v", bobs, err)
	}
	if _, err = s.client.ClaimPhotos(context.Background(), &mediav1.ClaimPhotosRequest{PhotoIds: ids}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("a claim without a token: %v", err)
	}

	// A photo goes to one post: the second claim is refused, until the
	// first post was and gave it back.
	for _, want := range []bool{true, false} {
		if got, err := s.client.ClaimPhotos(s.as(t, ann), &mediav1.ClaimPhotosRequest{PhotoIds: ids}); err != nil || got.GetClaimed() != want {
			t.Fatalf("ann's claim = %v, %v; want %v", got, err, want)
		}
	}
	if _, err = s.client.ReleasePhotos(s.as(t, ann), &mediav1.ReleasePhotosRequest{PhotoIds: ids}); err != nil {
		t.Fatalf("ReleasePhotos: %v", err)
	}
	if got, err := s.client.ClaimPhotos(s.as(t, ann), &mediav1.ClaimPhotosRequest{PhotoIds: ids}); err != nil || !got.GetClaimed() {
		t.Fatalf("a claim after a release = %v, %v", got, err)
	}

	// post publishes and deletes the photos of posts, with no user token,
	// for the owner it names: bob's name does not reach ann's photos.
	if _, err = s.client.PublishPhotos(context.Background(), &mediav1.PublishPhotosRequest{OwnerId: ann.String(), PhotoIds: ids}); err != nil {
		t.Fatalf("PublishPhotos: %v", err)
	}
	if _, err = s.client.DeletePublishedPhotos(context.Background(), &mediav1.DeletePublishedPhotosRequest{OwnerId: bob.String(), PhotoIds: ids}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: ids[0]}); err != nil {
		t.Fatalf("ann's published photo after bob's delete: %v", err)
	}

	// post keeps a photo of a post once more, for "Фотографии со стены".
	copied := uuid.NewString()
	copies := &mediav1.CopyPhotosRequest{OwnerId: ann.String(), Copies: []*mediav1.PhotoCopy{{FromId: ids[0], ToId: copied}}}
	if _, err = s.client.CopyPhotos(context.Background(), copies); err != nil {
		t.Fatalf("CopyPhotos: %v", err)
	}
	if _, err = s.client.CopyPhotos(context.Background(), &mediav1.CopyPhotosRequest{OwnerId: ann.String(), Copies: []*mediav1.PhotoCopy{{FromId: ids[0], ToId: "x"}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CopyPhotos to a bad id: %v", err)
	}

	if _, err = s.client.DeletePublishedPhotos(context.Background(), &mediav1.DeletePublishedPhotosRequest{OwnerId: ann.String(), PhotoIds: ids[:1]}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: copied}); err != nil {
		t.Fatalf("the copy after the photo is deleted: %v", err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: ids[0]}); status.Code(err) != codes.NotFound {
		t.Fatalf("deleted photo: %v", err)
	}
	if _, err = s.client.PublishPhotos(context.Background(), &mediav1.PublishPhotosRequest{OwnerId: "ann", PhotoIds: ids}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PublishPhotos of a bad owner: %v", err)
	}

	// Published, a photo is its post's for good.
	if got, err := s.client.ClaimPhotos(s.as(t, ann), &mediav1.ClaimPhotosRequest{PhotoIds: ids[1:]}); err != nil || got.GetClaimed() {
		t.Fatalf("a claim of a published photo = %v, %v", got, err)
	}

	if _, err = s.client.ClaimPhotos(s.as(t, ann), &mediav1.ClaimPhotosRequest{PhotoIds: make([]string, 11)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("too many ids: %v", err)
	}
	if _, err = s.client.ReleasePhotos(s.as(t, ann), &mediav1.ReleasePhotosRequest{PhotoIds: []string{"x"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a release of a bad id: %v", err)
	}
}

func TestPurgeUser(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 120, 80))); err != nil {
		t.Fatal(err)
	}

	avatar, err := s.client.UploadAvatar(s.as(t, ann), &mediav1.UploadAvatarRequest{Image: picture.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	annsPhoto, err := s.client.UploadPhoto(s.as(t, ann), &mediav1.UploadPhotoRequest{Image: picture.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	bobsPhoto, err := s.client.UploadPhoto(s.as(t, bob), &mediav1.UploadPhotoRequest{Image: picture.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	annsDocument, err := s.client.UploadDocument(s.as(t, ann), &mediav1.UploadDocumentRequest{Data: []byte("%PDF")})
	if err != nil {
		t.Fatal(err)
	}

	// Called by the user service, with no user token; twice is fine.
	for range 2 {
		if _, err = s.client.PurgeUser(context.Background(), &mediav1.PurgeUserRequest{UserId: ann.String()}); err != nil {
			t.Fatalf("PurgeUser: %v", err)
		}
	}
	if _, err = s.client.PurgeUser(context.Background(), &mediav1.PurgeUserRequest{UserId: "nobody"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("PurgeUser of a bad ID: %v", err)
	}

	if _, err = s.client.GetAvatar(context.Background(), &mediav1.GetAvatarRequest{UserId: ann.String(), AvatarId: avatar.GetAvatarId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("ann's avatar after the purge: %v", err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: ann.String(), PhotoId: annsPhoto.GetPhotoId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("ann's photo after the purge: %v", err)
	}
	if _, err = s.client.GetDocument(context.Background(), &mediav1.GetDocumentRequest{OwnerId: ann.String(), DocumentId: annsDocument.GetDocumentId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("ann's document after the purge: %v", err)
	}
	if _, err = s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: bob.String(), PhotoId: bobsPhoto.GetPhotoId()}); err != nil {
		t.Fatalf("bob's photo after ann's purge: %v", err)
	}
}

func TestClubAvatars(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	club, admin := uuid.New(), uuid.New()

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 120, 80))); err != nil {
		t.Fatal(err)
	}

	first, err := s.client.UploadClubAvatar(s.as(t, admin), &mediav1.UploadClubAvatarRequest{ClubId: club.String(), Image: picture.Bytes()})
	if err != nil {
		t.Fatalf("UploadClubAvatar: %v", err)
	}
	second, err := s.client.UploadClubAvatar(s.as(t, admin), &mediav1.UploadClubAvatarRequest{ClubId: club.String(), Image: picture.Bytes()})
	if err != nil {
		t.Fatal(err)
	}

	// Served under the club's ID, as a user's avatar.
	get := func(id string) error {
		_, err := s.client.GetAvatar(context.Background(), &mediav1.GetAvatarRequest{UserId: club.String(), AvatarId: id, Size: mediav1.AvatarSize_AVATAR_SIZE_LARGE})
		return err
	}
	if err = get(first.GetAvatarId()); err != nil {
		t.Fatalf("GetAvatar: %v", err)
	}

	if _, err = s.client.DeleteClubAvatars(s.as(t, admin), &mediav1.DeleteClubAvatarsRequest{ClubId: club.String(), KeepAvatarId: second.GetAvatarId()}); err != nil {
		t.Fatalf("DeleteClubAvatars: %v", err)
	}
	if err = get(first.GetAvatarId()); status.Code(err) != codes.NotFound {
		t.Fatalf("the old avatar: %v", err)
	}
	if err = get(second.GetAvatarId()); err != nil {
		t.Fatalf("the kept avatar: %v", err)
	}

	if _, err = s.client.UploadClubAvatar(s.as(t, admin), &mediav1.UploadClubAvatarRequest{ClubId: "x", Image: picture.Bytes()}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad club id: %v", err)
	}
}

func TestChatAvatars(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	chat, member := uuid.New(), uuid.New()

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 80, 80))); err != nil {
		t.Fatal(err)
	}

	first, err := s.client.UploadChatAvatar(s.as(t, member), &mediav1.UploadChatAvatarRequest{ChatId: chat.String(), Image: picture.Bytes()})
	if err != nil {
		t.Fatalf("UploadChatAvatar: %v", err)
	}

	get := func(id string) error {
		_, err := s.client.GetAvatar(context.Background(), &mediav1.GetAvatarRequest{UserId: chat.String(), AvatarId: id, Size: mediav1.AvatarSize_AVATAR_SIZE_SMALL})
		return err
	}
	if err = get(first.GetAvatarId()); err != nil {
		t.Fatalf("GetAvatar: %v", err)
	}

	// The photo removed: nothing is kept.
	if _, err = s.client.DeleteChatAvatars(s.as(t, member), &mediav1.DeleteChatAvatarsRequest{ChatId: chat.String()}); err != nil {
		t.Fatalf("DeleteChatAvatars: %v", err)
	}
	if err = get(first.GetAvatarId()); status.Code(err) != codes.NotFound {
		t.Fatalf("the removed avatar: %v", err)
	}

	if _, err = s.client.UploadChatAvatar(s.as(t, member), &mediav1.UploadChatAvatarRequest{ChatId: "x", Image: picture.Bytes()}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad chat id: %v", err)
	}
}

func TestClubPhotos(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	club, admin := uuid.New(), uuid.New()

	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 120, 300))); err != nil {
		t.Fatal(err)
	}

	left, err := s.client.UploadClubPhoto(s.as(t, admin), &mediav1.UploadClubPhotoRequest{ClubId: club.String(), Image: picture.Bytes()})
	if err != nil {
		t.Fatalf("UploadClubPhoto: %v", err)
	}
	right, err := s.client.UploadClubPhoto(s.as(t, admin), &mediav1.UploadClubPhotoRequest{ClubId: club.String(), Image: picture.Bytes()})
	if err != nil {
		t.Fatal(err)
	}

	// Served under the club's ID as a photo, and kept: no post publishes it.
	get := func(id string) error {
		_, err := s.client.GetPhoto(context.Background(), &mediav1.GetPhotoRequest{UserId: club.String(), PhotoId: id, Size: mediav1.PhotoSize_PHOTO_SIZE_LARGE})
		return err
	}
	if err = get(left.GetPhotoId()); err != nil {
		t.Fatalf("GetPhoto: %v", err)
	}

	if _, err = s.client.DeleteClubPhotos(s.as(t, admin), &mediav1.DeleteClubPhotosRequest{ClubId: club.String(), KeepPhotoIds: []string{right.GetPhotoId()}}); err != nil {
		t.Fatalf("DeleteClubPhotos: %v", err)
	}
	if err = get(left.GetPhotoId()); status.Code(err) != codes.NotFound {
		t.Fatalf("the dropped photo: %v", err)
	}
	if err = get(right.GetPhotoId()); err != nil {
		t.Fatalf("the kept photo: %v", err)
	}

	if _, err = s.client.DeleteClubPhotos(s.as(t, admin), &mediav1.DeleteClubPhotosRequest{ClubId: club.String(), KeepPhotoIds: []string{"x"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad photo id: %v", err)
	}
}

func TestDocuments(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()

	if _, err := s.client.UploadDocument(context.Background(), &mediav1.UploadDocumentRequest{Data: []byte("x")}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("upload without a token: %v", err)
	}
	if _, err := s.client.UploadDocument(s.as(t, ann), &mediav1.UploadDocumentRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an empty document: %v", err)
	}

	uploaded, err := s.client.UploadDocument(s.as(t, ann), &mediav1.UploadDocumentRequest{Data: []byte("привет")})
	if err != nil {
		t.Fatal(err)
	}

	// Anyone downloads it by its address.
	got, err := s.client.GetDocument(context.Background(), &mediav1.GetDocumentRequest{OwnerId: ann.String(), DocumentId: uploaded.GetDocumentId()})
	if err != nil || string(got.GetData()) != "привет" {
		t.Fatalf("GetDocument = %q, %v", got.GetData(), err)
	}
	if _, err = s.client.GetDocument(context.Background(), &mediav1.GetDocumentRequest{OwnerId: uuid.NewString(), DocumentId: uploaded.GetDocumentId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("under another owner: %v", err)
	}

	// Deleted by the post service, twice is fine.
	for range 2 {
		if _, err = s.client.DeleteDocuments(context.Background(), &mediav1.DeleteDocumentsRequest{OwnerId: ann.String(), DocumentIds: []string{uploaded.GetDocumentId()}}); err != nil {
			t.Fatalf("DeleteDocuments: %v", err)
		}
	}
	if _, err = s.client.GetDocument(context.Background(), &mediav1.GetDocumentRequest{OwnerId: ann.String(), DocumentId: uploaded.GetDocumentId()}); status.Code(err) != codes.NotFound {
		t.Fatalf("a deleted document: %v", err)
	}
}
