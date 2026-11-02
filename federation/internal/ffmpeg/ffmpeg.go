// Package ffmpeg cuts the first frames of videos of other servers with
// the ffmpeg program, for the covers of the videos in the news.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
	"golang.org/x/sync/singleflight"
)

// parallel is how many videos are cut at once: each takes a core.
const parallel = 2

// Source reads the beginning of a video; one that is no video is
// fedi.ErrNotFound.
type Source interface {
	VideoHead(ctx context.Context, u fedi.URL) ([]byte, error)
}

// Framer cuts the first frame of a video as a JPEG at most 640 pixels
// wide. It reads only the beginning of the video, and runs ffmpeg on it
// with no files and no network, for the formats of videos only.
type Framer struct {
	path    string
	timeout time.Duration
	source  Source
	slots   chan struct{}
	// cuts makes the readers of a cover the news just showed many people
	// wait for one cut.
	cuts singleflight.Group
}

func New(path string, timeout time.Duration, source Source) *Framer {
	return &Framer{path: path, timeout: timeout, source: source, slots: make(chan struct{}, parallel)}
}

func (f *Framer) Frame(ctx context.Context, video fedi.URL) ([]byte, error) {
	frame, err, _ := f.cuts.Do(video.String(), func() (any, error) {
		return f.frame(ctx, video)
	})
	if err != nil {
		return nil, err
	}

	return frame.([]byte), nil
}

func (f *Framer) frame(ctx context.Context, video fedi.URL) ([]byte, error) {
	select {
	case f.slots <- struct{}{}:
		defer func() { <-f.slots }()
	case <-ctx.Done():
		return nil, errs.Wrap("ctx.Done", ctx.Err())
	}

	head, err := f.source.VideoHead(ctx, video)
	if err != nil {
		return nil, errs.Wrap("f.source.VideoHead", err)
	}

	return f.cut(ctx, head)
}

// cut runs ffmpeg on the beginning of a video. A video it cannot read, or
// takes too long with, has no frame.
func (f *Framer) cut(ctx context.Context, head []byte) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, f.path, //nolint:gosec // the path comes from the configuration
		"-hide_banner", "-loglevel", "error",
		"-protocol_whitelist", "pipe",
		"-format_whitelist", "mov,mp4,m4a,3gp,3g2,mj2,matroska,webm",
		"-i", "pipe:0",
		"-map", "0:v:0", "-an",
		"-frames:v", "1",
		"-vf", "scale='min(640,iw)':-2",
		"-f", "image2", "-c:v", "mjpeg", "-q:v", "4",
		"pipe:1",
	)

	var out bytes.Buffer
	cmd.Stdin = bytes.NewReader(head)
	cmd.Stdout = &out

	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, errs.Wrap("cmd.Run", ctx.Err())
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return nil, fedi.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("cmd.Run", err)
	}

	if !bytes.HasPrefix(out.Bytes(), []byte{0xff, 0xd8}) {
		return nil, fedi.ErrNotFound
	}

	return out.Bytes(), nil
}
