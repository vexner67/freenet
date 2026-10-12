package s3

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

// The places of a photo, as the directories of fs: published for good,
// uploaded and waiting for its post, set aside as waiting too long.
const (
	published = "photos"
	uploads   = "uploads"
	retired   = "retired"
)

// claimName marks an upload as claimed, next to its images. It is written
// only if not there, so of two claims one wins.
const claimName = "claimed"

// sizes are written, copied and moved in this order and removed in the
// reverse one: the large image is there only while the photo is whole.
var sizes = []photo.Size{photo.Small, photo.Large}

// PhotoStore lays objects out as <place>/<owner>/<photo>/<size>.jpg. An
// object cannot be renamed: a photo is moved by copying its images and
// removing them from where they were, so a reader finds it in one place
// or in both, never in neither.
type PhotoStore struct {
	bucket *Bucket
}

func NewPhotoStore(bucket *Bucket) *PhotoStore {
	return &PhotoStore{bucket: bucket}
}

func photoDir(place string, ownerID, photoID uuid.UUID) string {
	return place + "/" + ownerID.String() + "/" + photoID.String() + "/"
}

func imageName(size photo.Size) string {
	return string(size) + ".jpg"
}

func (s *PhotoStore) Save(ctx context.Context, ownerID, photoID uuid.UUID, images photo.Images) error {
	dir := photoDir(uploads, ownerID, photoID)

	for _, size := range sizes {
		if err := s.bucket.put(ctx, dir+imageName(size), images.Of(size)); err != nil {
			return err
		}
	}

	return nil
}

func (s *PhotoStore) Load(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	if size != photo.Large && size != photo.Small {
		return nil, photo.ErrNotFound
	}

	// Published first: being published copies the photo there before it
	// leaves the uploads.
	for _, place := range []string{published, uploads} {
		data, ok, err := s.bucket.get(ctx, photoDir(place, ownerID, photoID)+imageName(size))
		if err != nil {
			return nil, err
		}
		if ok {
			return data, nil
		}
	}

	return nil, photo.ErrNotFound
}

// Claim leaves its mark where a published photo is no more, so the mark
// of a photo published meanwhile claims nothing: the upload is looked for
// again after it.
func (s *PhotoStore) Claim(ctx context.Context, ownerID, photoID uuid.UUID) (bool, error) {
	dir := photoDir(uploads, ownerID, photoID)

	if waiting, err := s.bucket.has(ctx, dir+imageName(photo.Large)); err != nil || !waiting {
		return false, err
	}

	if marked, err := s.bucket.putNew(ctx, dir+claimName); err != nil || !marked {
		return false, err
	}

	return s.bucket.has(ctx, dir+imageName(photo.Large))
}

func (s *PhotoStore) Release(ctx context.Context, ownerID, photoID uuid.UUID) error {
	return s.bucket.remove(ctx, photoDir(uploads, ownerID, photoID)+claimName)
}

// Publish keeps the mark of a claim in the uploads: whoever is claiming
// the photo right now finds it taken. Retire sweeps the marks.
func (s *PhotoStore) Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	for _, id := range photoIDs {
		for _, place := range []string{uploads, retired} {
			moved, err := s.move(ctx, photoDir(place, ownerID, id), photoDir(published, ownerID, id))
			if err != nil {
				return err
			}
			if moved {
				break
			}
		}
	}

	return nil
}

// move copies the images of a photo and removes them from where they
// were, telling whether they were there. Done again after a failure half
// way, it finishes the job.
func (s *PhotoStore) move(ctx context.Context, from, to string) (bool, error) {
	for _, size := range sizes {
		copied, err := s.bucket.copy(ctx, from+imageName(size), to+imageName(size))
		if err != nil || !copied {
			return false, err
		}
	}

	for _, size := range slices.Backward(sizes) {
		if err := s.bucket.remove(ctx, from+imageName(size)); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (s *PhotoStore) Copy(ctx context.Context, ownerID, from, to uuid.UUID) error {
	final := photoDir(published, ownerID, to)

	if copied, err := s.bucket.has(ctx, final+imageName(photo.Large)); err != nil || copied {
		return err
	}

	for _, place := range []string{published, uploads} {
		dir := photoDir(place, ownerID, from)

		found := false
		for _, size := range sizes {
			copied, err := s.bucket.copy(ctx, dir+imageName(size), final+imageName(size))
			if err != nil {
				return err
			}
			if !copied {
				break
			}
			found = true
		}

		if found {
			return nil
		}
	}

	return nil
}

func (s *PhotoStore) Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	for _, id := range photoIDs {
		for _, place := range []string{published, uploads, retired} {
			dir := photoDir(place, ownerID, id)

			if err := s.bucket.remove(ctx, dir+imageName(photo.Large), dir+imageName(photo.Small), dir+claimName); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *PhotoStore) DeleteAll(ctx context.Context, ownerID uuid.UUID) error {
	for _, place := range []string{published, uploads, retired} {
		if err := s.removeUnder(ctx, place+"/"+ownerID.String()+"/", nil); err != nil {
			return err
		}
	}

	return nil
}

func (s *PhotoStore) DeleteExcept(ctx context.Context, ownerID uuid.UUID, keep []uuid.UUID) error {
	return s.removeUnder(ctx, published+"/"+ownerID.String()+"/", keep)
}

// removeUnder removes the objects under prefix, an owner's, but those of
// the photos to keep.
func (s *PhotoStore) removeUnder(ctx context.Context, prefix string, keep []uuid.UUID) error {
	objects, err := s.bucket.list(ctx, prefix)
	if err != nil {
		return err
	}

	for _, o := range objects {
		name, _, _ := strings.Cut(strings.TrimPrefix(o.key, prefix), "/")
		if id, err := uuid.Parse(name); err == nil && slices.Contains(keep, id) {
			continue
		}

		if err = s.bucket.remove(ctx, o.key); err != nil {
			return err
		}
	}

	return nil
}

// Retire moves the uploads older than before, by the time in their
// UUIDv7, to where only a late Publish finds them. The marks of claims go:
// those of photos published long ago, and of uploads set aside.
func (s *PhotoStore) Retire(ctx context.Context, before time.Time) (int, error) {
	return s.sweep(ctx, uploads, before, func(ctx context.Context, dir string) error {
		if _, err := s.move(ctx, dir, retired+strings.TrimPrefix(dir, uploads)); err != nil {
			return err
		}

		// What a save or a move left half way goes too.
		return s.bucket.remove(ctx, dir+imageName(photo.Large), dir+imageName(photo.Small), dir+claimName)
	})
}

func (s *PhotoStore) DeleteRetired(ctx context.Context, before time.Time) (int, error) {
	return s.sweep(ctx, retired, before, func(ctx context.Context, dir string) error {
		return s.bucket.remove(ctx, dir+imageName(photo.Large), dir+imageName(photo.Small))
	})
}

// sweep takes the photos of a place older than before away with take,
// which is given <place>/<owner>/<photo>/, and returns how many whole
// photos went. The whole place is listed: it holds what waits, not what
// is published.
func (s *PhotoStore) sweep(ctx context.Context, place string, before time.Time, take func(ctx context.Context, dir string) error) (int, error) {
	objects, err := s.bucket.list(ctx, place+"/")
	if err != nil {
		return 0, err
	}

	taken, last := 0, ""

	for _, o := range objects {
		if err = ctx.Err(); err != nil {
			return taken, errs.Wrap("ctx.Err", err)
		}

		dir := photoOf(o.key)
		if dir == "" || dir == last || !uploadTime(dir, o.modified).Before(before) {
			continue
		}

		// Keys come sorted: the objects of a photo are next to each other.
		last = dir

		whole, err := s.bucket.has(ctx, dir+imageName(photo.Large))
		if err != nil {
			return taken, err
		}

		if err = take(ctx, dir); err != nil {
			return taken, err
		}

		if whole {
			taken++
		}
	}

	return taken, nil
}

// photoOf is the <place>/<owner>/<photo>/ of an object of a photo; a key
// laid out otherwise, which is not ours, gives "".
func photoOf(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) != 4 {
		return ""
	}

	return strings.Join(parts[:3], "/") + "/"
}

// uploadTime is when the photo at dir was uploaded: the time in its
// UUIDv7, or else when its object was written.
func uploadTime(dir string, modified time.Time) time.Time {
	parts := strings.Split(dir, "/")

	if id, err := uuid.Parse(parts[2]); err == nil && id.Version() == 7 {
		return time.Unix(id.Time().UnixTime())
	}

	return modified
}
