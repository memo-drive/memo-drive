package parser

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/config"
)

type frameText struct {
	Index        int
	TimestampSec int
	Text         string
}

// ExtractVideoText extracts text from a video by sampling frames at intervals
// and running OCR on each frame, with optional audio transcription.
func ExtractVideoText(
	ctx context.Context,
	cfg config.VideoConfig,
	ocr *OCRRunner,
	transcriber Transcriber,
	absPath string,
) (*ParsedDocument, error) {
	started := time.Now()
	var sections []Section
	transcriptRunes := 0
	frames := []frameText{}

	if cfg.AudioEnabled && transcriber != nil && transcriber.Available() {
		transcript, err := extractVideoAudioTranscript(ctx, transcriber, absPath)
		if err != nil {
			log.Printf("level=warn component=parser parser=video_text event=audio_transcribe_failed file=%q err=%q", filepath.Base(absPath), err)
		} else if transcript != "" {
			transcriptRunes = len([]rune(transcript))
			sections = append(sections, Section{Heading: "音频转录", Body: transcript})
		}
	} else {
		log.Printf("level=warn component=parser parser=video_text event=audio_skipped file=%q enabled=%t transcriber_available=%t",
			filepath.Base(absPath), cfg.AudioEnabled, transcriber != nil && transcriber.Available())
	}

	if cfg.OCREnabled && ocr != nil && ocr.Available() {
		extracted, err := extractVideoFrameText(ctx, cfg, ocr, absPath)
		if err != nil {
			log.Printf("level=warn component=parser parser=video_text event=frame_ocr_failed file=%q err=%q", filepath.Base(absPath), err)
		} else {
			frames = extracted
			for _, frame := range frames {
				sections = append(sections, Section{
					Heading: formatFrameHeading(frame.TimestampSec),
					Body:    frame.Text,
				})
			}
		}
	} else {
		log.Printf("level=warn component=parser parser=video_text event=frame_ocr_skipped file=%q enabled=%t ocr_available=%t",
			filepath.Base(absPath), cfg.OCREnabled, ocr != nil && ocr.Available())
	}

	text := joinSections(sections)
	meta := map[string]string{
		"source":           "video",
		"frames":           strconv.Itoa(len(frames)),
		"transcript_runes": strconv.Itoa(transcriptRunes),
	}
	if text == "" {
		log.Printf("level=info component=parser parser=video_text event=empty file=%q frames=%d transcript_runes=%d duration_ms=%d",
			filepath.Base(absPath), len(frames), transcriptRunes, time.Since(started).Milliseconds())
		return &ParsedDocument{Meta: meta}, nil
	}
	if ocr != nil && ocr.MinTextRunes() > 0 && len([]rune(text)) < ocr.MinTextRunes() {
		meta["skipped"] = "text_too_short"
		return &ParsedDocument{Meta: meta}, nil
	}
	log.Printf("level=info component=parser parser=video_text event=complete file=%q frames=%d transcript_runes=%d runes=%d duration_ms=%d",
		filepath.Base(absPath), len(frames), transcriptRunes, len([]rune(text)), time.Since(started).Milliseconds())
	return &ParsedDocument{
		Text:     text,
		Title:    filepath.Base(absPath),
		Sections: sections,
		Meta:     meta,
	}, nil
}

func extractVideoAudioTranscript(ctx context.Context, transcriber Transcriber, absPath string) (string, error) {
	tmp, err := os.CreateTemp("", "memodrive-video-audio-*.wav")
	if err != nil {
		return "", err
	}
	audioPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(audioPath)
		return "", err
	}
	defer os.Remove(audioPath)

	args := []string{"-y", "-i", absPath, "-vn", "-ar", "16000", "-ac", "1", "-f", "wav", audioPath}
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("extract video audio with ffmpeg: %w output=%s", err, truncateForLog(string(out), 500))
	}
	return transcriber.Transcribe(ctx, audioPath)
}

func extractVideoFrameText(ctx context.Context, cfg config.VideoConfig, ocr *OCRRunner, absPath string) ([]frameText, error) {
	frameInterval := cfg.FrameInterval
	if frameInterval <= 0 {
		frameInterval = 30
	}
	frameLimit := cfg.FrameLimit
	if frameLimit <= 0 {
		frameLimit = 60
	}
	dir, err := os.MkdirTemp("", "memodrive-video-frames-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	pattern := filepath.Join(dir, "frame_%04d.jpg")
	filter := fmt.Sprintf("select='eq(n\\,0)+gte(t-prev_selected_t\\,%d)',format=yuvj420p", frameInterval)
	args := []string{"-y", "-i", absPath, "-vf", filter, "-fps_mode", "vfr", "-frames:v", strconv.Itoa(frameLimit), pattern}
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("extract video frames with ffmpeg: %w output=%s", err, truncateForLog(string(out), 2000))
	}
	files, err := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	frames := make([]frameText, 0, len(files))
	for i, framePath := range files {
		text, err := ocr.Run(ctx, framePath)
		if err != nil {
			log.Printf("level=warn component=parser parser=video_text event=frame_ocr_item_failed frame=%q err=%q", filepath.Base(framePath), err)
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		frames = append(frames, frameText{
			Index:        i,
			TimestampSec: i * frameInterval,
			Text:         text,
		})
	}
	return frames, nil
}

func formatFrameHeading(timestampSec int) string {
	if timestampSec < 0 {
		timestampSec = 0
	}
	return fmt.Sprintf("关键帧 %02d:%02d", timestampSec/60, timestampSec%60)
}

func joinSections(sections []Section) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		body := strings.TrimSpace(section.Body)
		if body == "" {
			continue
		}
		heading := strings.TrimSpace(section.Heading)
		if heading != "" {
			parts = append(parts, heading+"\n"+body)
		} else {
			parts = append(parts, body)
		}
	}
	return cleanExtractedText(strings.Join(parts, "\n\n"))
}
