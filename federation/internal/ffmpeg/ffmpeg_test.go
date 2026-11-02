package ffmpeg_test

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/federation/internal/ffmpeg"
)

// source serves the videos it has.
type source map[fedi.URL][]byte

func (s source) VideoHead(_ context.Context, u fedi.URL) ([]byte, error) {
	v, ok := s[u]
	if !ok {
		return nil, fedi.ErrNotFound
	}

	return v, nil
}

func TestFramer(t *testing.T) {
	t.Parallel()

	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg here")
	}

	video := filepath.Join(t.TempDir(), "v.mp4")
	// As Mastodon keeps videos: what tells how to play them first.
	out, err := exec.Command(path, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=10",
		"-t", "2", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", "-movflags", "faststart", video).CombinedOutput()
	if err != nil {
		t.Fatalf("making a video: %v: %s", err, out)
	}

	whole, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}

	f := ffmpeg.New(path, 10*time.Second, source{
		"https://m.example/v.mp4":    whole,
		"https://m.example/head.mp4": whole[:len(whole)/2],
		"https://m.example/junk.mp4": []byte("no video at all"),
	})

	for _, u := range []fedi.URL{"https://m.example/v.mp4", "https://m.example/head.mp4"} {
		frame, err := f.Frame(context.Background(), u)
		if err != nil {
			t.Fatalf("Frame %s: %v", u, err)
		}

		cfg, err := jpeg.DecodeConfig(bytes.NewReader(frame))
		if err != nil || cfg.Width != 640 || cfg.Height != 360 {
			t.Fatalf("frame of %s = %+v, %v; want a JPEG 640×360", u, cfg, err)
		}
	}

	for _, u := range []fedi.URL{"https://m.example/junk.mp4", "https://m.example/missing.mp4"} {
		if _, err := f.Frame(context.Background(), u); !errors.Is(err, fedi.ErrNotFound) {
			t.Errorf("Frame %s = %v, want ErrNotFound", u, err)
		}
	}
}

func TestFramer_NoProgram(t *testing.T) {
	t.Parallel()

	f := ffmpeg.New(filepath.Join(t.TempDir(), "ffmpeg"), time.Second, source{"https://m.example/v.mp4": []byte("mp4")})

	if _, err := f.Frame(context.Background(), "https://m.example/v.mp4"); err == nil || errors.Is(err, fedi.ErrNotFound) {
		t.Fatalf("Frame without ffmpeg = %v, want a failure, not a video with no frame", err)
	}
}
