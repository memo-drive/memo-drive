package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/config"
)

const minTranscribedRunes = 8

type Transcriber interface {
	Available() bool
	Transcribe(ctx context.Context, audioPath string) (string, error)
	Mode() string
}

func NewTranscriber(cfg config.TranscribeConfig) Transcriber {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "whisper-cpp"
	}
	if !cfg.Enabled {
		log.Printf("level=warn component=transcribe event=skipped reason=disabled mode=%q", mode)
		return noopTranscriber{mode: mode, reason: "disabled"}
	}
	switch mode {
	case "openai":
		return newOpenAITranscriber(cfg)
	case "whisper-cpp", "whisper_cpp", "whisper":
		return newWhisperCPPTranscriber(cfg)
	default:
		log.Printf("level=warn component=transcribe event=skipped reason=unsupported_mode mode=%q", mode)
		return noopTranscriber{mode: mode, reason: "unsupported_mode"}
	}
}

type noopTranscriber struct {
	mode   string
	reason string
}

func (t noopTranscriber) Available() bool { return false }
func (t noopTranscriber) Mode() string    { return t.mode }
func (t noopTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	log.Printf("level=warn component=transcribe event=skipped reason=%s mode=%q file=%q", t.reason, t.mode, filepath.Base(audioPath))
	return "", nil
}

type whisperCPPTranscriber struct {
	bin       string
	modelPath string
	lang      string
	timeout   time.Duration
	maxBytes  int64
}

func newWhisperCPPTranscriber(cfg config.TranscribeConfig) Transcriber {
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		bin = "whisper-cli"
	}
	resolved, err := execLookPath(bin)
	if err != nil {
		log.Printf("level=warn component=transcribe event=skipped reason=unavailable mode=whisper-cpp bin=%q err=%q", bin, err)
		return noopTranscriber{mode: "whisper-cpp", reason: "unavailable"}
	}
	if strings.TrimSpace(cfg.ModelPath) == "" {
		log.Printf("level=warn component=transcribe event=skipped reason=missing_model mode=whisper-cpp")
		return noopTranscriber{mode: "whisper-cpp", reason: "missing_model"}
	}
	if _, err := os.Stat(cfg.ModelPath); err != nil {
		log.Printf("level=warn component=transcribe event=skipped reason=missing_model mode=whisper-cpp model_path=%q err=%q", cfg.ModelPath, err)
		return noopTranscriber{mode: "whisper-cpp", reason: "missing_model"}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 200 * 1024 * 1024
	}
	log.Printf("level=info component=transcribe event=ready mode=whisper-cpp bin=%q model_path=%q timeout_seconds=%.0f max_bytes=%d",
		resolved, cfg.ModelPath, timeout.Seconds(), maxBytes)
	return &whisperCPPTranscriber{
		bin:       resolved,
		modelPath: cfg.ModelPath,
		lang:      strings.TrimSpace(cfg.Lang),
		timeout:   timeout,
		maxBytes:  maxBytes,
	}
}

func (t *whisperCPPTranscriber) Available() bool { return t != nil && t.bin != "" && t.modelPath != "" }
func (t *whisperCPPTranscriber) Mode() string    { return "whisper-cpp" }

func (t *whisperCPPTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	started := time.Now()
	if err := enforceMaxFileSize(audioPath, t.maxBytes, ErrAudioTooLarge); err != nil {
		log.Printf("level=warn component=transcribe event=skipped reason=too_large mode=whisper-cpp file=%q err=%q", filepath.Base(audioPath), err)
		return "", err
	}
	runCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	wav, err := normalizeAudioForWhisper(runCtx, audioPath)
	if err != nil {
		return "", err
	}
	defer os.Remove(wav)

	tmpDir, err := os.MkdirTemp("", "memodrive-transcribe-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	stem := filepath.Join(tmpDir, "transcript")
	args := []string{"-m", t.modelPath, "-f", wav, "-otxt", "-of", stem, "--no-timestamps"}
	if t.lang != "" && !strings.EqualFold(t.lang, "auto") {
		args = append(args, "-l", t.lang)
	}
	out, err := exec.CommandContext(runCtx, t.bin, args...).CombinedOutput()
	if runCtx.Err() != nil {
		return "", runCtx.Err()
	}
	if err != nil {
		log.Printf("level=warn component=transcribe event=run_failed mode=whisper-cpp file=%q duration_ms=%d err=%q output=%q",
			filepath.Base(audioPath), time.Since(started).Milliseconds(), err, truncateForLog(string(out), 500))
		return "", err
	}
	text, err := os.ReadFile(stem + ".txt")
	if err != nil {
		return "", err
	}
	cleaned := cleanTranscribedText(string(text))
	log.Printf("level=info component=transcribe event=run_complete mode=whisper-cpp file=%q runes=%d duration_ms=%d",
		filepath.Base(audioPath), len([]rune(cleaned)), time.Since(started).Milliseconds())
	return cleaned, nil
}

type openAITranscriber struct {
	baseURL  string
	apiKey   string
	model    string
	lang     string
	timeout  time.Duration
	maxBytes int64
	client   *http.Client
}

func newOpenAITranscriber(cfg config.TranscribeConfig) Transcriber {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if baseURL == "" || strings.TrimSpace(cfg.APIKey) == "" {
		log.Printf("level=warn component=transcribe event=skipped reason=missing_api_config mode=openai")
		return noopTranscriber{mode: "openai", reason: "missing_api_config"}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 200 * 1024 * 1024
	}
	model := strings.TrimSpace(cfg.APIModel)
	if model == "" {
		model = "whisper-1"
	}
	log.Printf("level=info component=transcribe event=ready mode=openai base_url=%s model=%q timeout_seconds=%.0f max_bytes=%d",
		baseURL, model, timeout.Seconds(), maxBytes)
	return &openAITranscriber{
		baseURL:  baseURL,
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    model,
		lang:     strings.TrimSpace(cfg.Lang),
		timeout:  timeout,
		maxBytes: maxBytes,
		client:   &http.Client{Timeout: timeout},
	}
}

func (t *openAITranscriber) Available() bool { return t != nil && t.baseURL != "" && t.apiKey != "" }
func (t *openAITranscriber) Mode() string    { return "openai" }

func (t *openAITranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	started := time.Now()
	if err := enforceMaxFileSize(audioPath, t.maxBytes, ErrAudioTooLarge); err != nil {
		log.Printf("level=warn component=transcribe event=skipped reason=too_large mode=openai file=%q err=%q", filepath.Base(audioPath), err)
		return "", err
	}
	file, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	_ = writer.WriteField("model", t.model)
	_ = writer.WriteField("response_format", "text")
	if t.lang != "" && !strings.EqualFold(t.lang, "auto") {
		_ = writer.WriteField("language", t.lang)
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint, err := url.JoinPath(t.baseURL, "audio", "transcriptions")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai transcription failed: status=%d body=%s", resp.StatusCode, truncateForLog(string(respBody), 500))
	}
	cleaned := cleanTranscribedText(string(respBody))
	log.Printf("level=info component=transcribe event=run_complete mode=openai file=%q runes=%d duration_ms=%d",
		filepath.Base(audioPath), len([]rune(cleaned)), time.Since(started).Milliseconds())
	return cleaned, nil
}

func ParseAudio(ctx context.Context, transcriber Transcriber, absPath string) (*ParsedDocument, error) {
	started := time.Now()
	if transcriber == nil || !transcriber.Available() {
		log.Printf("level=warn component=parser parser=audio event=skipped file=%q reason=transcriber_unavailable", filepath.Base(absPath))
		return &ParsedDocument{Meta: map[string]string{"source": "audio_transcribe", "skipped": "transcriber_unavailable"}}, nil
	}
	text, err := transcriber.Transcribe(ctx, absPath)
	if err != nil {
		if isTooLargeErr(err) {
			return &ParsedDocument{Meta: map[string]string{"source": "audio_transcribe", "skipped": err.Error()}}, nil
		}
		return nil, err
	}
	if text == "" {
		log.Printf("level=info component=parser parser=audio event=empty file=%q duration_ms=%d", filepath.Base(absPath), time.Since(started).Milliseconds())
		return &ParsedDocument{Meta: map[string]string{"source": "audio_transcribe", "skipped": "empty_text"}}, nil
	}
	return &ParsedDocument{
		Text:  text,
		Title: filepath.Base(absPath),
		Sections: []Section{{
			Heading: "音频转录",
			Body:    text,
		}},
		Meta: map[string]string{
			"source": "audio_transcribe",
			"mode":   transcriber.Mode(),
		},
	}, nil
}

func normalizeAudioForWhisper(ctx context.Context, inputPath string) (string, error) {
	tmp, err := os.CreateTemp("", "memodrive-transcribe-*.wav")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	args := []string{"-y", "-i", inputPath, "-ar", "16000", "-ac", "1", "-f", "wav", tmpPath}
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if ctx.Err() != nil {
		_ = os.Remove(tmpPath)
		return "", ctx.Err()
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("normalize audio with ffmpeg: %w output=%s", err, truncateForLog(string(out), 500))
	}
	return tmpPath, nil
}

func enforceMaxFileSize(path string, maxBytes int64, tooLargeErr error) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return fmt.Errorf("%w: bytes=%d max_bytes=%d", tooLargeErr, info.Size(), maxBytes)
	}
	return nil
}

func cleanTranscribedText(text string) string {
	text = cleanExtractedText(text)
	if len([]rune(text)) < minTranscribedRunes {
		return ""
	}
	return text
}
