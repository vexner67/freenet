package fs_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vexner67/freenet/media/internal/s3"
)

const testBucket = "freenet-media-test"

// bucket is a part of a bucket of its own for a test, at the storage
// TEST_S3_ENDPOINT names (plain HTTP, with TEST_S3_ACCESS_KEY and
// TEST_S3_SECRET_KEY), or nil without it: the stores kept in a bucket are
// then not checked. scripts/check.sh --integration runs a MinIO for it.
func bucket(t *testing.T) *s3.Bucket {
	t.Helper()

	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		return nil
	}

	access, secret := os.Getenv("TEST_S3_ACCESS_KEY"), os.Getenv("TEST_S3_SECRET_KEY")
	ctx := context.Background()

	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, "")})
	if err != nil {
		t.Fatal(err)
	}
	// Another test may have made it first.
	if err = client.MakeBucket(ctx, testBucket, minio.MakeBucketOptions{}); err != nil {
		if ok, existsErr := client.BucketExists(ctx, testBucket); existsErr != nil || !ok {
			t.Fatalf("MakeBucket: %v", err)
		}
	}

	// A prefix of its own: Retire sweeps all the uploads it sees.
	b, err := s3.Open(ctx, s3.Config{
		Endpoint: endpoint, Bucket: testBucket, AccessKey: access, SecretKey: secret,
		Prefix: uuid.NewString(), Insecure: true,
	})
	if err != nil {
		t.Fatalf("s3.Open: %v", err)
	}

	return b
}

func TestBucket_Missing(t *testing.T) {
	t.Parallel()

	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is not set")
	}

	_, err := s3.Open(context.Background(), s3.Config{
		Endpoint: endpoint, Bucket: "freenet-no-such-bucket", Insecure: true,
		AccessKey: os.Getenv("TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TEST_S3_SECRET_KEY"),
	})
	if err == nil {
		t.Fatal("a bucket that is not there was opened")
	}
}
