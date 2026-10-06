package fs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

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
