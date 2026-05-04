package parser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
)

func TestOpenAITranscriberTranscribesMultipartAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-test" {
			t.Fatalf("unexpected model %q", got)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file field: %v", err)
		}
		defer file.Close()
		body, _ := io.ReadAll(file)
		if string(body) != "audio bytes" {
			t.Fatalf("unexpected file body %q", body)
		}
		_, _ = w.Write([]byte("hello from audio"))
	}))
	defer server.Close()

	audioPath := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(audioPath, []byte("audio bytes"), 0o644); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	transcriber := NewTranscriber(config.TranscribeConfig{
		Enabled:    true,
		Mode:       "openai",
		APIBaseURL: server.URL,
		APIKey:     "test-key",
		APIModel:   "whisper-test",
		Timeout:    5 * time.Second,
		MaxBytes:   10 << 20,
	})
	if !transcriber.Available() {
		t.Fatal("expected transcriber to be available")
	}
	text, err := transcriber.Transcribe(context.Background(), audioPath)
	if err != nil {
		t.Fatalf("Transcribe returned error: %v", err)
	}
	if text != "hello from audio" {
		t.Fatalf("unexpected transcript %q", text)
	}
}

func TestNewTranscriberDisabledReturnsNoop(t *testing.T) {
	transcriber := NewTranscriber(config.TranscribeConfig{Enabled: false, Mode: "openai"})
	if transcriber.Available() {
		t.Fatal("expected disabled transcriber to be unavailable")
	}
	if !strings.Contains(transcriber.Mode(), "openai") {
		t.Fatalf("unexpected mode %q", transcriber.Mode())
	}
	text, err := transcriber.Transcribe(context.Background(), "/tmp/missing.wav")
	if err != nil {
		t.Fatalf("noop transcriber returned error: %v", err)
	}
	if text != "" {
		t.Fatalf("expected empty transcript, got %q", text)
	}
}

func TestParseAudioUnavailableReturnsEmptyDocument(t *testing.T) {
	doc, err := ParseAudio(context.Background(), noopTranscriber{mode: "openai", reason: "disabled"}, "/tmp/sample.wav")
	if err != nil {
		t.Fatalf("ParseAudio returned error: %v", err)
	}
	if doc.Text != "" || doc.Meta["source"] != "audio_transcribe" {
		t.Fatalf("unexpected document %#v", doc)
	}
}
