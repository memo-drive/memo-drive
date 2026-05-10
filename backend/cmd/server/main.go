package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/handler"
	"github.com/memodrive/backend/internal/llm"
	appmw "github.com/memodrive/backend/internal/middleware"
	"github.com/memodrive/backend/internal/parser"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.LUTC)
	log.Printf("level=info component=server event=startup_begin")

	ctx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("level=fatal component=config event=load_failed err=%q", err)
	}
	log.Printf("level=info component=config event=loaded host=%s port=%s storage_root=%q db_path=%q upload_chunk_size=%d pipeline_chunk_size=%d pipeline_chunk_overlap=%d pipeline_embed_batch_size=%d rag_top_k=%d rag_search_top_k=%d rag_max_context_chars=%d rag_min_score=%.3f ocr_enabled=%t transcribe_enabled=%t video_ocr=%t janitor_enabled=%t trash_auto_purge=%t trash_retention_days=%d",
		cfg.Server.Host, cfg.Server.Port, cfg.Storage.Root, cfg.Storage.DBPath, cfg.Storage.ChunkSize, cfg.Pipeline.ChunkSize, cfg.Pipeline.ChunkOverlap, cfg.Pipeline.EmbedBatchSize, cfg.RAG.TopK, cfg.RAG.SearchTopK, cfg.RAG.MaxContextChars, cfg.RAG.MinScore, cfg.OCR.Enabled, cfg.Transcribe.Enabled, cfg.Video.OCREnabled, cfg.Janitor.Enabled, cfg.Trash.AutoPurgeEnabled, cfg.Trash.RetentionDays)
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("level=fatal component=config event=directories_failed err=%q", err)
	}
	log.Printf("level=info component=config event=directories_ready storage_root=%q temp_dir=%q thumbnail_dir=%q", cfg.Storage.Root, cfg.Storage.TempDir, cfg.Storage.ThumbnailDir)
	db, err := store.Open(ctx, cfg)
	if err != nil {
		log.Fatalf("level=fatal component=store event=open_failed err=%q", err)
	}
	defer db.Close()
	log.Printf("level=info component=store event=opened db_path=%q", cfg.Storage.DBPath)

	llmProvider := llm.NewProvider(cfg.LLM)
	chromaClient := vectordb.NewChroma(cfg.LLM.ChromaBaseURL)
	chromaCtx, chromaCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := chromaClient.EnsureCollection(chromaCtx, vectordb.DefaultCollection); err != nil {
		log.Printf("level=warn component=vectordb event=default_collection_init_failed collection=%q err=%q", vectordb.DefaultCollection, err)
	} else {
		log.Printf("level=info component=vectordb event=default_collection_ready collection=%q base_url=%s", vectordb.DefaultCollection, chromaClient.BaseURL)
	}
	chromaCancel()

	ocrRunner := parser.NewOCRRunner(cfg.OCR)
	transcriber := parser.NewTranscriber(cfg.Transcribe)

	fileService := service.NewFileService(cfg, db, chromaClient)
	uploadService := service.NewUploadService(cfg, db, fileService)
	pipelineService := service.NewPipelineService(cfg, db, llmProvider, chromaClient, ocrRunner, transcriber)
	searchService := service.NewSearchService(cfg, db, llmProvider, chromaClient)
	ragService := service.NewRAGService(cfg, llmProvider, searchService)
	conversationService := service.NewConversationService(db)
	reconciler := service.NewReconciler(cfg, db, pipelineService, fileService)

	if cfg.Janitor.RecoverOnBoot {
		if err := reconciler.RecoverOnBoot(ctx); err != nil {
			log.Printf("level=warn component=reconciler event=recover_failed err=%q", err)
		}
	}

	app := fiber.New(fiber.Config{
		AppName:      "MemoDrive",
		ErrorHandler: errorHandler,
		BodyLimit:    int(cfg.Storage.ChunkSize + 1024*1024),
	})
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(appmw.CORS())

	api := app.Group("/api")
	handler.RegisterHealth(api)
	handler.NewAuthHandler(cfg).Register(api)

	protected := api.Group("", appmw.NewAuthMiddleware(cfg.Auth), appmw.RateLimit())
	handler.NewStorageHandler(fileService).Register(protected)
	handler.NewFileHandler(fileService, searchService).Register(protected)
	handler.NewTrashHandler(fileService).Register(protected)
	handler.NewUploadHandler(uploadService, pipelineService).Register(protected)
	handler.NewTaskHandler(pipelineService).Register(protected)
	handler.NewAIHandler(llmProvider, ragService, searchService, conversationService).Register(protected)
	handler.NewConversationHandler(conversationService).Register(protected)

	go cleanupUploads(ctx, uploadService)
	if cfg.Janitor.Enabled {
		go runReconcilerLoop(ctx, reconciler, cfg.Janitor.Interval)
	}

	go func() {
		addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
		log.Printf("level=info component=server event=listening addr=%s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("level=fatal component=server event=listen_failed addr=%s err=%q", addr, err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	log.Printf("level=info component=server event=shutdown_begin signal=%s", sig)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("level=error component=server event=shutdown_failed err=%q", err)
	} else {
		log.Printf("level=info component=server event=shutdown_complete")
	}
	stopBackground()
	if err := pipelineService.Shutdown(shutdownCtx); err != nil {
		log.Printf("level=warn component=pipeline event=shutdown_incomplete err=%q", err)
	} else {
		log.Printf("level=info component=pipeline event=shutdown_complete")
	}
}

func cleanupUploads(ctx context.Context, uploads *service.UploadService) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := uploads.CleanupExpired(ctx); err != nil {
				log.Printf("level=warn component=upload event=cleanup_failed err=%q", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func runReconcilerLoop(ctx context.Context, reconciler *service.Reconciler, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := reconciler.PeriodicSweep(ctx); err != nil {
				log.Printf("level=warn component=janitor event=sweep_failed err=%q", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "internal server error"
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		code = fiberErr.Code
		message = fiberErr.Message
	} else if err != nil {
		message = err.Error()
	}
	if code >= fiber.StatusInternalServerError {
		log.Printf("level=error component=http event=request_failed method=%s path=%s status=%d err=%q", c.Method(), c.Path(), code, err)
	} else if code >= fiber.StatusBadRequest {
		log.Printf("level=warn component=http event=request_rejected method=%s path=%s status=%d err=%q", c.Method(), c.Path(), code, err)
	}
	return c.Status(code).JSON(fiber.Map{
		"error": message,
	})
}
