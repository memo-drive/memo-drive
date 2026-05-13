package parser

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/memodrive/backend/internal/config"
)

func TestExtractVideoFrameTextIncludesFirstFrameForShortVideos(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake ffmpeg shell scripts are only used on unix-like systems")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	videoPath := filepath.Join(root, "short.mp4")
	if err := os.WriteFile(videoPath, []byte("fake short video"), 0o644); err != nil {
		t.Fatalf("write fake video: %v", err)
	}
	argsLog := filepath.Join(root, "ffmpeg.args")
	writeExecutable(t, filepath.Join(binDir, "ffmpeg"), `#!/bin/sh
printf '%s\n' "$*" > "$FFMPEG_ARGS_LOG"
case "$*" in
  *"select='eq(n\\,0)+gte(t-prev_selected_t\\,30)',format=yuvj420p"*) ;;
  *) exit 234 ;;
esac
out=""
for arg in "$@"; do
  out="$arg"
done
out_file=$(printf '%s\n' "$out" | sed 's/%04d/0001/')
printf 'fake frame' > "$out_file"
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FFMPEG_ARGS_LOG", argsLog)
	ocr := &OCRRunner{
		ready: true,
		runFunc: func(ctx context.Context, imagePath string) (string, error) {
			if _, err := os.Stat(imagePath); err != nil {
				t.Fatalf("OCR received missing frame %q: %v", imagePath, err)
			}
			return "screen text", nil
		},
	}

	frames, err := extractVideoFrameText(context.Background(), config.VideoConfig{
		FrameInterval: 30,
		FrameLimit:    60,
	}, ocr, videoPath)
	if err != nil {
		t.Fatalf("extractVideoFrameText returned error: %v", err)
	}
	if len(frames) != 1 || frames[0].TimestampSec != 0 || frames[0].Text != "screen text" {
		t.Fatalf("expected OCR from the first frame, got %#v", frames)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read ffmpeg args: %v", err)
	}
	if !strings.Contains(string(args), "select='eq(n\\,0)+gte(t-prev_selected_t\\,30)',format=yuvj420p") {
		t.Fatalf("expected first-frame-preserving filter, args were %q", string(args))
	}
}
