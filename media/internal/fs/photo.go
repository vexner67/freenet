package fs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/media/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// PhotoStore lays files out as <root>/photos/<owner>/<photo>/<size>.jpg,
// and photos not published yet the same way under <root>/uploads, so that
// finding the unpublished ones does not walk every photo. Uploads that
// waited too long are set aside under <root>/retired, where only a late
// Publish finds them. Paths are built from UUIDs and fixed names only,
// never from user input.
type PhotoStore struct {
	published string
	uploads   string
	retired   string
}

func NewPhotoStore(root string) (*PhotoStore, error) {
	s := &PhotoStore{
		published: filepath.Join(root, "photos"),
		uploads:   filepath.Join(root, "uploads"),
		retired:   filepath.Join(root, "retired"),
	}

	for _, dir := range []string{s.published, s.uploads, s.retired} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, errs.Wrap("os.MkdirAll", err)
		}
	}

	return s, nil
}

// dirs are where a photo may be: published or not.
func (s *PhotoStore) dirs(ownerID, photoID uuid.UUID) [2]string {
	return [2]string{
		filepath.Join(s.published, ownerID.String(), photoID.String()),
		filepath.Join(s.uploads, ownerID.String(), photoID.String()),
	}
}

func (s *PhotoStore) Save(_ context.Context, ownerID, photoID uuid.UUID, images photo.Images) error {
	return saveDir(s.dirs(ownerID, photoID)[1], map[string][]byte{
		string(photo.Large) + ".jpg": images.Of(photo.Large),
		string(photo.Small) + ".jpg": images.Of(photo.Small),
	})
}

func (s *PhotoStore) Load(_ context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	if size != photo.Large && size != photo.Small {
		return nil, photo.ErrNotFound
	}

	// Published first: being published moves the photo there.
	for _, dir := range s.dirs(ownerID, photoID) {
		data, err := os.ReadFile(filepath.Join(dir, string(size)+".jpg"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap("os.ReadFile", err)
		}

		return data, nil
	}

	return nil, photo.ErrNotFound
}

// claimFile marks an upload as claimed. It is created exclusively, so of
// two claims one wins, and travels with the directory of the photo. A
// claim taken over leaves a file of its own next to it, claimed.<number>,
// created exclusively too: the upload is claimed while claimFile is
// there, and the highest number tells whose claim it is.
const claimFile = "claimed"

// takeover is the file of the claim with this number.
func takeover(number int) string {
	if number == 0 {
		return claimFile
	}

	return claimFile + "." + strconv.Itoa(number)
}

// claimNumber reads the number of a claim from the name of its file.
func claimNumber(name string) (int, bool) {
	if name == claimFile {
		return 0, true
	}

	suffix, ok := strings.CutPrefix(name, claimFile+".")
	if !ok {
		return 0, false
	}

	n, err := strconv.Atoi(suffix)

	return n, err == nil && n > 0
}

func (s *PhotoStore) Claim(_ context.Context, ownerID, photoID uuid.UUID) (bool, error) {
	f, err := os.OpenFile(filepath.Join(s.dirs(ownerID, photoID)[1], claimFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	// No upload: no such photo, or it is published. There: claimed.
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("os.OpenFile", err)
	}

	return true, errs.Wrap("f.Close", f.Close())
}

func (s *PhotoStore) Claimed(_ context.Context, ownerID, photoID uuid.UUID) (service.PhotoClaim, bool, error) {
	dir := s.dirs(ownerID, photoID)[1]

	numbers, err := claims(dir)
	if err != nil || len(numbers) == 0 || numbers[0] != 0 {
		return service.PhotoClaim{}, false, err
	}

	last := numbers[len(numbers)-1]

	info, err := os.Stat(filepath.Join(dir, takeover(last)))
	// Released this instant.
	if errors.Is(err, fs.ErrNotExist) {
		return service.PhotoClaim{}, false, nil
	}
	if err != nil {
		return service.PhotoClaim{}, false, errs.Wrap("os.Stat", err)
	}

	return service.PhotoClaim{At: info.ModTime(), Number: last}, true, nil
}

// claims are the numbers of the claim files of an upload, ascending; none
// when the upload is not there.
func claims(dir string) ([]int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errs.Wrap("os.ReadDir", err)
	}

	var numbers []int
	for _, e := range entries {
		if n, ok := claimNumber(e.Name()); ok {
			numbers = append(numbers, n)
		}
	}
	slices.Sort(numbers)

	return numbers, nil
}

func (s *PhotoStore) Reclaim(_ context.Context, ownerID, photoID uuid.UUID, number int) (bool, error) {
	dir := s.dirs(ownerID, photoID)[1]
	mine := filepath.Join(dir, takeover(number+1))

	f, err := os.OpenFile(mine, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	// No upload: it is published. There: someone took the claim over first.
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("os.OpenFile", err)
	}
	if err = f.Close(); err != nil {
		return false, errs.Wrap("f.Close", err)
	}

	// Released meanwhile: there was no claim left to take over.
	held, err := exists(filepath.Join(dir, claimFile))
	if err != nil || held {
		return held, err
	}

	if err = os.Remove(mine); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, errs.Wrap("os.Remove", err)
	}

	return false, nil
}

// Release removes claimFile first: from then on the upload is not
// claimed, whatever files of claims taken over are still to go.
func (s *PhotoStore) Release(_ context.Context, ownerID, photoID uuid.UUID) error {
	dir := s.dirs(ownerID, photoID)[1]

	numbers, err := claims(dir)
	if err != nil {
		return err
	}

	for _, n := range numbers {
		if err = os.Remove(filepath.Join(dir, takeover(n))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errs.Wrap("os.Remove", err)
		}
	}

	return nil
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("os.Stat", err)
	}

	return true, nil
}

// set is where a photo set aside is.
func (s *PhotoStore) set(ownerID, photoID uuid.UUID) string {
	return filepath.Join(s.retired, ownerID.String(), photoID.String())
}

// Publish moves the photos out of the uploads, or back from where they
// were set aside; a rename, so a reader finds a photo in one place or the
// other.
func (s *PhotoStore) Publish(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	if err := os.MkdirAll(filepath.Join(s.published, ownerID.String()), 0o750); err != nil {
		return errs.Wrap("os.MkdirAll", err)
	}

	for _, id := range photoIDs {
		dirs := s.dirs(ownerID, id)

		for _, from := range []string{dirs[1], s.set(ownerID, id)} {
			err := os.Rename(from, dirs[0])
			if err == nil {
				break
			}
			// Missing everywhere: published already, or deleted.
			if !errors.Is(err, fs.ErrNotExist) {
				return errs.Wrap("os.Rename", err)
			}
		}
	}

	return nil
}

// Copy links the files of from into a new published photo: the images
// never change, so both photos share them on disk until one is deleted.
func (s *PhotoStore) Copy(_ context.Context, ownerID, from, to uuid.UUID) error {
	final := s.dirs(ownerID, to)[0]
	if ok, err := exists(final); err != nil || ok {
		return err
	}

	for _, dir := range s.dirs(ownerID, from) {
		err := linkDir(dir, final, string(photo.Large)+".jpg", string(photo.Small)+".jpg")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		return err
	}

	return nil
}

func (s *PhotoStore) Delete(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	for _, id := range photoIDs {
		dirs := s.dirs(ownerID, id)

		for _, dir := range []string{dirs[0], dirs[1], s.set(ownerID, id)} {
			if err := os.RemoveAll(dir); err != nil {
				return errs.Wrap("os.RemoveAll", err)
			}
		}
	}

	return nil
}

func (s *PhotoStore) DeleteAll(_ context.Context, ownerID uuid.UUID) error {
	for _, root := range []string{s.published, s.uploads, s.retired} {
		if err := os.RemoveAll(filepath.Join(root, ownerID.String())); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
	}

	return nil
}

func (s *PhotoStore) DeleteExcept(_ context.Context, ownerID uuid.UUID, keep []uuid.UUID) error {
	dir := filepath.Join(s.published, ownerID.String())

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errs.Wrap("os.ReadDir", err)
	}

	for _, e := range entries {
		if id, err := uuid.Parse(e.Name()); err == nil && slices.Contains(keep, id) {
			continue
		}

		if err = os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
	}

	return nil
}

// Retire sets aside the uploads older than before: by the time in their
// UUIDv7. Leftovers of failed saves, older by their modification time,
// are deleted. An owner's directory left empty goes too.
func (s *PhotoStore) Retire(ctx context.Context, before time.Time) (int, error) {
	return s.sweep(ctx, s.uploads, before, func(owner, name string) error {
		if err := os.MkdirAll(filepath.Join(s.retired, owner), 0o750); err != nil {
			return errs.Wrap("os.MkdirAll", err)
		}

		return errs.Wrap("os.Rename", os.Rename(filepath.Join(s.uploads, owner, name), filepath.Join(s.retired, owner, name)))
	})
}

// DeleteRetired deletes the photos set aside that were uploaded before
// the time.
func (s *PhotoStore) DeleteRetired(ctx context.Context, before time.Time) (int, error) {
	return s.sweep(ctx, s.retired, before, func(owner, name string) error {
		return errs.Wrap("os.RemoveAll", os.RemoveAll(filepath.Join(s.retired, owner, name)))
	})
}

// sweep takes the photos under root older than before away with take,
// returning how many.
func (s *PhotoStore) sweep(ctx context.Context, root string, before time.Time, take func(owner, name string) error) (int, error) {
	owners, err := os.ReadDir(root)
	if err != nil {
		return 0, errs.Wrap("os.ReadDir", err)
	}

	taken := 0

	for _, owner := range owners {
		if err = ctx.Err(); err != nil {
			return taken, errs.Wrap("ctx.Err", err)
		}

		n, err := sweepOlder(filepath.Join(root, owner.Name()), before, func(name string) error {
			return take(owner.Name(), name)
		})
		taken += n
		if err != nil {
			return taken, err
		}
	}

	return taken, nil
}

func sweepOlder(dir string, before time.Time, take func(name string) error) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, errs.Wrap("os.ReadDir", err)
	}

	deleted, removed := 0, 0

	for _, e := range entries {
		uploaded, err := uploadTime(e)
		if err != nil {
			return deleted, err
		}
		if !uploaded.Before(before) {
			continue
		}

		// What is not a photo is a leftover: nothing will ask for it.
		if _, err = uuid.Parse(e.Name()); err != nil {
			if err = os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return deleted, errs.Wrap("os.RemoveAll", err)
			}
			removed++

			continue
		}

		if err = take(e.Name()); err != nil {
			return deleted, err
		}
		removed++
		deleted++
	}

	// Fails while something is in it, a photo saved meanwhile included.
	if removed == len(entries) {
		_ = os.Remove(dir)
	}

	return deleted, nil
}

func uploadTime(e os.DirEntry) (time.Time, error) {
	if id, err := uuid.Parse(e.Name()); err == nil && id.Version() == 7 {
		return time.Unix(id.Time().UnixTime()), nil
	}

	info, err := e.Info()
	if err != nil {
		return time.Time{}, errs.Wrap("e.Info", err)
	}

	return info.ModTime(), nil
}
