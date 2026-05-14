package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/parser"
)

func (s *PipelineService) extractMediaText(ctx context.Context, file *model.File) (*parser.ParsedDocument, error) {
	absPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(file.StoragePath))
	switch {
	case strings.HasPrefix(file.MimeType, "image/") || isImageName(file.Name):
		return parser.ParseImageOCR(ctx, s.ocr, absPath)
	case strings.HasPrefix(file.MimeType, "audio/") || isAudioName(file.Name):
		return parser.ParseAudio(ctx, s.transcriber, absPath)
	case strings.HasPrefix(file.MimeType, "video/") || isVideoName(file.Name):
		return parser.ExtractVideoText(ctx, s.cfg.Video, s.ocr, s.transcriber, absPath)
	default:
		return nil, nil
	}
}

func (s *PipelineService) mediaTextExtractionEnabled() bool {
	if s == nil || s.cfg == nil {
		return false
	}
	return s.cfg.OCR.Enabled || s.cfg.Transcribe.Enabled
}

// IsMedia reports whether a file is a media file (image, video, or audio) based on
// its MIME type or file extension.
func IsMedia(mimeType, name string) bool {
	if strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".mp4", ".mov", ".mkv", ".webm", ".mp3", ".wav", ".flac", ".m4a":
		return true
	default:
		return false
	}
}

func isImageName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic":
		return true
	default:
		return false
	}
}

func isAudioName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".wav", ".flac", ".m4a":
		return true
	default:
		return false
	}
}

func isVideoName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mov", ".mkv", ".webm":
		return true
	default:
		return false
	}
}
