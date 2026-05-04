package parser

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/memodrive/backend/internal/config"
)

var execLookPath = exec.LookPath

type OCRRunner struct {
	bin           string
	langs         string
	timeout       time.Duration
	minTextRunes  int
	maxImageBytes int64
	ready         bool
	reason        string

	runFunc func(ctx context.Context, imagePath string) (string, error)
}

func NewOCRRunner(cfg config.OCRConfig) *OCRRunner {
	r := &OCRRunner{
		bin:           strings.TrimSpace(cfg.Bin),
		langs:         strings.TrimSpace(cfg.Langs),
		timeout:       cfg.Timeout,
		minTextRunes:  cfg.MinTextRunes,
		maxImageBytes: cfg.MaxImageBytes,
	}
	if r.bin == "" {
		r.bin = "tesseract"
	}
	if r.langs == "" {
		r.langs = "eng+chi_sim"
	}
	if r.timeout <= 0 {
		r.timeout = 60 * time.Second
	}
	if r.maxImageBytes <= 0 {
		r.maxImageBytes = 20 * 1024 * 1024
	}
	if !cfg.Enabled {
		r.reason = "disabled"
		log.Printf("level=warn component=ocr event=skipped reason=disabled")
		return r
	}
	path, err := execLookPath(r.bin)
	if err != nil {
		r.reason = fmt.Sprintf("binary %q not found", r.bin)
		log.Printf("level=warn component=ocr event=skipped reason=unavailable bin=%q err=%q", r.bin, err)
		return r
	}
	r.bin = path
	r.ready = true
	log.Printf("level=info component=ocr event=ready bin=%q langs=%q timeout_seconds=%.0f max_image_bytes=%d min_text_runes=%d",
		r.bin, r.langs, r.timeout.Seconds(), r.maxImageBytes, r.minTextRunes)
	return r
}

func (r *OCRRunner) Available() bool {
	return r != nil && r.ready
}

func (r *OCRRunner) Langs() string {
	if r == nil {
		return ""
	}
	return r.langs
}

func (r *OCRRunner) MinTextRunes() int {
	if r == nil {
		return 0
	}
	return r.minTextRunes
}

func (r *OCRRunner) Run(ctx context.Context, imagePath string) (string, error) {
	started := time.Now()
	if r == nil || !r.ready {
		reason := "unavailable"
		if r != nil && r.reason != "" {
			reason = r.reason
		}
		log.Printf("level=warn component=ocr event=skipped reason=%s file=%q", reason, filepath.Base(imagePath))
		return "", nil
	}
	if r.runFunc != nil {
		text, err := r.runFunc(ctx, imagePath)
		if err != nil {
			return "", err
		}
		cleaned := cleanOCRText(text, r.minTextRunes)
		log.Printf("level=info component=ocr event=run_complete file=%q runes=%d duration_ms=%d",
			filepath.Base(imagePath), len([]rune(cleaned)), time.Since(started).Milliseconds())
		return cleaned, nil
	}

	info, err := os.Stat(imagePath)
	if err != nil {
		return "", err
	}
	if info.Size() > r.maxImageBytes {
		log.Printf("level=warn component=ocr event=skipped reason=too_large file=%q bytes=%d max_bytes=%d",
			filepath.Base(imagePath), info.Size(), r.maxImageBytes)
		return "", ErrImageTooLarge
	}

	inputPath := imagePath
	cleanup := func() {}
	if needsOCRPNGConversion(imagePath) {
		converted, err := convertImageForOCR(imagePath)
		if err != nil {
			return "", fmt.Errorf("convert image for ocr: %w", err)
		}
		inputPath = converted
		cleanup = func() { _ = os.Remove(converted) }
	}
	defer cleanup()

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, r.bin, inputPath, "stdout", "-l", r.langs, "--psm", "3", "-c", "preserve_interword_spaces=1")
	out, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return "", runCtx.Err()
	}
	if err != nil {
		log.Printf("level=warn component=ocr event=run_failed file=%q duration_ms=%d err=%q output=%q",
			filepath.Base(imagePath), time.Since(started).Milliseconds(), err, truncateForLog(string(out), 500))
		return "", err
	}
	cleaned := cleanOCRText(string(out), r.minTextRunes)
	log.Printf("level=info component=ocr event=run_complete file=%q runes=%d duration_ms=%d",
		filepath.Base(imagePath), len([]rune(cleaned)), time.Since(started).Milliseconds())
	return cleaned, nil
}

func needsOCRPNGConversion(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".bmp":
		return false
	default:
		return true
	}
}

func convertImageForOCR(path string) (string, error) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "memodrive-ocr-*.png")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}
	if err := imaging.Save(img, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func cleanOCRText(text string, minTextRunes int) string {
	text = cleanExtractedText(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			kept = append(kept, "")
			continue
		}
		if len([]rune(trimmed)) == 1 {
			continue
		}
		kept = append(kept, trimmed)
	}
	text = cleanExtractedText(strings.Join(kept, "\n"))
	if minTextRunes > 0 && len([]rune(text)) < minTextRunes {
		return ""
	}
	return text
}

func truncateForLog(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func isTooLargeErr(err error) bool {
	return errors.Is(err, ErrImageTooLarge) || errors.Is(err, ErrAudioTooLarge)
}
