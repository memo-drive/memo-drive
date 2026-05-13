package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/memodrive/backend/internal/model"
	"github.com/rwcarlsen/goexif/exif"
)

func ExtractMedia(ctx context.Context, absPath, mimeType, fileID, thumbnailDir string) (*model.MediaMeta, string, error) {
	started := time.Now()
	meta := &model.MediaMeta{}
	if strings.HasPrefix(mimeType, "image/") || isImageExt(absPath) {
		thumb, err := extractImage(absPath, fileID, thumbnailDir, meta)
		if err != nil {
			log.Printf("level=error component=parser parser=media event=image_extract_failed file=%q file_id=%s duration_ms=%d err=%q", filepath.Base(absPath), fileID, time.Since(started).Milliseconds(), err)
		} else {
			log.Printf("level=info component=parser parser=media event=image_extract_complete file=%q file_id=%s width=%d height=%d thumbnail=%t duration_ms=%d", filepath.Base(absPath), fileID, meta.Width, meta.Height, thumb != "", time.Since(started).Milliseconds())
		}
		return meta, thumb, err
	}
	if isVideoMedia(absPath, mimeType) || isAudioMedia(absPath, mimeType) {
		err := extractAV(ctx, absPath, meta)
		if err != nil {
			log.Printf("level=error component=parser parser=media event=av_extract_failed file=%q file_id=%s duration_ms=%d err=%q", filepath.Base(absPath), fileID, time.Since(started).Milliseconds(), err)
		} else {
			log.Printf("level=info component=parser parser=media event=av_extract_complete file=%q file_id=%s format=%q codec=%q duration=%.3f width=%d height=%d duration_ms=%d", filepath.Base(absPath), fileID, meta.Format, meta.Codec, meta.Duration, meta.Width, meta.Height, time.Since(started).Milliseconds())
		}
		if err != nil || !isVideoMedia(absPath, mimeType) {
			return meta, "", err
		}
		thumb, thumbErr := extractVideoThumbnail(ctx, absPath, fileID, thumbnailDir)
		if thumbErr != nil {
			log.Printf("level=warn component=parser parser=media event=video_thumbnail_skipped file=%q file_id=%s err=%q", filepath.Base(absPath), fileID, thumbErr)
			return meta, "", nil
		}
		log.Printf("level=info component=parser parser=media event=video_thumbnail_complete file=%q file_id=%s thumbnail=%t duration_ms=%d", filepath.Base(absPath), fileID, thumb != "", time.Since(started).Milliseconds())
		return meta, thumb, nil
	}
	return meta, "", nil
}

func extractImage(absPath, fileID, thumbnailDir string, meta *model.MediaMeta) (string, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	cfg, _, err := image.DecodeConfig(file)
	_ = file.Close()
	if err == nil {
		meta.Width = cfg.Width
		meta.Height = cfg.Height
	}

	if strings.EqualFold(filepath.Ext(absPath), ".jpg") || strings.EqualFold(filepath.Ext(absPath), ".jpeg") {
		if err := extractEXIF(absPath, meta); err != nil {
			// EXIF is best-effort: many images simply do not contain it.
		}
	}

	if err := os.MkdirAll(thumbnailDir, 0o755); err != nil {
		return "", err
	}
	img, err := imaging.Open(absPath, imaging.AutoOrientation(true))
	if err != nil {
		log.Printf("level=warn component=parser parser=media event=image_thumbnail_skipped file=%q err=%q", filepath.Base(absPath), err)
		return "", nil
	}
	thumb := imaging.Fit(img, 360, 360, imaging.Lanczos)
	thumbName := fileID + ".jpg"
	thumbPath := filepath.Join(thumbnailDir, thumbName)
	if err := imaging.Save(thumb, thumbPath, imaging.JPEGQuality(82)); err != nil {
		return "", err
	}
	return thumbName, nil
}

func extractEXIF(absPath string, meta *model.MediaMeta) error {
	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	x, err := exif.Decode(file)
	if err != nil {
		return err
	}
	if taken, err := x.DateTime(); err == nil {
		meta.TakenAt = &taken
	}
	if tag, err := x.Get(exif.Model); err == nil {
		if camera, err := tag.StringVal(); err == nil {
			meta.Camera = strings.TrimSpace(camera)
		}
	}
	if lat, lon, err := x.LatLong(); err == nil {
		meta.Latitude = &lat
		meta.Longitude = &lon
	}
	return nil
}

type ffprobeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		BitRate   string `json:"bit_rate"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

func extractAV(ctx context.Context, absPath string, meta *model.MediaMeta) error {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, "ffprobe", "-v", "error", "-show_entries", "format=duration,bit_rate,format_name", "-show_streams", "-of", "json", absPath).Output()
	if err != nil {
		log.Printf("level=warn component=parser parser=media event=ffprobe_failed file=%q err=%q", filepath.Base(absPath), err)
		return nil
	}
	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return err
	}
	meta.Format = parsed.Format.FormatName
	if duration, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
		meta.Duration = duration
	}
	if bitrate, err := strconv.Atoi(parsed.Format.BitRate); err == nil {
		meta.Bitrate = bitrate
	}
	for _, stream := range parsed.Streams {
		if stream.CodecType == "video" && (meta.Width == 0 || meta.Height == 0) {
			meta.Width = stream.Width
			meta.Height = stream.Height
			meta.Codec = stream.CodecName
			if bitrate, err := strconv.Atoi(stream.BitRate); err == nil && bitrate > 0 {
				meta.Bitrate = bitrate
			}
			continue
		}
		if stream.CodecType == "audio" && meta.Codec == "" {
			meta.Codec = stream.CodecName
		}
	}
	return nil
}

func extractVideoThumbnail(ctx context.Context, absPath, fileID, thumbnailDir string) (string, error) {
	if err := os.MkdirAll(thumbnailDir, 0o755); err != nil {
		return "", err
	}
	thumbName := fileID + ".jpg"
	thumbPath := filepath.Join(thumbnailDir, thumbName)
	thumbCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(
		thumbCtx,
		"ffmpeg",
		"-y",
		"-i",
		absPath,
		"-frames:v",
		"1",
		"-vf",
		"scale=360:-2",
		"-q:v",
		"4",
		thumbPath,
	).CombinedOutput()
	if err != nil {
		return "", errWithOutput("extract first video frame with ffmpeg", err, out)
	}
	return thumbName, nil
}

func errWithOutput(prefix string, err error, out []byte) error {
	if len(out) == 0 {
		return err
	}
	return fmt.Errorf("%s: %w output=%s", prefix, err, truncateForLog(string(out), 500))
}

func isVideoMedia(name, mimeType string) bool {
	if strings.HasPrefix(mimeType, "audio/") {
		return false
	}
	return strings.HasPrefix(mimeType, "video/") || isVideoExt(name)
}

func isAudioMedia(name, mimeType string) bool {
	if strings.HasPrefix(mimeType, "video/") {
		return false
	}
	return strings.HasPrefix(mimeType, "audio/") || isAudioExt(name)
}

func isImageExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic":
		return true
	default:
		return false
	}
}

func isAVExt(name string) bool {
	return isVideoExt(name) || isAudioExt(name)
}

func isVideoExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mov", ".mkv", ".webm":
		return true
	default:
		return false
	}
}

func isAudioExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".wav", ".flac", ".m4a":
		return true
	default:
		return false
	}
}
