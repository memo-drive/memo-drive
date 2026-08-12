package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

func TestTaskListReturnsPipelineTaskWithFileSummary(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{
		ID:          "report",
		Name:        "report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/report.pdf",
		Size:        2048,
		MimeType:    "application/pdf",
		Status:      model.FileStatusFailed,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	errorText := "document parse failed"
	if err := db.CreateTask(context.Background(), &model.Task{
		ID:         "task-1",
		FileID:     file.ID,
		Type:       "pipeline",
		Status:     model.TaskStatusFailed,
		Progress:   100,
		Error:      &errorText,
		RetryCount: 2,
	}); err != nil {
		t.Fatalf("create Task: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tasks", nil))
	if err != nil {
		t.Fatalf("GET /tasks: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /tasks status = %d, want 200: %s", resp.StatusCode, body)
	}
	var body struct {
		Items []struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			Progress   int    `json:"progress"`
			Error      string `json:"error"`
			RetryCount int    `json:"retry_count"`
			File       struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Path     string `json:"path"`
				Size     int64  `json:"size"`
				MimeType string `json:"mime_type"`
				Status   string `json:"status"`
			} `json:"file"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode Task list: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("Task count = %d, want 1", len(body.Items))
	}
	item := body.Items[0]
	if item.ID != "task-1" || item.Status != model.TaskStatusFailed || item.Progress != 100 ||
		item.Error != errorText || item.RetryCount != 2 {
		t.Fatalf("unexpected Task item: %#v", item)
	}
	if item.File.ID != file.ID || item.File.Name != file.Name || item.File.Path != file.Path ||
		item.File.Size != file.Size || item.File.MimeType != file.MimeType || item.File.Status != file.Status {
		t.Fatalf("unexpected File summary: %#v", item.File)
	}
	if body.NextCursor != "" || body.HasMore {
		t.Fatalf("unexpected pagination: cursor=%q has_more=%t", body.NextCursor, body.HasMore)
	}
}

func TestTaskListFiltersByStatusAndFile(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	for _, file := range []*model.File{
		{ID: "alpha", Name: "alpha.md", Path: "/", StoragePath: "alpha.md", Status: model.FileStatusFailed},
		{ID: "beta", Name: "beta.md", Path: "/", StoragePath: "beta.md", Status: model.FileStatusFailed},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create File %s: %v", file.ID, err)
		}
	}
	for _, task := range []*model.Task{
		{ID: "alpha-failed", FileID: "alpha", Type: "pipeline", Status: model.TaskStatusFailed},
		{ID: "alpha-done", FileID: "alpha", Type: "pipeline", Status: model.TaskStatusDone},
		{ID: "beta-failed", FileID: "beta", Type: "pipeline", Status: model.TaskStatusFailed},
	} {
		if err := db.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("create Task %s: %v", task.ID, err)
		}
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tasks?status=failed&file_id=alpha", nil))
	if err != nil {
		t.Fatalf("GET filtered /tasks: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET filtered /tasks status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Items []model.TaskListItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode filtered Task list: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != "alpha-failed" {
		t.Fatalf("filtered Tasks = %#v, want alpha-failed only", body.Items)
	}
}

func TestTaskListRejectsUnknownStatus(t *testing.T) {
	app, _, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tasks?status=waiting", nil))
	if err != nil {
		t.Fatalf("GET /tasks with unknown status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode invalid status response: %v", err)
	}
	if body.Error.Code != "invalid_task_status" || body.Error.Retryable {
		t.Fatalf("unexpected invalid status error: %#v", body.Error)
	}
}

func TestTaskListCursorRemainsStableWhileTasksUpdate(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{ID: "notes", Name: "notes.md", Path: "/", StoragePath: "notes.md", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		if err := db.CreateTask(context.Background(), &model.Task{ID: id, FileID: file.ID, Type: "pipeline", Status: model.TaskStatusDone}); err != nil {
			t.Fatalf("create Task %s: %v", id, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	first := getTaskListPage(t, app, "/tasks?limit=2")
	if got := taskItemIDs(first.Items); len(got) != 2 || got[0] != "task-3" || got[1] != "task-2" {
		t.Fatalf("first page IDs = %v, want [task-3 task-2]", got)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page pagination = %#v, want cursor and has_more", first)
	}
	if err := db.UpdateTask(context.Background(), "task-2", model.TaskStatusDone, 45, nil); err != nil {
		t.Fatalf("update first-page Task: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{ID: "task-4", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusDone}); err != nil {
		t.Fatalf("create newer Task: %v", err)
	}

	second := getTaskListPage(t, app, "/tasks?limit=2&cursor="+url.QueryEscape(first.NextCursor))
	if got := taskItemIDs(second.Items); len(got) != 1 || got[0] != "task-1" {
		t.Fatalf("second page IDs = %v, want [task-1] without duplicates or newer inserts", got)
	}
	if second.HasMore || second.NextCursor != "" {
		t.Fatalf("second page pagination = %#v, want exhausted", second)
	}
}

func TestTaskListRejectsInvalidCursor(t *testing.T) {
	app, _, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tasks?cursor=not-a-cursor", nil))
	if err != nil {
		t.Fatalf("GET /tasks with invalid cursor: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode invalid cursor response: %v", err)
	}
	if body.Error.Code != "invalid_cursor" || body.Error.Retryable {
		t.Fatalf("unexpected invalid cursor error: %#v", body.Error)
	}
}

func TestRetryFailedTaskCreatesLinkedTaskAndPreservesFailure(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{ID: "retry-file", Name: "retry.md", Path: "/", StoragePath: "retry.md", Status: model.FileStatusFailed}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	failure := "embedding provider unavailable"
	if err := db.CreateTask(context.Background(), &model.Task{
		ID:         "failed-task",
		FileID:     file.ID,
		Type:       "pipeline",
		Status:     model.TaskStatusFailed,
		Progress:   100,
		Error:      &failure,
		RetryCount: 2,
	}); err != nil {
		t.Fatalf("create failed Task: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/failed-task/retry", nil))
	if err != nil {
		t.Fatalf("POST retry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("retry status = %d, want 201: %s", resp.StatusCode, body)
	}
	var created struct {
		Task struct {
			ID            string `json:"id"`
			FileID        string `json:"file_id"`
			Status        string `json:"status"`
			RetryCount    int    `json:"retry_count"`
			RetryOfTaskID string `json:"retry_of_task_id"`
		} `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if created.Task.ID == "" || created.Task.ID == "failed-task" ||
		created.Task.FileID != file.ID || created.Task.Status != model.TaskStatusPending ||
		created.Task.RetryCount != 3 || created.Task.RetryOfTaskID != "failed-task" {
		t.Fatalf("unexpected retry Task: %#v", created.Task)
	}

	oldResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tasks/failed-task", nil))
	if err != nil {
		t.Fatalf("GET original Task: %v", err)
	}
	defer oldResp.Body.Close()
	var original model.Task
	if err := json.NewDecoder(oldResp.Body).Decode(&original); err != nil {
		t.Fatalf("decode original Task: %v", err)
	}
	if original.Status != model.TaskStatusFailed || original.RetryCount != 2 || original.Error == nil || *original.Error != failure {
		t.Fatalf("original Task changed after retry: %#v", original)
	}
}

func TestRetryRejectsTaskThatIsNotFailed(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{ID: "ready-file", Name: "ready.md", Path: "/", StoragePath: "ready.md", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{ID: "done-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusDone, Progress: 100}); err != nil {
		t.Fatalf("create done Task: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/done-task/retry", nil))
	if err != nil {
		t.Fatalf("POST retry done Task: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("retry status = %d, want 409", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				TaskID string `json:"task_id"`
				Status string `json:"status"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode state conflict: %v", err)
	}
	if body.Error.Code != "task_not_failed" || body.Error.Retryable ||
		body.Error.Details.TaskID != "done-task" || body.Error.Details.Status != model.TaskStatusDone {
		t.Fatalf("unexpected state conflict: %#v", body.Error)
	}
}

func TestRetryRejectsFileWithActiveTask(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{ID: "active-file", Name: "active.md", Path: "/", StoragePath: "active.md", Status: model.FileStatusProcessing}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	for _, task := range []*model.Task{
		{ID: "failed-attempt", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100},
		{ID: "active-attempt", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusProcessing, Progress: 30},
	} {
		if err := db.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("create Task %s: %v", task.ID, err)
		}
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/failed-attempt/retry", nil))
	if err != nil {
		t.Fatalf("POST retry with active Task: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("retry status = %d, want 409", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				FileID string `json:"file_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode active Task conflict: %v", err)
	}
	if body.Error.Code != "task_already_active" || body.Error.Retryable || body.Error.Details.FileID != file.ID {
		t.Fatalf("unexpected active Task conflict: %#v", body.Error)
	}
}

func TestRetryRejectsFileInTrash(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{ID: "trashed-file", Name: "trashed.md", Path: "/", StoragePath: "trashed.md", Status: model.FileStatusFailed}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{ID: "trashed-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100}); err != nil {
		t.Fatalf("create Task: %v", err)
	}
	if err := db.SoftDeleteFile(context.Background(), file.ID, file.ID); err != nil {
		t.Fatalf("move File to Trash: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/trashed-task/retry", nil))
	if err != nil {
		t.Fatalf("POST retry trashed File: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("retry status = %d, want 409", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				FileID string `json:"file_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode Trash conflict: %v", err)
	}
	if body.Error.Code != "file_in_trash" || body.Error.Retryable || body.Error.Details.FileID != file.ID {
		t.Fatalf("unexpected Trash conflict: %#v", body.Error)
	}
}

func TestRetryReturnsStructuredNotFoundForMissingTask(t *testing.T) {
	app, _, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/missing/retry", nil))
	if err != nil {
		t.Fatalf("POST retry missing Task: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("retry status = %d, want 404", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				TaskID string `json:"task_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode missing Task response: %v", err)
	}
	if body.Error.Code != "task_not_found" || body.Error.Retryable || body.Error.Details.TaskID != "missing" {
		t.Fatalf("unexpected missing Task error: %#v", body.Error)
	}
}

func TestRetryClearsIncompleteIndexArtifactsBeforeProcessing(t *testing.T) {
	vectors := &taskVectorStore{}
	app, db, cleanup := newTaskHandlerTestAppWithVector(t, vectors)
	defer cleanup()

	file := &model.File{
		ID:          "stale-file",
		Name:        "stale.bin",
		Path:        "/",
		StoragePath: "stale.bin",
		MimeType:    "application/x-unsupported",
		Status:      model.FileStatusFailed,
		ChunkCount:  2,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.UpsertChunks(context.Background(), []store.ChunkRow{
		{ID: indexing.ChunkID(file.ID, 0), FileID: file.ID, FileName: file.Name, ChunkIndex: 0, Text: "stale one"},
		{ID: indexing.ChunkID(file.ID, 1), FileID: file.ID, FileName: file.Name, ChunkIndex: 1, Text: "stale two"},
	}); err != nil {
		t.Fatalf("create stale Chunks: %v", err)
	}
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{FileID: file.ID, MetaJSON: `{"stale":true}`}); err != nil {
		t.Fatalf("create stale Metadata: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{ID: "stale-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100}); err != nil {
		t.Fatalf("create failed Task: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/stale-task/retry", nil))
	if err != nil {
		t.Fatalf("POST retry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("retry status = %d, want 201: %s", resp.StatusCode, body)
	}

	fileResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+file.ID, nil))
	if err != nil {
		t.Fatalf("GET retried File: %v", err)
	}
	defer fileResp.Body.Close()
	var current model.File
	if err := json.NewDecoder(fileResp.Body).Decode(&current); err != nil {
		t.Fatalf("decode retried File: %v", err)
	}
	if current.ChunkCount != 0 {
		t.Fatalf("File chunk_count = %d, want 0 before reprocessing", current.ChunkCount)
	}
	metadataResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+file.ID+"/metadata", nil))
	if err != nil {
		t.Fatalf("GET stale Metadata: %v", err)
	}
	defer metadataResp.Body.Close()
	if metadataResp.StatusCode != http.StatusNotFound {
		t.Fatalf("Metadata status = %d, want 404 after cleanup", metadataResp.StatusCode)
	}
	wantVectorIDs := []string{indexing.ChunkID(file.ID, 0), indexing.ChunkID(file.ID, 1)}
	if len(vectors.deletedIDs) != len(wantVectorIDs) || vectors.deletedIDs[0] != wantVectorIDs[0] || vectors.deletedIDs[1] != wantVectorIDs[1] {
		t.Fatalf("deleted vector IDs = %v, want %v", vectors.deletedIDs, wantVectorIDs)
	}
}

func TestTaskListRedactsAbsolutePathsFromErrors(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{ID: "private-file", Name: "private.md", Path: "/", StoragePath: "private.md", Status: model.FileStatusFailed}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	rawError := "open /Users/private/Documents/secret.md: permission denied"
	if err := db.CreateTask(context.Background(), &model.Task{
		ID: "private-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Error: &rawError,
	}); err != nil {
		t.Fatalf("create Task: %v", err)
	}

	page := getTaskListPage(t, app, "/tasks")
	if len(page.Items) != 1 || page.Items[0].Error == nil {
		t.Fatalf("Task list = %#v, want one visible error", page.Items)
	}
	publicError := *page.Items[0].Error
	if strings.Contains(publicError, "/Users/private") || !strings.Contains(publicError, "permission denied") {
		t.Fatalf("public error = %q, want redacted path and preserved reason", publicError)
	}
}

func TestRetryReturnsRetryableErrorWithoutBlockingWhenQueueIsFull(t *testing.T) {
	app, pipeline, provider, cleanup := newQueueFullTaskTestApp(t)
	defer cleanup()

	first, err := pipeline.Enqueue(context.Background(), provider.files[0])
	if err != nil {
		t.Fatalf("enqueue running Task: %v", err)
	}
	provider.waitForEmbed(t)
	if _, err := pipeline.Enqueue(context.Background(), provider.files[1]); err != nil {
		t.Fatalf("enqueue waiting Task: %v", err)
	}

	type result struct {
		response *http.Response
		err      error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/failed-for-capacity/retry", nil))
		done <- result{response: resp, err: err}
	}()

	var request result
	select {
	case request = <-done:
	case <-time.After(200 * time.Millisecond):
		provider.releaseEmbeds()
		request = <-done
		t.Fatalf("retry request blocked while worker queue was full; running Task was %s", first.ID)
	}
	defer request.response.Body.Close()
	if request.err != nil {
		t.Fatalf("POST retry with full queue: %v", request.err)
	}
	if request.response.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(request.response.Body)
		t.Fatalf("retry status = %d, want 503: %s", request.response.StatusCode, body)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.NewDecoder(request.response.Body).Decode(&body); err != nil {
		t.Fatalf("decode queue full response: %v", err)
	}
	if body.Error.Code != "pipeline_queue_full" || !body.Error.Retryable {
		t.Fatalf("unexpected queue full error: %#v", body.Error)
	}

	page := getTaskListPage(t, app, "/tasks?file_id=retry-capacity")
	for _, item := range page.Items {
		if item.Status == model.TaskStatusPending {
			t.Fatalf("queue-full retry left a pending Task: %#v", item.Task)
		}
	}
}

func TestConcurrentRetryCreatesOnlyOneActiveTask(t *testing.T) {
	vectors := newRetryBarrierVectorStore()
	app, db, cleanup := newTaskHandlerTestAppWithVector(t, vectors)
	defer cleanup()

	file := &model.File{
		ID: "concurrent-file", Name: "concurrent.bin", Path: "/", StoragePath: "concurrent.bin",
		MimeType: "application/x-unsupported", Status: model.FileStatusFailed, ChunkCount: 1,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{
		ID: "concurrent-failure", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100,
	}); err != nil {
		t.Fatalf("create failed Task: %v", err)
	}

	start := make(chan struct{})
	statuses := make(chan int, 2)
	for range 2 {
		go func() {
			<-start
			resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/concurrent-failure/retry", nil))
			if err != nil {
				statuses <- 0
				return
			}
			defer resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	close(start)
	got := []int{<-statuses, <-statuses}
	sort.Ints(got)
	want := []int{http.StatusCreated, http.StatusConflict}
	if got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("concurrent retry statuses = %v, want %v", got, want)
	}
}

func TestRetryEventuallyReturnsFileToReady(t *testing.T) {
	app, db, cleanup := newTaskHandlerTestApp(t)
	defer cleanup()

	file := &model.File{
		ID: "recoverable-file", Name: "recoverable.bin", Path: "/", StoragePath: "recoverable.bin",
		MimeType: "application/x-unsupported", Status: model.FileStatusFailed,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{
		ID: "recoverable-failure", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100,
	}); err != nil {
		t.Fatalf("create failed Task: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/recoverable-failure/retry", nil))
	if err != nil {
		t.Fatalf("POST retry: %v", err)
	}
	defer resp.Body.Close()
	var created struct {
		Task model.Task `json:"task"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		taskResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/tasks/"+created.Task.ID, nil))
		if err != nil {
			t.Fatalf("GET retry Task: %v", err)
		}
		var current model.Task
		decodeErr := json.NewDecoder(taskResp.Body).Decode(&current)
		_ = taskResp.Body.Close()
		if decodeErr != nil {
			t.Fatalf("decode retry Task: %v", decodeErr)
		}
		if current.Status == model.TaskStatusDone {
			break
		}
		if current.Status == model.TaskStatusFailed {
			t.Fatalf("retry Task failed: %#v", current)
		}
		if time.Now().After(deadline) {
			t.Fatalf("retry Task did not complete: %#v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}

	fileResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+file.ID, nil))
	if err != nil {
		t.Fatalf("GET retried File: %v", err)
	}
	defer fileResp.Body.Close()
	var currentFile model.File
	if err := json.NewDecoder(fileResp.Body).Decode(&currentFile); err != nil {
		t.Fatalf("decode retried File: %v", err)
	}
	if currentFile.Status != model.FileStatusReady {
		t.Fatalf("File status = %q, want ready", currentFile.Status)
	}
}

func TestRetryHidesSensitiveCleanupFailureDetails(t *testing.T) {
	vectors := &taskVectorStore{deleteErr: errors.New("read /Users/private/vector-cache: permission denied")}
	app, db, cleanup := newTaskHandlerTestAppWithVector(t, vectors)
	defer cleanup()

	file := &model.File{
		ID: "cleanup-error-file", Name: "cleanup.bin", Path: "/", StoragePath: "cleanup.bin",
		MimeType: "application/x-unsupported", Status: model.FileStatusFailed, ChunkCount: 1,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.CreateTask(context.Background(), &model.Task{
		ID: "cleanup-error-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100,
	}); err != nil {
		t.Fatalf("create failed Task: %v", err)
	}

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/tasks/cleanup-error-task/retry", nil))
	if err != nil {
		t.Fatalf("POST retry with cleanup failure: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("retry status = %d, want 500", resp.StatusCode)
	}
	encoded, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read retry error: %v", err)
	}
	if strings.Contains(string(encoded), "/Users/private") {
		t.Fatalf("retry error leaked an absolute path: %s", encoded)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatalf("decode retry cleanup error: %v", err)
	}
	if body.Error.Code != "task_retry_failed" || !body.Error.Retryable {
		t.Fatalf("unexpected retry cleanup error: %#v", body.Error)
	}
}

func getTaskListPage(t *testing.T, app *fiber.App, target string) model.TaskListPage {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, target, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status = %d, want 200: %s", target, resp.StatusCode, body)
	}
	var page model.TaskListPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode %s: %v", target, err)
	}
	return page
}

func taskItemIDs(items []model.TaskListItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func newTaskHandlerTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	return newTaskHandlerTestAppWithVector(t, nil)
}

func newTaskHandlerTestAppWithVector(t *testing.T, vectors vectordb.VectorStore) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         filepath.Join(root, "files"),
		DBPath:       filepath.Join(root, "db", "memodrive.db"),
		TempDir:      filepath.Join(root, "tmp"),
		ThumbnailDir: filepath.Join(root, "thumbs"),
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	pipeline := service.NewPipelineService(cfg, db, nil, vectors, nil, nil)
	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, vectors), nil).Register(app)
	NewTaskHandler(pipeline).Register(app)
	return app, db, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pipeline.Shutdown(ctx)
		_ = db.Close()
	}
}

type taskVectorStore struct {
	deletedIDs []string
	deleteErr  error
}

type retryBarrierVectorStore struct {
	mu      sync.Mutex
	arrived int
	release chan struct{}
	once    sync.Once
}

func newRetryBarrierVectorStore() *retryBarrierVectorStore {
	return &retryBarrierVectorStore{release: make(chan struct{})}
}

func (s *retryBarrierVectorStore) EnsureCollection(context.Context, string) error { return nil }
func (s *retryBarrierVectorStore) Upsert(context.Context, string, []string, [][]float32, []string, []map[string]any) error {
	return nil
}
func (s *retryBarrierVectorStore) Query(context.Context, string, []float32, int) (*vectordb.QueryResult, error) {
	return &vectordb.QueryResult{}, nil
}
func (s *retryBarrierVectorStore) Delete(context.Context, string, []string) error {
	s.mu.Lock()
	s.arrived++
	if s.arrived >= 2 {
		s.once.Do(func() { close(s.release) })
	}
	s.mu.Unlock()
	select {
	case <-s.release:
	case <-time.After(100 * time.Millisecond):
	}
	return nil
}

type blockingTaskEmbedProvider struct {
	started     chan struct{}
	releaseCh   chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
	files       []*model.File
}

func (p *blockingTaskEmbedProvider) Name() string { return "blocking-task-test" }
func (p *blockingTaskEmbedProvider) Chat(context.Context, []llm.Message) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (p *blockingTaskEmbedProvider) Complete(context.Context, []llm.Message) (string, error) {
	return "", nil
}
func (p *blockingTaskEmbedProvider) Embed(context.Context, []string) ([][]float32, error) {
	p.startedOnce.Do(func() { close(p.started) })
	<-p.releaseCh
	return [][]float32{{1}}, nil
}
func (p *blockingTaskEmbedProvider) waitForEmbed(t *testing.T) {
	t.Helper()
	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for running pipeline Task")
	}
}
func (p *blockingTaskEmbedProvider) releaseEmbeds() {
	p.releaseOnce.Do(func() { close(p.releaseCh) })
}

func newQueueFullTaskTestApp(t *testing.T) (*fiber.App, *service.PipelineService, *blockingTaskEmbedProvider, func()) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
		Pipeline: config.PipelineConfig{Workers: 1, ChunkSize: 64, ChunkOverlap: 8, EmbedBatchSize: 1},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	provider := &blockingTaskEmbedProvider{started: make(chan struct{}), releaseCh: make(chan struct{})}
	for index, id := range []string{"running-capacity", "waiting-capacity", "retry-capacity"} {
		file := &model.File{
			ID: id, Name: id + ".md", Path: "/", StoragePath: id + ".md", MimeType: "text/markdown", Status: model.FileStatusReady,
		}
		if index == 2 {
			file.Status = model.FileStatusFailed
		}
		if err := os.WriteFile(filepath.Join(cfg.Storage.Root, file.StoragePath), []byte("# Searchable\n\nEnough text for a pipeline chunk."), 0o644); err != nil {
			_ = db.Close()
			t.Fatalf("write test File: %v", err)
		}
		if err := db.CreateFile(context.Background(), file); err != nil {
			_ = db.Close()
			t.Fatalf("create File: %v", err)
		}
		provider.files = append(provider.files, file)
	}
	if err := db.CreateTask(context.Background(), &model.Task{
		ID: "failed-for-capacity", FileID: "retry-capacity", Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100,
	}); err != nil {
		_ = db.Close()
		t.Fatalf("create failed Task: %v", err)
	}
	pipeline := service.NewPipelineService(cfg, db, provider, &taskVectorStore{}, nil, nil)
	app := fiber.New()
	NewTaskHandler(pipeline).Register(app)
	return app, pipeline, provider, func() {
		provider.releaseEmbeds()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pipeline.Shutdown(ctx)
		_ = db.Close()
	}
}

func (s *taskVectorStore) EnsureCollection(context.Context, string) error { return nil }
func (s *taskVectorStore) Upsert(context.Context, string, []string, [][]float32, []string, []map[string]any) error {
	return nil
}
func (s *taskVectorStore) Query(context.Context, string, []float32, int) (*vectordb.QueryResult, error) {
	return &vectordb.QueryResult{}, nil
}
func (s *taskVectorStore) Delete(_ context.Context, _ string, ids []string) error {
	s.deletedIDs = append(s.deletedIDs, ids...)
	return s.deleteErr
}
