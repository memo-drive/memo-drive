package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func TestUploadInitRejectsExistingTargetWithStructuredConflict(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	existing := &model.File{
		ID:          "existing-file",
		Name:        "Report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/Report.pdf",
		Size:        7,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		t.Fatalf("create existing File: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":128,"dest_path":"/Docs"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Path           string `json:"path"`
				Name           string `json:"name"`
				ExistingFileID string `json:"existing_file_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if body.Error.Code != "path_conflict" {
		t.Fatalf("expected path_conflict, got %q", body.Error.Code)
	}
	if body.Error.Message != "target already exists" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected conflict not to be retryable")
	}
	if body.Error.Details.Path != "/Docs" ||
		body.Error.Details.Name != "report.pdf" ||
		body.Error.Details.ExistingFileID != existing.ID {
		t.Fatalf("unexpected conflict details %#v", body.Error.Details)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/upload/sessions", nil)
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list upload sessions: %v", err)
	}
	defer listResp.Body.Close()
	var sessions struct {
		Sessions []model.UploadSession `json:"sessions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode upload sessions: %v", err)
	}
	if len(sessions.Sessions) != 0 {
		t.Fatalf("expected no upload session after conflict, got %d", len(sessions.Sessions))
	}
}

func TestDirectoryPrepareCreatesNestedUploadTarget(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	prepareReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{
				"client_id":"local-1",
				"relative_path":"Project/src/main.ts",
				"file_size":12
			}]
		}`),
	)
	prepareReq.Header.Set("Content-Type", "application/json")
	prepareResp, err := app.Test(prepareReq)
	if err != nil {
		t.Fatalf("prepare directory upload: %v", err)
	}
	defer prepareResp.Body.Close()
	if prepareResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(prepareResp.Body)
		t.Fatalf("expected 200, got %d: %s", prepareResp.StatusCode, body)
	}

	var prepared struct {
		BatchID string `json:"batch_id"`
		Entries []struct {
			ClientID     string `json:"client_id"`
			RelativePath string `json:"relative_path"`
			DestPath     string `json:"dest_path"`
			FileName     string `json:"file_name"`
			Status       string `json:"status"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(prepareResp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode prepared directory: %v", err)
	}
	if prepared.BatchID == "" {
		t.Fatal("expected directory upload batch id")
	}
	if len(prepared.Entries) != 1 {
		t.Fatalf("expected one prepared entry, got %d", len(prepared.Entries))
	}
	entry := prepared.Entries[0]
	if entry.ClientID != "local-1" || entry.RelativePath != "Project/src/main.ts" ||
		entry.DestPath != "/Docs/Project/src" || entry.FileName != "main.ts" ||
		entry.Status != "ready" {
		t.Fatalf("unexpected prepared entry %#v", entry)
	}

	for _, listedPath := range []string{"/Docs", "/Docs/Project"} {
		listReq := httptest.NewRequest(
			http.MethodGet,
			"/files?path="+listedPath+"&sort=name",
			nil,
		)
		listResp, err := app.Test(listReq)
		if err != nil {
			t.Fatalf("list %s: %v", listedPath, err)
		}
		var page struct {
			Files []model.File `json:"files"`
		}
		if err := json.NewDecoder(listResp.Body).Decode(&page); err != nil {
			_ = listResp.Body.Close()
			t.Fatalf("decode list %s: %v", listedPath, err)
		}
		_ = listResp.Body.Close()
		if len(page.Files) != 1 || !page.Files[0].IsDir {
			t.Fatalf("expected one nested Folder under %s, got %#v", listedPath, page.Files)
		}
	}

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{
			"file_name":"main.ts",
			"file_size":12,
			"dest_path":"/Docs/Project/src"
		}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init prepared upload: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(initResp.Body)
		t.Fatalf("expected prepared target to accept upload, got %d: %s", initResp.StatusCode, body)
	}
	var session model.UploadSession
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode prepared Upload Session: %v", err)
	}

	chunkReq := httptest.NewRequest(
		http.MethodPatch,
		"/upload/"+session.ID+"?chunk=0",
		bytes.NewBufferString("hello world!"),
	)
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload prepared chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk 200, got %d", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete prepared upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(completeResp.Body)
		t.Fatalf("expected complete 200, got %d: %s", completeResp.StatusCode, body)
	}
	var completed struct {
		File model.File `json:"file"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&completed); err != nil {
		t.Fatalf("decode completed upload: %v", err)
	}

	downloadReq := httptest.NewRequest(
		http.MethodGet,
		"/files/"+completed.File.ID+"/download",
		nil,
	)
	downloadResp, err := app.Test(downloadReq)
	if err != nil {
		t.Fatalf("download prepared File: %v", err)
	}
	defer downloadResp.Body.Close()
	downloaded, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		t.Fatalf("read prepared File: %v", err)
	}
	if string(downloaded) != "hello world!" {
		t.Fatalf("downloaded content = %q", downloaded)
	}
}

func TestDirectoryPrepareRejectsParentTraversalWithoutCreatingObjects(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{
				"client_id":"unsafe-1",
				"relative_path":"Project/../escape.txt",
				"file_size":12
			}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare unsafe directory upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected per-entry result, got %d: %s", resp.StatusCode, body)
	}

	var prepared struct {
		Entries []struct {
			Status string `json:"status"`
			Error  struct {
				Code      string `json:"code"`
				Retryable bool   `json:"retryable"`
				Details   struct {
					RelativePath string `json:"relative_path"`
					Reason       string `json:"reason"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode unsafe result: %v", err)
	}
	if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "failed" {
		t.Fatalf("expected one failed entry, got %#v", prepared.Entries)
	}
	entryError := prepared.Entries[0].Error
	if entryError.Code != "invalid_relative_path" || entryError.Retryable ||
		entryError.Details.RelativePath != "Project/../escape.txt" ||
		entryError.Details.Reason != "parent_segment" {
		t.Fatalf("unexpected relative path error %#v", entryError)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/files?path=/Docs&sort=name", nil)
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list Docs after rejected entry: %v", err)
	}
	defer listResp.Body.Close()
	var page struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&page); err != nil {
		t.Fatalf("decode Docs after rejected entry: %v", err)
	}
	if len(page.Files) != 0 {
		t.Fatalf("rejected relative path created objects: %#v", page.Files)
	}
}

func TestDirectoryPrepareRequiresPortableRelativePaths(t *testing.T) {
	cases := []struct {
		name         string
		relativePath string
		reason       string
	}{
		{name: "unix absolute", relativePath: "/Project/main.ts", reason: "absolute_path"},
		{name: "windows drive", relativePath: "C:/Project/main.ts", reason: "absolute_path"},
		{name: "backslash", relativePath: `Project\main.ts`, reason: "backslash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, _, cleanup := newUploadHandlerTestApp(t)
			defer cleanup()

			payload, err := json.Marshal(map[string]any{
				"dest_path": "/Docs",
				"entries": []map[string]any{{
					"client_id":     "unsafe",
					"relative_path": tc.relativePath,
					"file_size":     12,
				}},
			})
			if err != nil {
				t.Fatalf("encode request: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/upload/directory/prepare", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("prepare directory: %v", err)
			}
			defer resp.Body.Close()
			var prepared struct {
				Entries []struct {
					Status string `json:"status"`
					Error  struct {
						Code    string `json:"code"`
						Details struct {
							Reason string `json:"reason"`
						} `json:"details"`
					} `json:"error"`
				} `json:"entries"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "failed" ||
				prepared.Entries[0].Error.Code != "invalid_relative_path" ||
				prepared.Entries[0].Error.Details.Reason != tc.reason {
				t.Fatalf("unexpected result %#v", prepared.Entries)
			}
		})
	}
}

func TestDirectoryPrepareRejectsAmbiguousRelativePathSegments(t *testing.T) {
	cases := []struct {
		name         string
		relativePath string
		reason       string
	}{
		{name: "nul", relativePath: "Project/ma\x00in.ts", reason: "nul"},
		{name: "empty path", relativePath: "", reason: "empty_segment"},
		{name: "double slash", relativePath: "Project//main.ts", reason: "empty_segment"},
		{name: "trailing slash", relativePath: "Project/", reason: "empty_segment"},
		{name: "dot segment", relativePath: "Project/./main.ts", reason: "dot_segment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, _, cleanup := newUploadHandlerTestApp(t)
			defer cleanup()
			payload, err := json.Marshal(map[string]any{
				"dest_path": "/Docs",
				"entries": []map[string]any{{
					"client_id":     "unsafe",
					"relative_path": tc.relativePath,
					"file_size":     12,
				}},
			})
			if err != nil {
				t.Fatalf("encode request: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/upload/directory/prepare", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("prepare directory: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected per-entry result, got %d: %s", resp.StatusCode, body)
			}
			var prepared struct {
				Entries []struct {
					Status string `json:"status"`
					Error  struct {
						Code    string `json:"code"`
						Details struct {
							Reason string `json:"reason"`
						} `json:"details"`
					} `json:"error"`
				} `json:"entries"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "failed" ||
				prepared.Entries[0].Error.Code != "invalid_relative_path" ||
				prepared.Entries[0].Error.Details.Reason != tc.reason {
				t.Fatalf("unexpected result %#v", prepared.Entries)
			}
		})
	}
}

func TestDirectoryPrepareReusesExistingFolders(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	payload := `{
		"dest_path":"/Docs",
		"entries":[{
			"client_id":"local-1",
			"relative_path":"Project/src/main.ts",
			"file_size":12
		}]
	}`
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/upload/directory/prepare", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("prepare attempt %d: %v", attempt+1, err)
		}
		if attempt == 0 {
			_ = resp.Body.Close()
			continue
		}
		defer resp.Body.Close()
		var prepared struct {
			Folders []struct {
				RelativePath string `json:"relative_path"`
				Status       string `json:"status"`
			} `json:"folders"`
			Entries []struct {
				Status string `json:"status"`
			} `json:"entries"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
			t.Fatalf("decode second prepare: %v", err)
		}
		if len(prepared.Folders) != 2 ||
			prepared.Folders[0].RelativePath != "Project" || prepared.Folders[0].Status != "existing" ||
			prepared.Folders[1].RelativePath != "Project/src" || prepared.Folders[1].Status != "existing" {
			t.Fatalf("unexpected reused folders %#v", prepared.Folders)
		}
		if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "ready" {
			t.Fatalf("unexpected prepared entries %#v", prepared.Entries)
		}
	}
}

func TestDirectoryPrepareContinuesWhenFileBlocksOneFolderPath(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()
	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "blocking-file",
		Name:        "Blocked",
		Path:        "/Docs",
		StoragePath: "Docs/Blocked",
		Size:        7,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create blocking File: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[
				{"client_id":"blocked","relative_path":"Blocked/sub/a.txt","file_size":1},
				{"client_id":"ready","relative_path":"Safe/b.txt","file_size":1}
			]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare partial directory: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected partial result, got %d: %s", resp.StatusCode, body)
	}
	var prepared struct {
		Entries []struct {
			ClientID string `json:"client_id"`
			Status   string `json:"status"`
			DestPath string `json:"dest_path"`
			Error    struct {
				Code    string `json:"code"`
				Details struct {
					Reason         string `json:"reason"`
					ExistingFileID string `json:"existing_file_id"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode partial result: %v", err)
	}
	if len(prepared.Entries) != 2 {
		t.Fatalf("expected two entry results, got %#v", prepared.Entries)
	}
	blocked := prepared.Entries[0]
	if blocked.ClientID != "blocked" || blocked.Status != "failed" ||
		blocked.Error.Code != "path_conflict" || blocked.Error.Details.Reason != "file_blocks_folder" ||
		blocked.Error.Details.ExistingFileID != "blocking-file" {
		t.Fatalf("unexpected blocked result %#v", blocked)
	}
	ready := prepared.Entries[1]
	if ready.ClientID != "ready" || ready.Status != "ready" || ready.DestPath != "/Docs/Safe" {
		t.Fatalf("unexpected ready result %#v", ready)
	}
}

func TestDirectoryPrepareReportsLeafFileConflict(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()
	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "existing-main",
		Name:        "main.ts",
		Path:        "/Docs",
		StoragePath: "Docs/main.ts",
		Size:        7,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create existing File: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{
				"client_id":"local-1",
				"relative_path":"main.ts",
				"file_size":12
			}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare conflicting File: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Entries []struct {
			Status           string `json:"status"`
			Conflict         bool   `json:"conflict"`
			ExistingFileID   string `json:"existing_file_id"`
			RenameSuggestion string `json:"rename_suggestion"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode conflict result: %v", err)
	}
	if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "ready" ||
		!prepared.Entries[0].Conflict || prepared.Entries[0].ExistingFileID != "existing-main" ||
		prepared.Entries[0].RenameSuggestion != "main (1).ts" {
		t.Fatalf("unexpected conflict result %#v", prepared.Entries)
	}
}

func TestDirectoryPrepareDoesNotOfferFileReplaceForLeafFolder(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/folders",
		bytes.NewBufferString(`{"path":"/Docs","name":"main.ts"}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	if err != nil {
		t.Fatalf("create blocking Folder: %v", err)
	}
	defer createResp.Body.Close()
	var folder model.File
	if err := json.NewDecoder(createResp.Body).Decode(&folder); err != nil {
		t.Fatalf("decode blocking Folder: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{"client_id":"local-1","relative_path":"main.ts","file_size":12}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare leaf Folder conflict: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Entries []struct {
			Status   string `json:"status"`
			Conflict bool   `json:"conflict"`
			Error    struct {
				Code    string `json:"code"`
				Details struct {
					Reason         string `json:"reason"`
					ExistingFileID string `json:"existing_file_id"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode leaf Folder conflict: %v", err)
	}
	if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "failed" ||
		prepared.Entries[0].Conflict || prepared.Entries[0].Error.Code != "path_conflict" ||
		prepared.Entries[0].Error.Details.Reason != "folder_blocks_file" ||
		prepared.Entries[0].Error.Details.ExistingFileID != folder.ID {
		t.Fatalf("unexpected leaf Folder conflict %#v", prepared.Entries)
	}
}

func TestDirectoryPrepareRejectsCaseInsensitiveDuplicateTargets(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()
	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[
				{"client_id":"first","relative_path":"Project/Main.ts","file_size":1},
				{"client_id":"second","relative_path":"Project/main.ts","file_size":1}
			]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare duplicate targets: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Entries []struct {
			ClientID string `json:"client_id"`
			Status   string `json:"status"`
			Error    struct {
				Code    string `json:"code"`
				Details struct {
					Reason string `json:"reason"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode duplicate targets: %v", err)
	}
	if len(prepared.Entries) != 2 || prepared.Entries[0].ClientID != "first" ||
		prepared.Entries[0].Status != "ready" || prepared.Entries[1].ClientID != "second" ||
		prepared.Entries[1].Status != "failed" || prepared.Entries[1].Error.Code != "duplicate_relative_path" ||
		prepared.Entries[1].Error.Details.Reason != "case_insensitive_duplicate" {
		t.Fatalf("unexpected duplicate results %#v", prepared.Entries)
	}
}

func TestDirectoryPrepareRejectsPathBeyondConfiguredDepth(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.Storage.DirectoryMaxDepth = 2
	})
	defer cleanup()
	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{"client_id":"deep","relative_path":"A/B/file.txt","file_size":1}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare deep path: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Entries []struct {
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Details struct {
					Reason string `json:"reason"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode deep path result: %v", err)
	}
	if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "failed" ||
		prepared.Entries[0].Error.Code != "invalid_relative_path" ||
		prepared.Entries[0].Error.Details.Reason != "max_depth" {
		t.Fatalf("unexpected deep path result %#v", prepared.Entries)
	}
}

func TestDirectoryPrepareMeasuresConfiguredPathLimitInUTF8Bytes(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.Storage.DirectoryMaxPathBytes = 10
	})
	defer cleanup()
	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{"client_id":"long","relative_path":"目录/file.txt","file_size":1}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare long path: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Entries []struct {
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Details struct {
					Reason string `json:"reason"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode long path result: %v", err)
	}
	if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "failed" ||
		prepared.Entries[0].Error.Code != "invalid_relative_path" ||
		prepared.Entries[0].Error.Details.Reason != "max_path_bytes" {
		t.Fatalf("unexpected long path result %#v", prepared.Entries)
	}
}

func TestDirectoryPrepareRejectsBatchBeyondConfiguredEntryLimit(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.Storage.DirectoryMaxEntries = 1
	})
	defer cleanup()
	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[
				{"client_id":"1","relative_path":"A/one.txt","file_size":1},
				{"client_id":"2","relative_path":"B/two.txt","file_size":1}
			]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare oversized batch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 413, got %d: %s", resp.StatusCode, body)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				EntryCount int `json:"entry_count"`
				MaxEntries int `json:"max_entries"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode oversized batch: %v", err)
	}
	if body.Error.Code != "directory_too_many_entries" || body.Error.Retryable ||
		body.Error.Details.EntryCount != 2 || body.Error.Details.MaxEntries != 1 {
		t.Fatalf("unexpected oversized batch error %#v", body.Error)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/files?path=/Docs&sort=name", nil)
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list Docs: %v", err)
	}
	defer listResp.Body.Close()
	var page struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&page); err != nil {
		t.Fatalf("decode Docs: %v", err)
	}
	if len(page.Files) != 0 {
		t.Fatalf("oversized request created objects: %#v", page.Files)
	}
}

func TestDirectoryPrepareRequiresExistingDestinationFolder(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()
	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Missing",
			"entries":[{"client_id":"1","relative_path":"Project/a.txt","file_size":1}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare missing destination: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, body)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Path string `json:"path"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode missing destination: %v", err)
	}
	if body.Error.Code != "parent_not_found" || body.Error.Details.Path != "/Missing" {
		t.Fatalf("unexpected missing destination error %#v", body.Error)
	}
}

func TestDirectoryPrepareUsesCanonicalCaseOfExistingFolders(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/folders",
		bytes.NewBufferString(`{"path":"/Docs","name":"Project"}`),
	)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	if err != nil {
		t.Fatalf("create Project: %v", err)
	}
	_ = createResp.Body.Close()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[{"client_id":"1","relative_path":"project/src/a.txt","file_size":1}]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare canonical path: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Entries []struct {
			DestPath string `json:"dest_path"`
			Status   string `json:"status"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode canonical path: %v", err)
	}
	if len(prepared.Entries) != 1 || prepared.Entries[0].Status != "ready" ||
		prepared.Entries[0].DestPath != "/Docs/Project/src" {
		t.Fatalf("unexpected canonical target %#v", prepared.Entries)
	}
}

func TestDirectoryPrepareRejectsFilesOutsideUploadSizeBounds(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.Storage.MaxFileSize = 10
	})
	defer cleanup()
	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/directory/prepare",
		bytes.NewBufferString(`{
			"dest_path":"/Docs",
			"entries":[
				{"client_id":"empty","relative_path":"Empty/a.txt","file_size":0},
				{"client_id":"large","relative_path":"Large/b.txt","file_size":11}
			]
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("prepare invalid sizes: %v", err)
	}
	defer resp.Body.Close()
	var prepared struct {
		Folders []any `json:"folders"`
		Entries []struct {
			ClientID string `json:"client_id"`
			Status   string `json:"status"`
			Error    struct {
				Code    string `json:"code"`
				Details struct {
					Reason string `json:"reason"`
				} `json:"details"`
			} `json:"error"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepared); err != nil {
		t.Fatalf("decode invalid sizes: %v", err)
	}
	if len(prepared.Folders) != 0 || len(prepared.Entries) != 2 ||
		prepared.Entries[0].ClientID != "empty" || prepared.Entries[0].Status != "failed" ||
		prepared.Entries[0].Error.Code != "invalid_file_size" || prepared.Entries[0].Error.Details.Reason != "non_positive" ||
		prepared.Entries[1].ClientID != "large" || prepared.Entries[1].Status != "failed" ||
		prepared.Entries[1].Error.Code != "file_too_large" || prepared.Entries[1].Error.Details.Reason != "max_file_size" {
		t.Fatalf("unexpected invalid size results %#v", prepared)
	}
}

func TestUploadInitRenamePersistsResolvedTarget(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	existing := &model.File{
		ID:          "existing-file",
		Name:        "Report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/Report.pdf",
		Size:        7,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		t.Fatalf("create existing File: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":128,"dest_path":"/Docs","overwrite_policy":"rename"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	type uploadTarget struct {
		ID              string `json:"id"`
		RequestedName   string `json:"requested_name"`
		ResolvedName    string `json:"resolved_name"`
		OverwritePolicy string `json:"overwrite_policy"`
	}
	var created uploadTarget
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}
	if created.RequestedName != "report.pdf" {
		t.Fatalf("expected requested name report.pdf, got %q", created.RequestedName)
	}
	if created.ResolvedName != "report (1).pdf" {
		t.Fatalf("expected resolved name report (1).pdf, got %q", created.ResolvedName)
	}
	if created.OverwritePolicy != "rename" {
		t.Fatalf("expected rename policy, got %q", created.OverwritePolicy)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/upload/"+created.ID, nil)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected persisted session 200, got %d", getResp.StatusCode)
	}
	var persisted uploadTarget
	if err := json.NewDecoder(getResp.Body).Decode(&persisted); err != nil {
		t.Fatalf("decode persisted upload: %v", err)
	}
	if persisted != created {
		t.Fatalf("expected persisted target %#v, got %#v", created, persisted)
	}
}

func TestUploadCompleteRenameCreatesFileAtResolvedTarget(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "existing-file",
		Name:        "Report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/Report.pdf",
		Size:        7,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create existing File: %v", err)
	}

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":3,"dest_path":"/Docs","overwrite_policy":"rename"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk upload 200, got %d", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected complete 200, got %d", completeResp.StatusCode)
	}
	var completion struct {
		File model.File `json:"file"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&completion); err != nil {
		t.Fatalf("decode completed upload: %v", err)
	}
	if completion.File.Name != "report (1).pdf" {
		t.Fatalf("expected completed File name report (1).pdf, got %q", completion.File.Name)
	}
	if completion.File.Path != "/Docs" {
		t.Fatalf("expected completed File path /Docs, got %q", completion.File.Path)
	}
}

func TestUploadCompleteRenameRechecksTargetAfterConcurrentConflict(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	for _, file := range []*model.File{
		{
			ID:          "existing-file",
			Name:        "report.pdf",
			Path:        "/Docs",
			StoragePath: "Docs/report.pdf",
			Size:        7,
			MimeType:    "application/pdf",
			Status:      model.FileStatusReady,
		},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create existing File: %v", err)
		}
	}

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":3,"dest_path":"/Docs","overwrite_policy":"rename"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID           string `json:"id"`
		ResolvedName string `json:"resolved_name"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}
	if session.ResolvedName != "report (1).pdf" {
		t.Fatalf("expected initial resolved name report (1).pdf, got %q", session.ResolvedName)
	}

	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "concurrent-file",
		Name:        "report (1).pdf",
		Path:        "/Docs",
		StoragePath: "Docs/report (1).pdf",
		Size:        5,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create concurrent File: %v", err)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk upload 200, got %d", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected complete 200, got %d", completeResp.StatusCode)
	}
	var completion struct {
		File model.File `json:"file"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&completion); err != nil {
		t.Fatalf("decode completed upload: %v", err)
	}
	if completion.File.Name != "report (2).pdf" {
		t.Fatalf("expected completed File name report (2).pdf, got %q", completion.File.Name)
	}
}

func TestUploadInitReplacePersistsExistingFileTarget(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	existing := &model.File{
		ID:          "existing-file",
		Name:        "Report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/Report.pdf",
		Size:        7,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		t.Fatalf("create existing File: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":128,"dest_path":"/Docs","overwrite_policy":"replace"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var session struct {
		ID              string `json:"id"`
		RequestedName   string `json:"requested_name"`
		ResolvedName    string `json:"resolved_name"`
		OverwritePolicy string `json:"overwrite_policy"`
		ExistingFileID  string `json:"existing_file_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}
	if session.RequestedName != "report.pdf" || session.ResolvedName != "report.pdf" {
		t.Fatalf("expected replace to preserve requested target, got %#v", session)
	}
	if session.OverwritePolicy != "replace" {
		t.Fatalf("expected replace policy, got %q", session.OverwritePolicy)
	}
	if session.ExistingFileID != existing.ID {
		t.Fatalf("expected existing File %q, got %q", existing.ID, session.ExistingFileID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/upload/"+session.ID, nil)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	defer getResp.Body.Close()
	var persisted struct {
		OverwritePolicy string `json:"overwrite_policy"`
		ExistingFileID  string `json:"existing_file_id"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&persisted); err != nil {
		t.Fatalf("decode persisted upload: %v", err)
	}
	if persisted.OverwritePolicy != "replace" || persisted.ExistingFileID != existing.ID {
		t.Fatalf("unexpected persisted replace target %#v", persisted)
	}
}

func TestUploadInitReplaceRejectsFolderTarget(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	folder := &model.File{
		ID:          "existing-folder",
		Name:        "Reports",
		Path:        "/Docs",
		StoragePath: "Docs/Reports",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), folder); err != nil {
		t.Fatalf("create existing Folder: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"reports","file_size":128,"dest_path":"/Docs","overwrite_policy":"replace"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				ExistingFileID string `json:"existing_file_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if body.Error.Code != "path_conflict" || body.Error.Details.ExistingFileID != folder.ID {
		t.Fatalf("unexpected Folder conflict %#v", body.Error)
	}
}

func TestUploadInitRejectsUnknownConflictPolicy(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":128,"dest_path":"/Docs","overwrite_policy":"overwrite"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Policy string `json:"policy"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode invalid policy response: %v", err)
	}
	if body.Error.Code != "invalid_conflict_policy" {
		t.Fatalf("expected invalid_conflict_policy, got %q", body.Error.Code)
	}
	if body.Error.Message != "overwrite_policy must be reject, rename, or replace" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected invalid policy not to be retryable")
	}
	if body.Error.Details.Policy != "overwrite" {
		t.Fatalf("expected rejected policy overwrite, got %q", body.Error.Details.Policy)
	}
}

func TestUploadInitRejectsMissingParentFolder(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":128,"dest_path":"/Missing"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Path string `json:"path"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode parent error response: %v", err)
	}
	if body.Error.Code != "parent_not_found" {
		t.Fatalf("expected parent_not_found, got %q", body.Error.Code)
	}
	if body.Error.Message != "parent folder does not exist" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected missing parent not to be retryable")
	}
	if body.Error.Details.Path != "/Missing" {
		t.Fatalf("expected missing path /Missing, got %q", body.Error.Details.Path)
	}
}

func TestUploadCompleteRejectsTargetCreatedAfterInit(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":3,"dest_path":"/Docs"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	concurrent := &model.File{
		ID:          "concurrent-file",
		Name:        "Report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/Report.pdf",
		Size:        5,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), concurrent); err != nil {
		t.Fatalf("create concurrent File: %v", err)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk upload 200, got %d", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected complete 409, got %d", completeResp.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Path           string `json:"path"`
				Name           string `json:"name"`
				ExistingFileID string `json:"existing_file_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if body.Error.Code != "path_conflict" ||
		body.Error.Details.Path != "/Docs" ||
		body.Error.Details.Name != "report.pdf" ||
		body.Error.Details.ExistingFileID != concurrent.ID {
		t.Fatalf("unexpected completion conflict %#v", body.Error)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/upload/"+session.ID, nil)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	defer getResp.Body.Close()
	var persisted model.UploadSession
	if err := json.NewDecoder(getResp.Body).Decode(&persisted); err != nil {
		t.Fatalf("decode persisted upload: %v", err)
	}
	if persisted.Status != model.UploadStatusUploading {
		t.Fatalf("expected session to remain uploading, got %q", persisted.Status)
	}
}

func TestUploadInitRejectsInvalidNormalizedName(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"...","file_size":128,"dest_path":"/Docs"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Path string `json:"path"`
				Name string `json:"name"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode invalid path response: %v", err)
	}
	if body.Error.Code != "invalid_path" {
		t.Fatalf("expected invalid_path, got %q", body.Error.Code)
	}
	if body.Error.Message != "invalid file name" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected invalid path not to be retryable")
	}
	if body.Error.Details.Path != "/Docs" || body.Error.Details.Name != "..." {
		t.Fatalf("unexpected invalid path details %#v", body.Error.Details)
	}
}

func TestUploadInitRejectsFileLargerThanConfiguredLimit(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"large.bin","file_size":1048577,"dest_path":"/Docs"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				FileSize    int64 `json:"file_size"`
				MaxFileSize int64 `json:"max_file_size"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode file too large response: %v", err)
	}
	if body.Error.Code != "file_too_large" {
		t.Fatalf("expected file_too_large, got %q", body.Error.Code)
	}
	if body.Error.Message != "file exceeds maximum size" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected file too large not to be retryable")
	}
	if body.Error.Details.FileSize != 1048577 || body.Error.Details.MaxFileSize != 1048576 {
		t.Fatalf("unexpected size details %#v", body.Error.Details)
	}
}

func TestUploadInitRejectsInsufficientLogicalStorageWithStructured507(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestAppWithConfig(
		t,
		func(cfg *config.Config) {
			cfg.Storage.QuotaBytes = 100
			cfg.Storage.TempLimitBytes = 1000
		},
	)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"quota.bin","file_size":101,"dest_path":"/Docs"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInsufficientStorage {
		t.Fatalf("expected 507, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Constraint     string `json:"constraint"`
				RequiredBytes  int64  `json:"required_bytes"`
				AvailableBytes int64  `json:"available_bytes"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode insufficient storage response: %v", err)
	}
	if body.Error.Code != "insufficient_storage" || body.Error.Retryable {
		t.Fatalf("unexpected insufficient storage envelope %#v", body.Error)
	}
	if body.Error.Details.Constraint != "quota" ||
		body.Error.Details.RequiredBytes != 101 ||
		body.Error.Details.AvailableBytes != 100 {
		t.Fatalf("unexpected insufficient storage details %#v", body.Error.Details)
	}
}

func TestUploadCompleteReportsIncompleteChunks(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"partial.bin","file_size":65,"dest_path":"/Docs"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewReader(make([]byte, 64)))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk upload 200, got %d", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", completeResp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				UploadedChunks int `json:"uploaded_chunks"`
				ExpectedChunks int `json:"expected_chunks"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode incomplete upload response: %v", err)
	}
	if body.Error.Code != "upload_incomplete" {
		t.Fatalf("expected upload_incomplete, got %q", body.Error.Code)
	}
	if body.Error.Message != "upload chunks are incomplete" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if !body.Error.Retryable {
		t.Fatal("expected incomplete upload to be retryable")
	}
	if body.Error.Details.UploadedChunks != 1 || body.Error.Details.ExpectedChunks != 2 {
		t.Fatalf("unexpected chunk details %#v", body.Error.Details)
	}
}

func TestUploadCompleteReportsSessionStateConflict(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"cancelled.bin","file_size":3,"dest_path":"/Docs"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	cancelReq := httptest.NewRequest(http.MethodDelete, "/upload/"+session.ID, nil)
	cancelResp, err := app.Test(cancelReq)
	if err != nil {
		t.Fatalf("cancel upload: %v", err)
	}
	_ = cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected cancel 204, got %d", cancelResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", completeResp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Status    string `json:"status"`
				Operation string `json:"operation"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode state conflict response: %v", err)
	}
	if body.Error.Code != "upload_state_conflict" {
		t.Fatalf("expected upload_state_conflict, got %q", body.Error.Code)
	}
	if body.Error.Message != "upload session state does not allow complete" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected cancelled upload conflict not to be retryable")
	}
	if body.Error.Details.Status != model.UploadStatusCancelled ||
		body.Error.Details.Operation != "complete" {
		t.Fatalf("unexpected state conflict details %#v", body.Error.Details)
	}
}

func TestUploadInitRenameReportsNameExhaustedAfterMaximumAttempts(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	for sequence := 0; sequence <= 10000; sequence++ {
		name := "report.pdf"
		if sequence > 0 {
			name = fmt.Sprintf("report (%d).pdf", sequence)
		}
		if err := db.CreateFile(context.Background(), &model.File{
			ID:          fmt.Sprintf("occupied-%05d", sequence),
			Name:        name,
			Path:        "/Docs",
			StoragePath: "Docs/" + name,
			Size:        1,
			MimeType:    "application/pdf",
			Status:      model.FileStatusReady,
		}); err != nil {
			t.Fatalf("create occupied target %d: %v", sequence, err)
		}
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":128,"dest_path":"/Docs","overwrite_policy":"rename"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Path        string `json:"path"`
				Name        string `json:"name"`
				MaxAttempts int    `json:"max_attempts"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode name exhausted response: %v", err)
	}
	if body.Error.Code != "name_exhausted" {
		t.Fatalf("expected name_exhausted, got %q", body.Error.Code)
	}
	if body.Error.Message != "no available rename target" {
		t.Fatalf("unexpected error message %q", body.Error.Message)
	}
	if body.Error.Retryable {
		t.Fatal("expected exhausted name not to be retryable")
	}
	if body.Error.Details.Path != "/Docs" ||
		body.Error.Details.Name != "report.pdf" ||
		body.Error.Details.MaxAttempts != 10000 {
		t.Fatalf("unexpected name exhausted details %#v", body.Error.Details)
	}
}

func TestUploadCompleteReplaceKeepsFileIdentityAndPublishesNewContent(t *testing.T) {
	app, db, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	existing := &model.File{
		ID:          "existing-file",
		Name:        "report.pdf",
		Path:        "/Docs",
		StoragePath: "Docs/report.pdf",
		Size:        3,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		t.Fatalf("create existing File: %v", err)
	}

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.pdf","file_size":3,"dest_path":"/Docs","overwrite_policy":"replace"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk upload 200, got %d", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	defer completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", completeResp.StatusCode)
	}
	var body struct {
		File model.File `json:"file"`
	}
	if err := json.NewDecoder(completeResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode completed replace: %v", err)
	}
	if body.File.ID != existing.ID {
		t.Fatalf("expected replace to preserve File ID %q, got %q", existing.ID, body.File.ID)
	}
	if body.File.Name != existing.Name || body.File.Path != existing.Path || body.File.Size != 3 {
		t.Fatalf("unexpected replaced File %#v", body.File)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/files/"+existing.ID+"/download", nil)
	downloadResp, err := app.Test(downloadReq)
	if err != nil {
		t.Fatalf("download replaced File: %v", err)
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("expected replaced File download 200, got %d", downloadResp.StatusCode)
	}
	content, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		t.Fatalf("read replaced File: %v", err)
	}
	if string(content) != "new" {
		t.Fatalf("expected replaced content %q, got %q", "new", content)
	}
}

func TestUploadReplaceMakesPreviousContentDownloadableAsVersion(t *testing.T) {
	var storageRoot string
	app, db, cleanup := newUploadHandlerTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.FileVersion.Enabled = true
		storageRoot = cfg.Storage.Root
	})
	defer cleanup()

	existing := &model.File{
		ID:          "versioned-file",
		Name:        "report.txt",
		Path:        "/Docs",
		StoragePath: "Docs/report.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		t.Fatalf("create existing File: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storageRoot, "Docs"), 0o755); err != nil {
		t.Fatalf("create existing content Folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, filepath.FromSlash(existing.StoragePath)), []byte("old"), 0o644); err != nil {
		t.Fatalf("write existing content: %v", err)
	}

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"report.txt","file_size":3,"dest_path":"/Docs","overwrite_policy":"replace"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode upload session: %v", err)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload replacement chunk: %v", err)
	}
	_ = chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("upload replacement chunk status = %d, want 200", chunkResp.StatusCode)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		t.Fatalf("complete replacement: %v", err)
	}
	_ = completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		t.Fatalf("complete replacement status = %d, want 200", completeResp.StatusCode)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/files/"+existing.ID+"/versions", nil)
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list File Versions: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		t.Fatalf("list File Versions status = %d, want 200: %s", listResp.StatusCode, body)
	}
	var versions struct {
		Versions []struct {
			ID             string `json:"id"`
			FileID         string `json:"file_id"`
			VersionNo      int    `json:"version_no"`
			Size           int64  `json:"size"`
			Source         string `json:"source"`
			SHA256         string `json:"sha256"`
			ChecksumStatus string `json:"checksum_status"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&versions); err != nil {
		t.Fatalf("decode File Versions: %v", err)
	}
	if len(versions.Versions) != 1 {
		t.Fatalf("File Version count = %d, want 1", len(versions.Versions))
	}
	version := versions.Versions[0]
	if version.FileID != existing.ID || version.VersionNo != 1 || version.Size != 3 ||
		version.Source != "upload_replace" || len(version.SHA256) != 64 || version.ChecksumStatus != "recorded" {
		t.Fatalf("unexpected File Version %#v", version)
	}

	downloadReq := httptest.NewRequest(
		http.MethodGet,
		"/files/"+existing.ID+"/versions/"+version.ID+"/download",
		nil,
	)
	downloadResp, err := app.Test(downloadReq)
	if err != nil {
		t.Fatalf("download File Version: %v", err)
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(downloadResp.Body)
		t.Fatalf("download File Version status = %d, want 200: %s", downloadResp.StatusCode, body)
	}
	content, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		t.Fatalf("read File Version download: %v", err)
	}
	if string(content) != "old" {
		t.Fatalf("File Version content = %q, want old", content)
	}
}

func TestRestoreFileVersionArchivesCurrentContentBeforeRestoring(t *testing.T) {
	app, fileID, _, cleanup := newVersionedUploadReplaceTestApp(t)
	defer cleanup()

	listReq := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/versions", nil)
	listResp, err := app.Test(listReq)
	if err != nil {
		t.Fatalf("list File Versions before restore: %v", err)
	}
	var before struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&before); err != nil {
		_ = listResp.Body.Close()
		t.Fatalf("decode File Versions before restore: %v", err)
	}
	_ = listResp.Body.Close()
	if len(before.Versions) != 1 {
		t.Fatalf("File Version count before restore = %d, want 1", len(before.Versions))
	}

	restoreReq := httptest.NewRequest(
		http.MethodPost,
		"/files/"+fileID+"/versions/"+before.Versions[0].ID+"/restore",
		nil,
	)
	restoreResp, err := app.Test(restoreReq)
	if err != nil {
		t.Fatalf("restore File Version: %v", err)
	}
	defer restoreResp.Body.Close()
	if restoreResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(restoreResp.Body)
		t.Fatalf("restore File Version status = %d, want 200: %s", restoreResp.StatusCode, body)
	}
	var restored struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(restoreResp.Body).Decode(&restored); err != nil {
		t.Fatalf("decode restored File Version response: %v", err)
	}
	if restored.TaskID == "" {
		t.Fatal("restore File Version response has no Pipeline Task ID")
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/download", nil)
	currentResp, err := app.Test(currentReq)
	if err != nil {
		t.Fatalf("download restored File: %v", err)
	}
	currentContent, err := io.ReadAll(currentResp.Body)
	_ = currentResp.Body.Close()
	if err != nil {
		t.Fatalf("read restored File: %v", err)
	}
	if string(currentContent) != "old" {
		t.Fatalf("restored File content = %q, want old", currentContent)
	}

	listAfterReq := httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/versions", nil)
	listAfterResp, err := app.Test(listAfterReq)
	if err != nil {
		t.Fatalf("list File Versions after restore: %v", err)
	}
	defer listAfterResp.Body.Close()
	var after struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listAfterResp.Body).Decode(&after); err != nil {
		t.Fatalf("decode File Versions after restore: %v", err)
	}
	if len(after.Versions) != 2 || after.Versions[0].VersionNo != 2 ||
		after.Versions[0].Source != "version_restore" {
		t.Fatalf("unexpected File Versions after restore %#v", after.Versions)
	}

	archivedReq := httptest.NewRequest(
		http.MethodGet,
		"/files/"+fileID+"/versions/"+after.Versions[0].ID+"/download",
		nil,
	)
	archivedResp, err := app.Test(archivedReq)
	if err != nil {
		t.Fatalf("download content archived by restore: %v", err)
	}
	archivedContent, err := io.ReadAll(archivedResp.Body)
	_ = archivedResp.Body.Close()
	if err != nil {
		t.Fatalf("read content archived by restore: %v", err)
	}
	if string(archivedContent) != "new" {
		t.Fatalf("content archived by restore = %q, want new", archivedContent)
	}
}

func TestDeleteFileVersionRemovesItFromListAndDownload(t *testing.T) {
	app, fileID, _, cleanup := newVersionedUploadReplaceTestApp(t)
	defer cleanup()
	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/versions", nil))
	if err != nil {
		t.Fatalf("list File Versions: %v", err)
	}
	var listed struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		_ = listResp.Body.Close()
		t.Fatalf("decode File Versions: %v", err)
	}
	_ = listResp.Body.Close()
	if len(listed.Versions) != 1 {
		t.Fatalf("File Version count = %d, want 1", len(listed.Versions))
	}
	versionURL := "/files/" + fileID + "/versions/" + listed.Versions[0].ID
	deleteResp, err := app.Test(httptest.NewRequest(http.MethodDelete, versionURL, nil))
	if err != nil {
		t.Fatalf("delete File Version: %v", err)
	}
	_ = deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete File Version status = %d, want 204", deleteResp.StatusCode)
	}

	listAfterResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/versions", nil))
	if err != nil {
		t.Fatalf("list File Versions after delete: %v", err)
	}
	defer listAfterResp.Body.Close()
	var after struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listAfterResp.Body).Decode(&after); err != nil {
		t.Fatalf("decode File Versions after delete: %v", err)
	}
	if len(after.Versions) != 0 {
		t.Fatalf("File Versions after delete = %#v, want empty", after.Versions)
	}

	downloadResp, err := app.Test(httptest.NewRequest(http.MethodGet, versionURL+"/download", nil))
	if err != nil {
		t.Fatalf("download deleted File Version: %v", err)
	}
	_ = downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusNotFound {
		t.Fatalf("download deleted File Version status = %d, want 404", downloadResp.StatusCode)
	}
}

func TestTrashPreservesFileVersionsUntilFileIsPurged(t *testing.T) {
	app, fileID, _, cleanup := newVersionedUploadReplaceTestApp(t)
	defer cleanup()
	versionBytes := func() int64 {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/storage/usage", nil))
		if err != nil {
			t.Fatalf("read storage usage: %v", err)
		}
		defer resp.Body.Close()
		var usage struct {
			VersionBytes int64 `json:"version_bytes"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
			t.Fatalf("decode storage usage: %v", err)
		}
		return usage.VersionBytes
	}
	if got := versionBytes(); got != 3 {
		t.Fatalf("version bytes before Trash = %d, want 3", got)
	}
	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/versions", nil))
	if err != nil {
		t.Fatalf("list File Versions: %v", err)
	}
	var listed struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		_ = listResp.Body.Close()
		t.Fatalf("decode File Versions: %v", err)
	}
	_ = listResp.Body.Close()
	versionURL := "/files/" + fileID + "/versions/" + listed.Versions[0].ID + "/download"

	trashResp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/files/"+fileID, nil))
	if err != nil {
		t.Fatalf("move File to Trash: %v", err)
	}
	_ = trashResp.Body.Close()
	if trashResp.StatusCode != http.StatusNoContent {
		t.Fatalf("move File to Trash status = %d, want 204", trashResp.StatusCode)
	}
	if got := versionBytes(); got != 3 {
		t.Fatalf("version bytes in Trash = %d, want 3", got)
	}
	versionResp, err := app.Test(httptest.NewRequest(http.MethodGet, versionURL, nil))
	if err != nil {
		t.Fatalf("download File Version from Trash: %v", err)
	}
	versionContent, err := io.ReadAll(versionResp.Body)
	_ = versionResp.Body.Close()
	if err != nil {
		t.Fatalf("read File Version from Trash: %v", err)
	}
	if versionResp.StatusCode != http.StatusOK || string(versionContent) != "old" {
		t.Fatalf("File Version in Trash status/content = %d/%q, want 200/old", versionResp.StatusCode, versionContent)
	}

	purgeResp, err := app.Test(httptest.NewRequest(http.MethodDelete, "/trash/"+fileID, nil))
	if err != nil {
		t.Fatalf("purge File: %v", err)
	}
	_ = purgeResp.Body.Close()
	if purgeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("purge File status = %d, want 204", purgeResp.StatusCode)
	}
	if got := versionBytes(); got != 0 {
		t.Fatalf("version bytes after Purge = %d, want 0", got)
	}
	missingResp, err := app.Test(httptest.NewRequest(http.MethodGet, versionURL, nil))
	if err != nil {
		t.Fatalf("download purged File Version: %v", err)
	}
	_ = missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("download purged File Version status = %d, want 404", missingResp.StatusCode)
	}
}

func TestDisabledFileVersioningKeepsExistingVersionsReadableButRejectsRestore(t *testing.T) {
	app, fileID, cfg, cleanup := newVersionedUploadReplaceTestApp(t)
	defer cleanup()
	cfg.FileVersion.Enabled = false
	listResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files/"+fileID+"/versions", nil))
	if err != nil {
		t.Fatalf("list existing File Versions while disabled: %v", err)
	}
	var listed struct {
		Versions []model.FileVersion `json:"versions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		_ = listResp.Body.Close()
		t.Fatalf("decode existing File Versions while disabled: %v", err)
	}
	_ = listResp.Body.Close()
	if len(listed.Versions) != 1 {
		t.Fatalf("existing File Versions while disabled = %#v, want one", listed.Versions)
	}
	versionURL := "/files/" + fileID + "/versions/" + listed.Versions[0].ID
	downloadResp, err := app.Test(httptest.NewRequest(http.MethodGet, versionURL+"/download", nil))
	if err != nil {
		t.Fatalf("download existing File Version while disabled: %v", err)
	}
	_ = downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("download existing File Version while disabled status = %d, want 200", downloadResp.StatusCode)
	}
	restoreResp, err := app.Test(httptest.NewRequest(http.MethodPost, versionURL+"/restore", nil))
	if err != nil {
		t.Fatalf("restore existing File Version while disabled: %v", err)
	}
	defer restoreResp.Body.Close()
	if restoreResp.StatusCode != http.StatusConflict {
		t.Fatalf("restore existing File Version while disabled status = %d, want 409", restoreResp.StatusCode)
	}
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(restoreResp.Body).Decode(&failure); err != nil {
		t.Fatalf("decode disabled restore response: %v", err)
	}
	if failure.Error.Code != "file_versioning_disabled" {
		t.Fatalf("disabled restore error code = %q, want file_versioning_disabled", failure.Error.Code)
	}
}

func TestMissingFileVersionReturnsStructuredNotFound(t *testing.T) {
	app, fileID, _, cleanup := newVersionedUploadReplaceTestApp(t)
	defer cleanup()
	resp, err := app.Test(httptest.NewRequest(
		http.MethodGet,
		"/files/"+fileID+"/versions/missing-version/download",
		nil,
	))
	if err != nil {
		t.Fatalf("download missing File Version: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("download missing File Version status = %d, want 404", resp.StatusCode)
	}
	var failure struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&failure); err != nil {
		t.Fatalf("decode missing File Version response: %v", err)
	}
	if failure.Error.Code != "version_not_found" || failure.Error.Retryable {
		t.Fatalf("unexpected missing File Version error %#v", failure.Error)
	}
}

func TestUploadChunkReportsSessionStateConflict(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"cancelled.bin","file_size":3,"dest_path":"/Docs"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	cancelReq := httptest.NewRequest(http.MethodDelete, "/upload/"+session.ID, nil)
	cancelResp, err := app.Test(cancelReq)
	if err != nil {
		t.Fatalf("cancel upload: %v", err)
	}
	_ = cancelResp.Body.Close()

	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	defer chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", chunkResp.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Status    string `json:"status"`
				Operation string `json:"operation"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(chunkResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode state conflict response: %v", err)
	}
	if body.Error.Code != "upload_state_conflict" ||
		body.Error.Details.Status != model.UploadStatusCancelled ||
		body.Error.Details.Operation != "upload_chunk" {
		t.Fatalf("unexpected chunk state conflict %#v", body.Error)
	}
}

func TestUploadSessionLookupReportsStructuredNotFound(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/upload/missing-upload", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				UploadID string `json:"upload_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode not found response: %v", err)
	}
	if body.Error.Code != "upload_not_found" ||
		body.Error.Message != "upload session not found" ||
		body.Error.Retryable ||
		body.Error.Details.UploadID != "missing-upload" {
		t.Fatalf("unexpected upload not found response %#v", body.Error)
	}
}

func TestUploadCancelReportsSessionStateConflict(t *testing.T) {
	app, _, cleanup := newUploadHandlerTestApp(t)
	defer cleanup()

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"cancelled.bin","file_size":3,"dest_path":"/Docs"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		t.Fatalf("init upload request failed: %v", err)
	}
	defer initResp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		t.Fatalf("decode initialized upload: %v", err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		req := httptest.NewRequest(http.MethodDelete, "/upload/"+session.ID, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("cancel upload attempt %d: %v", attempt, err)
		}
		if attempt == 1 {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("expected first cancel 204, got %d", resp.StatusCode)
			}
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected second cancel 409, got %d", resp.StatusCode)
		}
		var body struct {
			Error struct {
				Code    string `json:"code"`
				Details struct {
					Status    string `json:"status"`
					Operation string `json:"operation"`
				} `json:"details"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode cancel conflict: %v", err)
		}
		if body.Error.Code != "upload_state_conflict" ||
			body.Error.Details.Status != model.UploadStatusCancelled ||
			body.Error.Details.Operation != "cancel" {
			t.Fatalf("unexpected cancel conflict %#v", body.Error)
		}
	}
}

func newUploadHandlerTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	return newUploadHandlerTestAppWithConfig(t, nil)
}

func newVersionedUploadReplaceTestApp(t *testing.T) (*fiber.App, string, *config.Config, func()) {
	t.Helper()
	var storageRoot string
	var appCfg *config.Config
	app, db, cleanup := newUploadHandlerTestAppWithConfig(t, func(cfg *config.Config) {
		cfg.FileVersion.Enabled = true
		storageRoot = cfg.Storage.Root
		appCfg = cfg
	})
	existing := &model.File{
		ID:          "version-restore-file",
		Name:        "restore.txt",
		Path:        "/Docs",
		StoragePath: "Docs/restore.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		cleanup()
		t.Fatalf("create existing File: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storageRoot, "Docs"), 0o755); err != nil {
		cleanup()
		t.Fatalf("create content Folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, filepath.FromSlash(existing.StoragePath)), []byte("old"), 0o644); err != nil {
		cleanup()
		t.Fatalf("write existing content: %v", err)
	}

	initReq := httptest.NewRequest(
		http.MethodPost,
		"/upload/init",
		bytes.NewBufferString(`{"file_name":"restore.txt","file_size":3,"dest_path":"/Docs","overwrite_policy":"replace"}`),
	)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := app.Test(initReq)
	if err != nil {
		cleanup()
		t.Fatalf("init replacement: %v", err)
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&session); err != nil {
		_ = initResp.Body.Close()
		cleanup()
		t.Fatalf("decode replacement session: %v", err)
	}
	_ = initResp.Body.Close()
	chunkReq := httptest.NewRequest(http.MethodPatch, "/upload/"+session.ID+"?chunk=0", bytes.NewBufferString("new"))
	chunkResp, err := app.Test(chunkReq)
	if err != nil {
		cleanup()
		t.Fatalf("upload replacement content: %v", err)
	}
	_ = chunkResp.Body.Close()
	completeReq := httptest.NewRequest(http.MethodPost, "/upload/"+session.ID+"/complete", nil)
	completeResp, err := app.Test(completeReq)
	if err != nil {
		cleanup()
		t.Fatalf("complete replacement: %v", err)
	}
	_ = completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusOK {
		cleanup()
		t.Fatalf("complete replacement status = %d, want 200", completeResp.StatusCode)
	}
	return app, existing.ID, appCfg, cleanup
}

func newUploadHandlerTestAppWithConfig(
	t *testing.T,
	configure func(*config.Config),
) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  1024 * 1024,
			ChunkSize:    64,
			UploadTTL:    time.Hour,
		},
	}
	if configure != nil {
		configure(cfg)
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "docs-folder",
		Name:        "Docs",
		Path:        "/",
		StoragePath: "Docs",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}); err != nil {
		_ = db.Close()
		t.Fatalf("create Docs Folder: %v", err)
	}
	files := service.NewFileService(cfg, db, nil)
	pipeline := service.NewPipelineService(cfg, db, nil, nil, nil, nil)
	uploads := service.NewUploadService(cfg, db, files, pipeline)
	app := fiber.New()
	NewFileHandler(files, nil).Register(app)
	NewTrashHandler(files).Register(app)
	NewStorageHandler(files).Register(app)
	NewUploadHandler(uploads).Register(app)
	return app, db, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pipeline.Shutdown(ctx)
		_ = db.Close()
	}
}
