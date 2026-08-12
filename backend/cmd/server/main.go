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
	"github.com/memodrive/backend/internal/maintenance"
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
	log.Printf("level=info component=config event=loaded app_env=%s host=%s port=%s storage_root=%q db_path=%q upload_chunk_size=%d storage_quota_bytes=%d storage_reserved_bytes=%d storage_temp_limit_bytes=%d directory_upload_max_entries=%d directory_upload_max_depth=%d directory_upload_max_path_bytes=%d pipeline_chunk_size=%d pipeline_chunk_overlap=%d pipeline_embed_batch_size=%d rag_top_k=%d rag_search_top_k=%d rag_max_context_chars=%d rag_min_score=%.3f ocr_enabled=%t transcribe_enabled=%t video_ocr=%t janitor_enabled=%t trash_auto_purge=%t trash_retention_days=%d file_versioning_enabled=%t file_version_max_count=%d file_version_retention_days=%d rate_limit_window_seconds=%d rate_limit_login=%d rate_limit_read=%d rate_limit_write=%d rate_limit_upload=%d rate_limit_ai=%d",
		cfg.AppEnv, cfg.Server.Host, cfg.Server.Port, cfg.Storage.Root, cfg.Storage.DBPath, cfg.Storage.ChunkSize, cfg.Storage.QuotaBytes, cfg.Storage.ReservedBytes, cfg.Storage.TempLimitBytes, cfg.Storage.DirectoryMaxEntries, cfg.Storage.DirectoryMaxDepth, cfg.Storage.DirectoryMaxPathBytes, cfg.Pipeline.ChunkSize, cfg.Pipeline.ChunkOverlap, cfg.Pipeline.EmbedBatchSize, cfg.RAG.TopK, cfg.RAG.SearchTopK, cfg.RAG.MaxContextChars, cfg.RAG.MinScore, cfg.OCR.Enabled, cfg.Transcribe.Enabled, cfg.Video.OCREnabled, cfg.Janitor.Enabled, cfg.Trash.AutoPurgeEnabled, cfg.Trash.RetentionDays, cfg.FileVersion.Enabled, cfg.FileVersion.MaxCount, cfg.FileVersion.RetentionDays, int(cfg.RateLimit.Window/time.Second), cfg.RateLimit.LoginFailures, cfg.RateLimit.ReadRequests, cfg.RateLimit.WriteRequests, cfg.RateLimit.UploadRequests, cfg.RateLimit.AIRequests)
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("level=fatal component=config event=directories_failed err=%q", err)
	}
	log.Printf("level=info component=config event=directories_ready storage_root=%q temp_dir=%q thumbnail_dir=%q", cfg.Storage.Root, cfg.Storage.TempDir, cfg.Storage.ThumbnailDir)
	writerLock, err := maintenance.AcquireWriterLock(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("level=fatal component=maintenance event=writer_lock_failed err=%q", err)
	}
	defer writerLock.Close()
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
	pipelineService := service.NewPipelineService(cfg, db, llmProvider, chromaClient, ocrRunner, transcriber)
	fileService.SetPipeline(pipelineService)
	webDAVService := service.NewWebDAVService(cfg, db, pipelineService)
	uploadService := service.NewUploadService(cfg, db, fileService, pipelineService)
	searchService := service.NewSearchService(cfg, db, llmProvider, chromaClient)
	ragService := service.NewRAGService(cfg, llmProvider, searchService)
	conversationService := service.NewConversationService(db)
	reconciler := service.NewReconciler(cfg, db, pipelineService, fileService)

	if cfg.Janitor.RecoverOnBoot {
		if err := reconciler.RecoverOnBoot(ctx); err != nil {
			log.Printf("level=warn component=reconciler event=recover_failed err=%q", err)
		}
	}

	app := fiber.New(httpConfig(cfg))
	app.Use(recover.New())
	app.Use(logger.New(httpLoggerConfig()))
	app.Use(appmw.CORS(cfg.CORS))
	app.Use("/api", appmw.BodyLimit(bufferedUploadBodyLimit(cfg)))
	handler.RegisterWebDAV(app, cfg, webDAVService)

	api := app.Group("/api")
	handler.RegisterHealth(api)
	rateLimiter := appmw.NewRateLimiter(cfg.RateLimit, cfg.Security.TrustedProxyCIDRs)
	api.Use("/auth/login", rateLimiter.LoginFailures())
	authHandler := handler.NewAuthHandler(cfg, db)
	authHandler.Register(api)

	protected := api.Group("", appmw.NewAuthMiddleware(cfg.Auth, db), rateLimiter.API())
	authHandler.RegisterProtected(protected)
	handler.NewStorageHandler(fileService).Register(protected)
	handler.NewFileHandler(fileService, searchService).Register(protected)
	handler.NewTrashHandler(fileService).Register(protected)
	handler.NewUploadHandler(uploadService).Register(protected)
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

func httpConfig(cfg *config.Config) fiber.Config {
	return fiber.Config{
		AppName:           "MemoDrive",
		ErrorHandler:      errorHandler,
		BodyLimit:         int(bufferedUploadBodyLimit(cfg)),
		StreamRequestBody: true,
		RequestMethods:    handler.WebDAVRequestMethods(fiber.DefaultMethods),
	}
}

func bufferedUploadBodyLimit(cfg *config.Config) int64 {
	return cfg.Storage.ChunkSize + 1024*1024
}

func httpLoggerConfig() logger.Config {
	return logger.Config{
		Next: func(c *fiber.Ctx) bool {
			return handler.IsWebDAVPath(c.Path()) && c.Method() == "PROPFIND"
		},
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
	if handler.IsWebDAVPath(c.Path()) {
		return c.SendStatus(code)
	}
	return c.Status(code).JSON(fiber.Map{
		"error": message,
	})
}
