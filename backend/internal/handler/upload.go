package handler

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

// UploadHandler manages chunked file upload sessions.
type UploadHandler struct {
	uploads *service.UploadService
}

// NewUploadHandler creates a new UploadHandler.
func NewUploadHandler(uploads *service.UploadService) *UploadHandler {
	return &UploadHandler{uploads: uploads}
}

func (h *UploadHandler) Register(router fiber.Router) {
	router.Post("/upload/init", h.init)
	router.Post("/upload/directory/prepare", h.prepareDirectory)
	router.Get("/upload/sessions", h.listSessions)
	router.Delete("/upload/sessions", h.clearSessions)
	router.Delete("/upload/sessions/:id", h.deleteSession)
	router.Get("/upload/:id", h.getSession)
	router.Delete("/upload/:id", h.cancel)
	router.Post("/upload/:id/complete", h.complete)
	router.Patch("/upload/:id", h.patch)
}

func (h *UploadHandler) prepareDirectory(c *fiber.Ctx) error {
	var input service.DirectoryPrepareInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid directory upload payload")
	}
	prepared, err := h.uploads.PrepareDirectory(c.Context(), input)
	if err != nil {
		return writeUploadError(c, err)
	}
	return c.JSON(prepared)
}

func (h *UploadHandler) init(c *fiber.Ctx) error {
	var input service.InitUploadInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid upload payload")
	}
	session, err := h.uploads.Init(c.Context(), input)
	if err != nil {
		return writeUploadError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(session)
}

func (h *UploadHandler) patch(c *fiber.Ctx) error {
	session, err := h.uploads.GetSession(c.Context(), c.Params("id"))
	if err != nil {
		return writeUploadError(c, err)
	}
	chunkIndex, err := chunkIndexFromRequest(c, session.ChunkSize)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	session, err = h.uploads.SaveChunk(c.Context(), c.Params("id"), chunkIndex, c.Body())
	if err != nil {
		return writeUploadError(c, err)
	}
	return c.JSON(fiber.Map{
		"upload_id":       session.ID,
		"uploaded_chunks": session.UploadedChunks,
	})
}

func (h *UploadHandler) complete(c *fiber.Ctx) error {
	completion, err := h.uploads.Complete(c.Context(), c.Params("id"))
	if err != nil {
		return writeUploadError(c, err)
	}
	return c.JSON(fiber.Map{
		"file":    completion.File,
		"task_id": completion.Task.ID,
	})
}

func (h *UploadHandler) getSession(c *fiber.Ctx) error {
	session, err := h.uploads.GetSession(c.Context(), c.Params("id"))
	if err != nil {
		return writeUploadError(c, err)
	}
	return c.JSON(session)
}

func (h *UploadHandler) cancel(c *fiber.Ctx) error {
	if err := h.uploads.CancelSession(c.Context(), c.Params("id")); err != nil {
		return writeUploadError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *UploadHandler) listSessions(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 100)
	sessions, err := h.uploads.ListSessions(c.Context(), limit)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"sessions": sessions})
}

func (h *UploadHandler) deleteSession(c *fiber.Ctx) error {
	if err := h.uploads.DeleteSession(c.Context(), c.Params("id")); err != nil {
		return writeUploadError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *UploadHandler) clearSessions(c *fiber.Ctx) error {
	count, err := h.uploads.ClearSessions(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"deleted": count})
}

func uploadError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fiber.NewError(fiber.StatusNotFound, "upload session not found")
	}
	return fiber.NewError(fiber.StatusBadRequest, err.Error())
}

func writeUploadError(c *fiber.Ctx, err error) error {
	var folderReplace *service.FolderReplaceUnsupportedError
	if errors.As(err, &folderReplace) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "folder_replace_unsupported",
				"message":   folderReplace.Error(),
				"retryable": false,
			},
		})
	}
	var copyLimit *service.FolderCopyLimitError
	if errors.As(err, &copyLimit) {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "copy_limit_exceeded",
				"message":   copyLimit.Error(),
				"retryable": false,
				"details": fiber.Map{
					"nodes":     copyLimit.Nodes,
					"max_nodes": copyLimit.MaxNodes,
				},
			},
		})
	}
	if errors.Is(err, store.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "upload_not_found",
				"message":   "upload session not found",
				"retryable": false,
				"details": fiber.Map{
					"upload_id": c.Params("id"),
				},
			},
		})
	}
	var conflict *service.FileConflictError
	if errors.As(err, &conflict) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "path_conflict",
				"message":   conflict.Error(),
				"retryable": false,
				"details": fiber.Map{
					"path":             conflict.Path,
					"name":             conflict.Name,
					"existing_file_id": conflict.ExistingFileID,
				},
			},
		})
	}
	var invalidPolicy *service.InvalidConflictPolicyError
	if errors.As(err, &invalidPolicy) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "invalid_conflict_policy",
				"message":   invalidPolicy.Error(),
				"retryable": false,
				"details": fiber.Map{
					"policy": invalidPolicy.Policy,
				},
			},
		})
	}
	var exhausted *service.NameExhaustedError
	if errors.As(err, &exhausted) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "name_exhausted",
				"message":   exhausted.Error(),
				"retryable": false,
				"details": fiber.Map{
					"path":         exhausted.Path,
					"name":         exhausted.Name,
					"max_attempts": exhausted.MaxAttempts,
				},
			},
		})
	}
	var missingParent *service.ParentFolderNotFoundError
	if errors.As(err, &missingParent) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "parent_not_found",
				"message":   missingParent.Error(),
				"retryable": false,
				"details": fiber.Map{
					"path": missingParent.Path,
				},
			},
		})
	}
	var invalidPath *service.InvalidFilePathError
	if errors.As(err, &invalidPath) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "invalid_path",
				"message":   invalidPath.Error(),
				"retryable": false,
				"details": fiber.Map{
					"path": invalidPath.Path,
					"name": invalidPath.Name,
				},
			},
		})
	}
	var tooLarge *service.FileTooLargeError
	if errors.As(err, &tooLarge) {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "file_too_large",
				"message":   tooLarge.Error(),
				"retryable": false,
				"details": fiber.Map{
					"file_size":     tooLarge.FileSize,
					"max_file_size": tooLarge.MaxFileSize,
				},
			},
		})
	}
	var tooManyDirectoryEntries *service.DirectoryTooManyEntriesError
	if errors.As(err, &tooManyDirectoryEntries) {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "directory_too_many_entries",
				"message":   tooManyDirectoryEntries.Error(),
				"retryable": false,
				"details": fiber.Map{
					"entry_count": tooManyDirectoryEntries.EntryCount,
					"max_entries": tooManyDirectoryEntries.MaxEntries,
				},
			},
		})
	}
	var insufficient *service.InsufficientStorageError
	if errors.As(err, &insufficient) {
		return c.Status(fiber.StatusInsufficientStorage).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "insufficient_storage",
				"message":   insufficient.Error(),
				"retryable": false,
				"details": fiber.Map{
					"constraint":      insufficient.Constraint,
					"required_bytes":  insufficient.RequiredBytes,
					"available_bytes": insufficient.AvailableBytes,
				},
			},
		})
	}
	var incomplete *service.UploadIncompleteError
	if errors.As(err, &incomplete) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "upload_incomplete",
				"message":   incomplete.Error(),
				"retryable": true,
				"details": fiber.Map{
					"uploaded_chunks": incomplete.UploadedChunks,
					"expected_chunks": incomplete.ExpectedChunks,
				},
			},
		})
	}
	var stateConflict *service.UploadStateConflictError
	if errors.As(err, &stateConflict) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": fiber.Map{
				"code":      "upload_state_conflict",
				"message":   stateConflict.Error(),
				"retryable": false,
				"details": fiber.Map{
					"status":    stateConflict.Status,
					"operation": stateConflict.Operation,
				},
			},
		})
	}
	return uploadError(err)
}

func chunkIndexFromRequest(c *fiber.Ctx, chunkSize int64) (int, error) {
	if value := c.Query("chunk"); value != "" {
		return strconv.Atoi(value)
	}
	if value := c.Get("Upload-Chunk-Index"); value != "" {
		return strconv.Atoi(value)
	}
	if value := c.Get("Upload-Offset"); value != "" {
		return service.ChunkIndexFromOffset(value, chunkSize)
	}
	return 0, nil
}
