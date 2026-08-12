package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memodrive/backend/internal/model"
)

func TestFileConflictPreflightReportsExistingTargetAndRenameSuggestion(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()

	for _, file := range []*model.File{
		{
			ID:          "docs-folder",
			Name:        "Docs",
			Path:        "/",
			StoragePath: "Docs",
			IsDir:       true,
			Status:      model.FileStatusReady,
		},
		{
			ID:          "existing-file",
			Name:        "A.pdf",
			Path:        "/Docs",
			StoragePath: "Docs/A.pdf",
			Size:        7,
			MimeType:    "application/pdf",
			Status:      model.FileStatusReady,
		},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create %s: %v", file.ID, err)
		}
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/files/conflicts",
		bytes.NewBufferString(`{"path":"/Docs","names":["a.pdf","b.pdf"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Items []struct {
			RequestedName    string `json:"requested_name"`
			NormalizedName   string `json:"normalized_name"`
			Conflict         bool   `json:"conflict"`
			ExistingFileID   string `json:"existing_file_id"`
			RenameSuggestion string `json:"rename_suggestion"`
			ReplaceAllowed   *bool  `json:"replace_allowed"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode preflight response: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 preflight items, got %d", len(body.Items))
	}
	if item := body.Items[0]; item.RequestedName != "a.pdf" ||
		item.NormalizedName != "a.pdf" ||
		!item.Conflict ||
		item.ExistingFileID != "existing-file" ||
		item.RenameSuggestion != "a (1).pdf" ||
		item.ReplaceAllowed == nil ||
		!*item.ReplaceAllowed {
		t.Fatalf("unexpected conflict item %#v", item)
	}
	if item := body.Items[1]; item.RequestedName != "b.pdf" ||
		item.NormalizedName != "b.pdf" ||
		item.Conflict ||
		item.ExistingFileID != "" ||
		item.RenameSuggestion != "" {
		t.Fatalf("unexpected available item %#v", item)
	}
}

func TestFileConflictPreflightDisallowsReplacingFolder(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()

	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "reports-folder",
		Name:        "Reports",
		Path:        "/",
		StoragePath: "Reports",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create Reports Folder: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/files/conflicts",
		bytes.NewBufferString(`{"path":"/","names":["Reports"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Items []struct {
			Conflict       bool  `json:"conflict"`
			ReplaceAllowed *bool `json:"replace_allowed"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode preflight response: %v", err)
	}
	if len(body.Items) != 1 || !body.Items[0].Conflict || body.Items[0].ReplaceAllowed == nil || *body.Items[0].ReplaceAllowed {
		t.Fatalf("unexpected Folder conflict item %#v", body.Items)
	}
}

func TestFileConflictPreflightDetectsCaseInsensitiveDuplicatesWithinBatch(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()

	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "docs-folder",
		Name:        "Docs",
		Path:        "/",
		StoragePath: "Docs",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create Docs Folder: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/files/conflicts",
		bytes.NewBufferString(`{"path":"/Docs","names":["a.pdf","A.PDF"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Items []struct {
			NormalizedName   string `json:"normalized_name"`
			Conflict         bool   `json:"conflict"`
			ExistingFileID   string `json:"existing_file_id"`
			RenameSuggestion string `json:"rename_suggestion"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode preflight response: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 preflight items, got %d", len(body.Items))
	}
	if body.Items[0].Conflict {
		t.Fatalf("expected first batch name to be available, got %#v", body.Items[0])
	}
	if item := body.Items[1]; !item.Conflict ||
		item.NormalizedName != "A.PDF" ||
		item.ExistingFileID != "" ||
		item.RenameSuggestion != "A (1).PDF" {
		t.Fatalf("unexpected batch conflict %#v", item)
	}
}

func TestFileConflictPreflightPreservesHiddenFileNameWhenSuggestingRename(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()

	for _, file := range []*model.File{
		{
			ID:          "docs-folder",
			Name:        "Docs",
			Path:        "/",
			StoragePath: "Docs",
			IsDir:       true,
			Status:      model.FileStatusReady,
		},
		{
			ID:          "env-file",
			Name:        ".env",
			Path:        "/Docs",
			StoragePath: "Docs/.env",
			Size:        7,
			MimeType:    "text/plain",
			Status:      model.FileStatusReady,
		},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create %s: %v", file.ID, err)
		}
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/files/conflicts",
		bytes.NewBufferString(`{"path":"/Docs","names":[".env"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Items []struct {
			NormalizedName   string `json:"normalized_name"`
			Conflict         bool   `json:"conflict"`
			RenameSuggestion string `json:"rename_suggestion"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode preflight response: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 preflight item, got %d", len(body.Items))
	}
	if item := body.Items[0]; item.NormalizedName != ".env" ||
		!item.Conflict ||
		item.RenameSuggestion != ".env (1)" {
		t.Fatalf("unexpected hidden File conflict %#v", item)
	}
}

func TestFileConflictPreflightRejectsInvalidNormalizedName(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()

	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "docs-folder",
		Name:        "Docs",
		Path:        "/",
		StoragePath: "Docs",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create Docs Folder: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/files/conflicts",
		bytes.NewBufferString(`{"path":"/Docs","names":["valid.pdf","..."]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Path string `json:"path"`
				Name string `json:"name"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode invalid path response: %v", err)
	}
	if body.Error.Code != "invalid_path" ||
		body.Error.Details.Path != "/Docs" ||
		body.Error.Details.Name != "..." {
		t.Fatalf("unexpected invalid path response %#v", body.Error)
	}
}

func TestFileConflictPreflightRenameSuggestionsPreserveFileNameShape(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()

	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "docs-folder",
		Name:        "Docs",
		Path:        "/",
		StoragePath: "Docs",
		IsDir:       true,
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create Docs Folder: %v", err)
	}
	names := []string{"archive.tar.gz", "报告📄.pdf", "README"}
	for index, name := range names {
		if err := db.CreateFile(context.Background(), &model.File{
			ID:          fmt.Sprintf("existing-%d", index),
			Name:        name,
			Path:        "/Docs",
			StoragePath: "Docs/" + name,
			Size:        1,
			MimeType:    "application/octet-stream",
			Status:      model.FileStatusReady,
		}); err != nil {
			t.Fatalf("create existing File %q: %v", name, err)
		}
	}

	payload, err := json.Marshal(map[string]any{
		"path":  "/Docs",
		"names": names,
	})
	if err != nil {
		t.Fatalf("encode preflight payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/files/conflicts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Items []struct {
			NormalizedName   string `json:"normalized_name"`
			RenameSuggestion string `json:"rename_suggestion"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode preflight response: %v", err)
	}
	expected := []string{"archive.tar (1).gz", "报告📄 (1).pdf", "README (1)"}
	if len(body.Items) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(body.Items))
	}
	for index, suggestion := range expected {
		if body.Items[index].NormalizedName != names[index] ||
			body.Items[index].RenameSuggestion != suggestion {
			t.Fatalf("item %d expected %q -> %q, got %#v", index, names[index], suggestion, body.Items[index])
		}
	}
}
