// Package document is a file of "Мои Документы" (openvk's documents),
// kept as uploaded and served for download. Its name and format the post
// service keeps; here it is bytes.
package document

import "errors"

// MaxBytes is the largest document, as openvk's docs.maxSize.
const MaxBytes = 10 << 20

var (
	ErrNotFound = errors.New("document not found")
	ErrEmpty    = errors.New("document is empty")
	ErrTooBig   = errors.New("document is larger than 10 MB")
)

// Check tells whether data may be kept as a document.
func Check(data []byte) error {
	switch {
	case len(data) == 0:
		return ErrEmpty
	case len(data) > MaxBytes:
		return ErrTooBig
	default:
		return nil
	}
}
