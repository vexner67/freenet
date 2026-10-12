// Package s3 keeps media in an S3 bucket, behind the same ports as fs: the
// files are not on the node, so any number of replicas serve them.
package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vexner67/freenet/pkg/errs"
)

const contentType = "image/jpeg"

var ErrNoBucket = errors.New("s3 bucket does not exist")

type Config struct {
	// Endpoint is host[:port], without a scheme.
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// Prefix goes before every key: a bucket shared with something else.
	Prefix string
	// Insecure talks plain HTTP, as to a MinIO next to the service.
	Insecure bool
}

// Bucket is the bucket of the media and the few things done with its
// objects. Keys are built from UUIDs and fixed names only.
type Bucket struct {
	client *minio.Client
	name   string
	prefix string
}

// Open returns ErrNoBucket for a bucket that is not there: it is made by
// whoever runs the storage, with its lifetime and access rules.
func Open(ctx context.Context, cfg Config) (*Bucket, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: !cfg.Insecure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, errs.Wrap("minio.New", err)
	}

	ok, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, errs.Wrap("client.BucketExists", err)
	}
	if !ok {
		return nil, ErrNoBucket
	}

	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}

	return &Bucket{client: client, name: cfg.Bucket, prefix: prefix}, nil
}

func (b *Bucket) put(ctx context.Context, key string, data []byte) error {
	_, err := b.client.PutObject(ctx, b.name, b.prefix+key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})

	return errs.Wrap("b.client.PutObject", err)
}

// putNew creates an empty object unless the key is taken, telling whether
// it did: of writers at once one wins (a conditional write, If-None-Match).
func (b *Bucket) putNew(ctx context.Context, key string) (bool, error) {
	opts := minio.PutObjectOptions{}
	opts.SetMatchETagExcept("*")

	_, err := b.client.PutObject(ctx, b.name, b.prefix+key, bytes.NewReader(nil), 0, opts)
	if is(err, http.StatusPreconditionFailed, http.StatusConflict) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("b.client.PutObject", err)
	}

	return true, nil
}

// get tells whether the object is there.
func (b *Bucket) get(ctx context.Context, key string) ([]byte, bool, error) {
	obj, err := b.client.GetObject(ctx, b.name, b.prefix+key, minio.GetObjectOptions{})
	if err != nil {
		return nil, false, errs.Wrap("b.client.GetObject", err)
	}
	defer func() { _ = obj.Close() }() // nothing was written

	// The request is made by the first read.
	data, err := io.ReadAll(obj)
	if is(err, http.StatusNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errs.Wrap("io.ReadAll", err)
	}

	return data, true, nil
}

func (b *Bucket) has(ctx context.Context, key string) (bool, error) {
	_, err := b.client.StatObject(ctx, b.name, b.prefix+key, minio.StatObjectOptions{})
	if is(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("b.client.StatObject", err)
	}

	return true, nil
}

// copy tells whether there was an object to copy; the copy is made by the
// storage, the bytes do not pass through the service.
func (b *Bucket) copy(ctx context.Context, from, to string) (bool, error) {
	_, err := b.client.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: b.name, Object: b.prefix + to},
		minio.CopySrcOptions{Bucket: b.name, Object: b.prefix + from},
	)
	if is(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("b.client.CopyObject", err)
	}

	return true, nil
}

// remove is fine with objects that are not there.
func (b *Bucket) remove(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := b.client.RemoveObject(ctx, b.name, b.prefix+key, minio.RemoveObjectOptions{}); err != nil {
			return errs.Wrap("b.client.RemoveObject", err)
		}
	}

	return nil
}

type object struct {
	key      string
	modified time.Time
}

// list are the objects whose keys begin with prefix, in the order of keys.
func (b *Bucket) list(ctx context.Context, prefix string) ([]object, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops the listing when an error ends the loop early

	var out []object

	for o := range b.client.ListObjects(ctx, b.name, minio.ListObjectsOptions{Prefix: b.prefix + prefix, Recursive: true}) {
		if o.Err != nil {
			return nil, errs.Wrap("b.client.ListObjects", o.Err)
		}

		out = append(out, object{key: strings.TrimPrefix(o.Key, b.prefix), modified: o.LastModified})
	}

	return out, nil
}

// is tells whether the storage answered err with one of the HTTP statuses.
func is(err error, statuses ...int) bool {
	if err == nil {
		return false
	}

	got := minio.ToErrorResponse(err).StatusCode
	for _, s := range statuses {
		if got == s {
			return true
		}
	}

	return false
}
