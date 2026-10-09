package fs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/vexner67/freenet/pkg/errs"
)

// saveDir creates the directory final with the given files. It writes
// into a temporary directory and renames it, so a reader never sees half
// of the files.
func saveDir(final string, files map[string][]byte) error {
	tmp, err := os.MkdirTemp(filepath.Dir(final), ".upload-*")
	if errors.Is(err, fs.ErrNotExist) {
		if err = os.MkdirAll(filepath.Dir(final), 0o750); err == nil {
			tmp, err = os.MkdirTemp(filepath.Dir(final), ".upload-*")
		}
	}
	if err != nil {
		return errs.Wrap("os.MkdirTemp", err)
	}

	for name, data := range files {
		if err = os.WriteFile(filepath.Join(tmp, name), data, 0o640); err != nil {
			_ = os.RemoveAll(tmp)
			return errs.Wrap("os.WriteFile", err)
		}
	}

	if err = os.Rename(tmp, final); err != nil {
		_ = os.RemoveAll(tmp)
		return errs.Wrap("os.Rename", err)
	}

	return nil
}

// linkDir creates the directory final with hard links to the named files
// of dir, through a temporary directory, as saveDir. It returns
// fs.ErrNotExist when dir has no such file, and nothing when final is
// there already.
func linkDir(dir, final string, names ...string) error {
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return errs.Wrap("os.MkdirAll", err)
	}

	tmp, err := os.MkdirTemp(filepath.Dir(final), ".upload-*")
	if err != nil {
		return errs.Wrap("os.MkdirTemp", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }() // gone already when renamed

	for _, name := range names {
		if err = os.Link(filepath.Join(dir, name), filepath.Join(tmp, name)); err != nil {
			return errs.Wrap("os.Link", err)
		}
	}

	// Renaming over a directory with files fails: another copy was first.
	if err = os.Rename(tmp, final); err != nil && !errors.Is(err, fs.ErrExist) && !isNotEmpty(err) {
		return errs.Wrap("os.Rename", err)
	}

	return nil
}

func isNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}
