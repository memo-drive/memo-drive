// Package config loads and validates server configuration from environment variables.
// All settings have sensible defaults for local development.
package config

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultJWTSecret = "change-me-in-production"

// Config is the root configuration structure containing all subsystem settings.
type Config struct {
	Server     ServerConfig
	Storage    StorageConfig
	Auth       AuthConfig
	Pipeline   PipelineConfig
	LLM        LLMConfig
	RAG        RAGConfig
	OCR        OCRConfig
	Transcribe TranscribeConfig
	Video      VideoConfig
	Janitor    JanitorConfig
	Trash      TrashConfig
}

// ServerConfig holds HTTP server bind address settings.
type ServerConfig struct {
	Host string
	Port string
}

// StorageConfig configures the file storage layer including paths, limits, and upload behavior.
type StorageConfig struct {
	Root         string
	DBPath       string
	TempDir      string
	ThumbnailDir string
	MaxFileSize  int64
	ChunkSize    int64
	UploadTTL    time.Duration
}

// AuthConfig holds JWT-based authentication settings.
type AuthConfig struct {
	Password  string
	JWTSecret string
	TokenTTL  time.Duration
}

// PipelineConfig controls the document processing pipeline: text splitting, embedding, and indexing.
type PipelineConfig struct {
	Workers         int
	SkipLarge       int64
	ChunkSize       int
	ChunkOverlap    int
	EmbedBatchSize  int
	ParentChunkSize int
	ChildChunkSize  int
}

// LLMConfig holds connection details for language model providers (Ollama and OpenAI)
// and the vector database (ChromaDB).
type LLMConfig struct {
	OllamaBaseURL string
	OllamaChat    string
	OllamaEmbed   string
	OpenAIBaseURL string
	OpenAIAPIKey  string
	OpenAIChat    string
	OpenAIEmbed   string
	ChromaBaseURL string
}

// RAGConfig tunes retrieval-augmented generation behavior: retrieval strategy,
// result counts, scoring thresholds, and query rewriting.
type RAGConfig struct {
	TopK              int
	SearchTopK        int
	MaxContextChars   int
	MinScore          float32
	QueryCondense     bool
	CondenseModel     string
	ScorePercentile   float32
	MultiQuery        bool
	MultiQueryCount   int
	HybridSearch      bool
	RRFConstant       int
	IntentParse       bool
	IntentLLMFallback bool
	IntentTimezone    string
	IntentFileLimit   int
}

// OCRConfig configures Tesseract OCR for extracting text from images.
type OCRConfig struct {
	Enabled       bool
	Bin           string
	Langs         string
	Timeout       time.Duration
	MinTextRunes  int
	MaxImageBytes int64
}

// TranscribeConfig configures audio transcription, supporting local whisper.cpp
// or remote API-based transcription.
type TranscribeConfig struct {
	Enabled    bool
	Mode       string
	Bin        string
	ModelPath  string
	APIBaseURL string
	APIKey     string
	APIModel   string
	Lang       string
	Timeout    time.Duration
	MaxBytes   int64
}

// VideoConfig controls video processing: frame extraction for OCR and audio track handling.
type VideoConfig struct {
	OCREnabled    bool
	FrameInterval int
	FrameLimit    int
	AudioEnabled  bool
}

// JanitorConfig controls the background maintenance worker that recovers stuck tasks
// and optionally sweeps orphaned storage files.
type JanitorConfig struct {
	Enabled             bool
	Interval            time.Duration
	RecoverOnBoot       bool
	MaxTaskAge          time.Duration
	SweepStorageEnabled bool
}

// TrashConfig controls automatic trash purging behavior.
type TrashConfig struct {
	AutoPurgeEnabled bool
	RetentionDays    int
}

// Load reads configuration from environment variables and returns a validated Config.
// It applies defaults for any unset variables and returns an error if required
// values are invalid (e.g., non-positive chunk sizes).
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: envString("HOST", "0.0.0.0"),
			Port: envString("PORT", "8080"),
		},
		Storage: StorageConfig{
			Root:         envString("STORAGE_ROOT", "./data/files"),
			DBPath:       databasePath(envString("DB_PATH", "./data/db")),
			TempDir:      envString("UPLOAD_TEMP_DIR", "./data/tmp"),
			ThumbnailDir: envString("THUMBNAIL_DIR", "./data/thumbnails"),
			MaxFileSize:  envInt64("MAX_FILE_SIZE", 5*1024*1024*1024),
			ChunkSize:    envInt64("UPLOAD_CHUNK_SIZE", 10*1024*1024),
			UploadTTL:    time.Duration(envInt64("UPLOAD_TIMEOUT", 3600)) * time.Second,
		},
		Auth: AuthConfig{
			Password:  envString("ADMIN_PASSWORD", ""),
			JWTSecret: envString("JWT_SECRET", defaultJWTSecret),
			TokenTTL:  time.Duration(envInt64("TOKEN_TTL_SECONDS", int64(7*24*time.Hour/time.Second))) * time.Second,
		},
		Pipeline: PipelineConfig{
			Workers:         envInt("PIPELINE_WORKERS", 4),
			SkipLarge:       envInt64("PIPELINE_SKIP_LARGE", 1024*1024*1024),
			ChunkSize:       envInt("PIPELINE_CHUNK_SIZE", 500),
			ChunkOverlap:    envInt("PIPELINE_CHUNK_OVERLAP", 100),
			EmbedBatchSize:  envInt("PIPELINE_EMBED_BATCH_SIZE", 20),
			ParentChunkSize: envInt("PIPELINE_PARENT_CHUNK_SIZE", 1024),
			ChildChunkSize:  envInt("PIPELINE_CHILD_CHUNK_SIZE", 256),
		},
		LLM: LLMConfig{
			OllamaBaseURL: envString("OLLAMA_BASE_URL", "http://ollama:11434"),
			OllamaChat:    envString("OLLAMA_CHAT_MODEL", "qwen2.5:1.5b"),
			OllamaEmbed:   envString("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
			OpenAIBaseURL: envString("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			OpenAIAPIKey:  envString("OPENAI_API_KEY", ""),
			OpenAIChat:    envString("OPENAI_CHAT_MODEL", "gpt-4o"),
			OpenAIEmbed:   envString("OPENAI_EMBED_MODEL", "text-embedding-3-small"),
			ChromaBaseURL: envString("CHROMA_BASE_URL", "http://chroma:8000"),
		},
		RAG: RAGConfig{
			TopK:              envInt("RAG_TOP_K", 5),
			SearchTopK:        envInt("RAG_SEARCH_TOP_K", 10),
			MaxContextChars:   envInt("RAG_MAX_CONTEXT_CHARS", 6000),
			MinScore:          envFloat32("RAG_MIN_SCORE", 0),
			QueryCondense:     envBool("RAG_QUERY_CONDENSE", true),
			CondenseModel:     envString("RAG_CONDENSE_MODEL", ""),
			ScorePercentile:   envFloat32("RAG_SCORE_PERCENTILE", 0.25),
			MultiQuery:        envBool("RAG_MULTI_QUERY", true),
			MultiQueryCount:   envInt("RAG_MULTI_QUERY_COUNT", 3),
			HybridSearch:      envBool("RAG_HYBRID_SEARCH", true),
			RRFConstant:       envInt("RAG_RRF_K", 60),
			IntentParse:       envBool("RAG_INTENT_PARSE", true),
			IntentLLMFallback: envBool("RAG_INTENT_LLM_FALLBACK", true),
			IntentTimezone:    envString("RAG_INTENT_TIMEZONE", "Asia/Shanghai"),
			IntentFileLimit:   envInt("RAG_INTENT_FILE_LIMIT", 500),
		},
		OCR: OCRConfig{
			Enabled:       envBool("OCR_ENABLED", true),
			Bin:           envString("OCR_BIN", "tesseract"),
			Langs:         envString("OCR_LANGS", "eng+chi_sim"),
			Timeout:       time.Duration(envInt64("OCR_TIMEOUT_SECONDS", 60)) * time.Second,
			MinTextRunes:  envInt("OCR_MIN_TEXT_RUNES", 8),
			MaxImageBytes: envInt64("OCR_MAX_IMAGE_BYTES", 20*1024*1024),
		},
		Transcribe: TranscribeConfig{
			Enabled:    envBool("TRANSCRIBE_ENABLED", false),
			Mode:       envString("TRANSCRIBE_MODE", "whisper-cpp"),
			Bin:        envString("TRANSCRIBE_BIN", "whisper-cli"),
			ModelPath:  envString("TRANSCRIBE_MODEL_PATH", ""),
			APIBaseURL: envString("TRANSCRIBE_API_BASE_URL", "https://api.openai.com/v1"),
			APIKey:     envString("TRANSCRIBE_API_KEY", ""),
			APIModel:   envString("TRANSCRIBE_API_MODEL", "whisper-1"),
			Lang:       envString("TRANSCRIBE_LANG", "auto"),
			Timeout:    time.Duration(envInt64("TRANSCRIBE_TIMEOUT_SECONDS", 600)) * time.Second,
			MaxBytes:   envInt64("TRANSCRIBE_MAX_BYTES", 200*1024*1024),
		},
		Video: VideoConfig{
			OCREnabled:    envBool("VIDEO_OCR_ENABLED", true),
			FrameInterval: envInt("VIDEO_FRAME_INTERVAL_SECONDS", 30),
			FrameLimit:    envInt("VIDEO_FRAME_LIMIT", 60),
			AudioEnabled:  envBool("VIDEO_AUDIO_ENABLED", true),
		},
		Janitor: JanitorConfig{
			Enabled:             envBool("JANITOR_ENABLED", true),
			Interval:            time.Duration(envInt64("JANITOR_INTERVAL_SECONDS", 600)) * time.Second,
			RecoverOnBoot:       envBool("JANITOR_RECOVER_ON_BOOT", true),
			MaxTaskAge:          time.Duration(envInt64("JANITOR_MAX_TASK_AGE_SECONDS", 1800)) * time.Second,
			SweepStorageEnabled: envBool("JANITOR_SWEEP_STORAGE_ENABLED", false),
		},
		Trash: TrashConfig{
			AutoPurgeEnabled: envBool("TRASH_AUTO_PURGE", true),
			RetentionDays:    envInt("TRASH_RETENTION_DAYS", 30),
		},
	}
	if cfg.Storage.ChunkSize <= 0 {
		return nil, errors.New("UPLOAD_CHUNK_SIZE must be greater than zero")
	}
	if cfg.Storage.MaxFileSize <= 0 {
		return nil, errors.New("MAX_FILE_SIZE must be greater than zero")
	}
	if cfg.Pipeline.ChunkSize <= 0 {
		return nil, errors.New("PIPELINE_CHUNK_SIZE must be greater than zero")
	}
	if cfg.Pipeline.ChunkOverlap < 0 {
		return nil, errors.New("PIPELINE_CHUNK_OVERLAP must not be negative")
	}
	if cfg.Pipeline.EmbedBatchSize <= 0 {
		return nil, errors.New("PIPELINE_EMBED_BATCH_SIZE must be greater than zero")
	}
	if cfg.Pipeline.ParentChunkSize <= 0 {
		return nil, errors.New("PIPELINE_PARENT_CHUNK_SIZE must be greater than zero")
	}
	if cfg.Pipeline.ChildChunkSize <= 0 {
		return nil, errors.New("PIPELINE_CHILD_CHUNK_SIZE must be greater than zero")
	}
	if cfg.Pipeline.ChildChunkSize > cfg.Pipeline.ParentChunkSize {
		return nil, errors.New("PIPELINE_CHILD_CHUNK_SIZE must not exceed PIPELINE_PARENT_CHUNK_SIZE")
	}
	if cfg.RAG.TopK <= 0 {
		return nil, errors.New("RAG_TOP_K must be greater than zero")
	}
	if cfg.RAG.SearchTopK <= 0 {
		return nil, errors.New("RAG_SEARCH_TOP_K must be greater than zero")
	}
	if cfg.RAG.MaxContextChars <= 0 {
		return nil, errors.New("RAG_MAX_CONTEXT_CHARS must be greater than zero")
	}
	if cfg.RAG.MinScore < 0 || cfg.RAG.MinScore > 1 {
		return nil, errors.New("RAG_MIN_SCORE must be between 0 and 1")
	}
	if cfg.RAG.ScorePercentile < 0 || cfg.RAG.ScorePercentile >= 1 {
		return nil, errors.New("RAG_SCORE_PERCENTILE must be between 0 and 1")
	}
	if cfg.RAG.MultiQueryCount < 0 {
		return nil, errors.New("RAG_MULTI_QUERY_COUNT must not be negative")
	}
	if cfg.RAG.RRFConstant < 0 {
		return nil, errors.New("RAG_RRF_K must not be negative")
	}
	if cfg.RAG.IntentFileLimit <= 0 {
		return nil, errors.New("RAG_INTENT_FILE_LIMIT must be greater than zero")
	}
	if cfg.OCR.Timeout <= 0 {
		return nil, errors.New("OCR_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.OCR.MinTextRunes < 0 {
		return nil, errors.New("OCR_MIN_TEXT_RUNES must not be negative")
	}
	if cfg.OCR.MaxImageBytes <= 0 {
		return nil, errors.New("OCR_MAX_IMAGE_BYTES must be greater than zero")
	}
	if cfg.Transcribe.Timeout <= 0 {
		return nil, errors.New("TRANSCRIBE_TIMEOUT_SECONDS must be greater than zero")
	}
	if cfg.Transcribe.MaxBytes <= 0 {
		return nil, errors.New("TRANSCRIBE_MAX_BYTES must be greater than zero")
	}
	if cfg.Video.FrameInterval <= 0 {
		return nil, errors.New("VIDEO_FRAME_INTERVAL_SECONDS must be greater than zero")
	}
	if cfg.Video.FrameLimit <= 0 {
		return nil, errors.New("VIDEO_FRAME_LIMIT must be greater than zero")
	}
	if cfg.Janitor.Interval <= 0 {
		return nil, errors.New("JANITOR_INTERVAL_SECONDS must be greater than zero")
	}
	if cfg.Janitor.MaxTaskAge <= 0 {
		return nil, errors.New("JANITOR_MAX_TASK_AGE_SECONDS must be greater than zero")
	}
	if cfg.Trash.RetentionDays < 0 {
		return nil, errors.New("TRASH_RETENTION_DAYS must not be negative")
	}
	if cfg.Auth.JWTSecret == defaultJWTSecret {
		log.Printf("level=warn component=config event=insecure_jwt_secret msg=\"JWT_SECRET is set to the default value — change it before deploying to production\"")
	}
	if cfg.Auth.Password == "" {
		log.Printf("level=warn component=config event=no_admin_password msg=\"ADMIN_PASSWORD is empty — the server is accessible without login\"")
	}
	return cfg, nil
}

// EnsureDirs creates all required storage directories if they do not exist.
func (c *Config) EnsureDirs() error {
	dirs := []string{
		c.Storage.Root,
		filepath.Dir(c.Storage.DBPath),
		c.Storage.TempDir,
		c.Storage.ThumbnailDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func databasePath(value string) string {
	if filepath.Ext(value) == ".db" || filepath.Ext(value) == ".sqlite" || filepath.Ext(value) == ".sqlite3" {
		return value
	}
	return filepath.Join(value, "memodrive.db")
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func envInt64(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat32(key string, fallback float32) float32 {
	value, err := strconv.ParseFloat(os.Getenv(key), 32)
	if err != nil {
		return fallback
	}
	return float32(value)
}
