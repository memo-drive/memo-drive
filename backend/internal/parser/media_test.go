package parser

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/disintegration/imaging"
)

func TestExtractMediaCreatesMOVThumbnailFromFirstFrame(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake ffmpeg shell scripts are only used on unix-like systems")
	}
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	videoPath := filepath.Join(root, "clip.mov")
	if err := os.WriteFile(videoPath, []byte("fake video"), 0o644); err != nil {
		t.Fatalf("write fake video: %v", err)
	}
	framePath := filepath.Join(root, "first-frame.jpg")
	writeTestJPEG(t, framePath)
	argsLog := filepath.Join(root, "ffmpeg.args")
	writeExecutable(t, filepath.Join(binDir, "ffprobe"), `#!/bin/sh
cat <<'JSON'
{"streams":[{"codec_type":"video","codec_name":"h264","width":640,"height":360}],"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"1.250","bit_rate":"1200000"}}
JSON
`)
	writeExecutable(t, filepath.Join(binDir, "ffmpeg"), `#!/bin/sh
printf '%s\n' "$*" > "$FFMPEG_ARGS_LOG"
out=""
for arg in "$@"; do
  out="$arg"
done
cp "$TEST_FIRST_FRAME" "$out"
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_FIRST_FRAME", framePath)
	t.Setenv("FFMPEG_ARGS_LOG", argsLog)

	thumbnailDir := filepath.Join(root, "thumbs")
	meta, thumbnail, err := ExtractMedia(context.Background(), videoPath, "application/octet-stream", "video-1", thumbnailDir)
	if err != nil {
		t.Fatalf("ExtractMedia returned error: %v", err)
	}
	if thumbnail != "video-1.jpg" {
		t.Fatalf("expected video thumbnail name, got %q", thumbnail)
	}
	if meta.Width != 640 || meta.Height != 360 || meta.Codec != "h264" {
		t.Fatalf("expected video metadata to be preserved, got %#v", meta)
	}
	if _, err := os.Stat(filepath.Join(thumbnailDir, thumbnail)); err != nil {
		t.Fatalf("expected generated video thumbnail: %v", err)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read ffmpeg args: %v", err)
	}
	if !strings.Contains(string(args), "-frames:v 1") {
		t.Fatalf("expected ffmpeg to capture one frame, args were %q", string(args))
	}
}

func writeTestJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.NRGBA{R: 80, G: 120, B: 200, A: 255})
		}
	}
	if err := imaging.Save(img, path, imaging.JPEGQuality(82)); err != nil {
		t.Fatalf("write test jpeg: %v", err)
	}
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", filepath.Base(path), err)
	}
}
