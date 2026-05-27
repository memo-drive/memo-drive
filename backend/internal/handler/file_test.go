package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func TestParseRangeSupportsPDFJSRanges(t *testing.T) {
	tests := []struct {
		name   string
		header string
		size   int64
		start  int64
		end    int64
	}{
		{name: "open ended", header: "bytes=100-", size: 1000, start: 100, end: 999},
		{name: "bounded", header: "bytes=100-199", size: 1000, start: 100, end: 199},
		{name: "suffix", header: "bytes=-128", size: 1000, start: 872, end: 999},
		{name: "suffix larger than file", header: "bytes=-2000", size: 1000, start: 0, end: 999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parseRange(tt.header, tt.size)
			if err != nil {
				t.Fatalf("parseRange returned error: %v", err)
			}
			if start != tt.start || end != tt.end {
				t.Fatalf("expected %d-%d, got %d-%d", tt.start, tt.end, start, end)
			}
		})
	}
}

func TestParseRangeRejectsInvalidRanges(t *testing.T) {
	for _, header := range []string{
		"bytes=100-50",
		"bytes=100-2000",
		"bytes=-0",
		"bytes=0-10,20-30",
		"items=0-10",
	} {
		if _, _, err := parseRange(header, 1000); err == nil {
			t.Fatalf("expected %q to be rejected", header)
		}
	}
}

func TestInlineDispositionUsesASCIIFallbackForUnicodeNames(t *testing.T) {
	header := inlineDisposition("美悦界一品牌管理 （上海）有限公司 _发票金额283.99元.pdf")
	if !strings.HasPrefix(header, "inline;") {
		t.Fatalf("expected inline disposition, got %q", header)
	}
	if !strings.Contains(header, `filename*=`) {
		t.Fatalf("expected RFC 5987 filename*, got %q", header)
	}
	for _, r := range header {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("header contains non-ASCII rune %q in %q", r, header)
		}
	}
}

func TestFileDownloadHeadersAndSuffixRange(t *testing.T) {
	app, cleanup := newFileDownloadTestApp(t, []byte("%PDF-1.7\nhello world\n%%EOF"))
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/files/file-1/download", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("expected application/pdf content type, got %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("expected inline disposition, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/files/file-1/download", nil)
	req.Header.Set("Range", "bytes=-5")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("range request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 21-25/26" {
		t.Fatalf("unexpected content range %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read range body: %v", err)
	}
	if string(body) != "%%EOF" {
		t.Fatalf("unexpected range body %q", body)
	}
}

func TestFileViewEndpointRecordsLastViewedAt(t *testing.T) {
	app, _, cleanup := newFileViewTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/files/file-1/view", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("view request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body model.File
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.LastViewedAt == nil {
		t.Fatal("expected last_viewed_at to be set")
	}
	if time.Since(*body.LastViewedAt) > time.Minute {
		t.Fatalf("expected recent last_viewed_at, got %s", body.LastViewedAt.Format(time.RFC3339Nano))
	}
}

func TestFileViewEndpointRefreshesLastViewedAt(t *testing.T) {
	app, _, cleanup := newFileViewTestApp(t)
	defer cleanup()

	first := markFileViewedViaHTTP(t, app, "file-1")
	time.Sleep(time.Millisecond)
	second := markFileViewedViaHTTP(t, app, "file-1")
	if !second.After(first) {
		t.Fatalf("expected second view time to be after first, first=%s second=%s", first.Format(time.RFC3339Nano), second.Format(time.RFC3339Nano))
	}
}

func TestReadOnlyFileEndpointsDoNotRecordLastViewedAt(t *testing.T) {
	app, _, cleanup := newFileViewTestApp(t)
	defer cleanup()

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/files/file-1", nil),
		httptest.NewRequest(http.MethodGet, "/files/file-1/download", nil),
		httptest.NewRequest(http.MethodGet, "/files/file-1/metadata", nil),
		httptest.NewRequest(http.MethodGet, "/files/file-1/thumbnail", nil),
		httptest.NewRequest(http.MethodPost, "/files/search", strings.NewReader(`{"query":"sample"}`)),
	}
	requests[len(requests)-1].Header.Set("Content-Type", "application/json")
	for _, req := range requests {
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", req.Method, req.URL.Path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("%s %s expected 2xx, got %d", req.Method, req.URL.Path, resp.StatusCode)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/files/file-1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("get file failed: %v", err)
	}
	defer resp.Body.Close()
	var body model.File
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.LastViewedAt != nil {
		t.Fatalf("expected read-only endpoints to leave last_viewed_at empty, got %s", body.LastViewedAt.Format(time.RFC3339Nano))
	}
}

func TestRecentFilesEndpointReturnsViewedFilesByLastViewedAt(t *testing.T) {
	app, db, cleanup := newFileViewTestApp(t)
	defer cleanup()
	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "file-2",
		Name:        "second.pdf",
		Path:        "/",
		StoragePath: "second.pdf",
		Size:        3,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create second file: %v", err)
	}

	markFileViewedViaHTTP(t, app, "file-1")
	time.Sleep(time.Millisecond)
	markFileViewedViaHTTP(t, app, "file-2")
	req := httptest.NewRequest(http.MethodGet, "/files/recent?limit=1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("recent request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Files) != 1 || body.Files[0].ID != "file-2" || body.Files[0].LastViewedAt == nil {
		t.Fatalf("expected newest viewed file only, got %#v", body.Files)
	}
}

func TestFileListEndpointDefaultsToNewestUploadsFirst(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()
	createHandlerTestFile(t, db, t.TempDir(), &model.File{
		ID:          "old-upload",
		Name:        "a-old.txt",
		Path:        "/",
		StoragePath: "a-old.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "old")
	time.Sleep(time.Millisecond)
	createHandlerTestFile(t, db, t.TempDir(), &model.File{
		ID:          "new-upload",
		Name:        "z-new.txt",
		Path:        "/",
		StoragePath: "z-new.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}, "new")

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/files", nil))
	if err != nil {
		t.Fatalf("GET /files: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := []string{}
	for _, file := range body.Files {
		got = append(got, file.ID)
	}
	want := []string{"new-upload", "old-upload"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected newest uploads first %v, got %v", want, got)
	}
}

func TestFileQueryEndpointReturnsUnifiedPageShape(t *testing.T) {
	app, _, cleanup := newFileQueryTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/files/query", strings.NewReader(`{
		"category":"photos",
		"query":"旅行",
		"sort":"created_at",
		"limit":1,
		"media_filter":"all",
		"document_subtype":"all"
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("file query request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	for _, key := range []string{"items", "next_cursor", "has_more"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("expected response to include %q, got %s", key, string(payload))
		}
	}
	var body struct {
		Items      []model.File `json:"items"`
		NextCursor string       `json:"next_cursor"`
		HasMore    bool         `json:"has_more"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != "photo-new" {
		t.Fatalf("expected newest matching photo only, got %#v", body.Items)
	}
	if !body.HasMore {
		t.Fatal("expected has_more to report the second matching photo")
	}
}

func TestFileQueryEndpointPaginatesWithCursor(t *testing.T) {
	app, db, cleanup := newFileQueryTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "photo-page-3", Name: "旅行-3.jpg", Path: "/Photos", StoragePath: "Photos/trip-3.jpg", Size: 14, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "photo-page-4", Name: "旅行-4.jpg", Path: "/Photos", StoragePath: "Photos/trip-4.jpg", Size: 16, MimeType: "image/jpeg", Status: model.FileStatusReady},
	} {
		time.Sleep(time.Millisecond)
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	first := queryFilesViaHTTP(t, app, `{"category":"photos","query":"旅行","sort":"created_at","limit":2}`)
	if got := fileIDs(first.Items); strings.Join(got, ",") != "photo-page-4,photo-page-3" {
		t.Fatalf("unexpected first page ids %v", got)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("expected first page cursor and has_more, got %#v", first)
	}

	second := queryFilesViaHTTP(t, app, `{"category":"photos","query":"旅行","sort":"created_at","limit":2,"cursor":`+strconv.Quote(first.NextCursor)+`}`)
	if got := fileIDs(second.Items); strings.Join(got, ",") != "photo-new,photo-old" {
		t.Fatalf("unexpected second page ids %v", got)
	}
	if second.HasMore || second.NextCursor != "" {
		t.Fatalf("expected second page to be terminal, got %#v", second)
	}
}

func TestFileQueryEndpointRejectsInvalidCursor(t *testing.T) {
	app, _, cleanup := newFileQueryTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/files/query", strings.NewReader(`{"category":"photos","limit":2,"cursor":"not-a-cursor"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("file query request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestFileQueryMediaCategoriesFallbackToExtension(t *testing.T) {
	app, db, cleanup := newFileQueryTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "photo-heic", Name: "IMG_0001.HEIC", Path: "/Photos", StoragePath: "Photos/IMG_0001.HEIC", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "video-mkv", Name: "clip.mkv", Path: "/Videos", StoragePath: "Videos/clip.mkv", Size: 20, MimeType: "application/octet-stream", Status: model.FileStatusReady},
		{ID: "audio-flac", Name: "song.flac", Path: "/Audio", StoragePath: "Audio/song.flac", Size: 30, MimeType: "", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	photos := queryFilesViaHTTP(t, app, `{"category":"photos","query":"IMG_0001","limit":10}`)
	if got := fileIDs(photos.Items); strings.Join(got, ",") != "photo-heic" {
		t.Fatalf("expected HEIC photo fallback hit, got %v", got)
	}
	videos := queryFilesViaHTTP(t, app, `{"category":"videos","query":"clip","limit":10}`)
	if got := fileIDs(videos.Items); strings.Join(got, ",") != "video-mkv" {
		t.Fatalf("expected MKV video fallback hit, got %v", got)
	}
	audio := queryFilesViaHTTP(t, app, `{"category":"audio","query":"song","limit":10}`)
	if got := fileIDs(audio.Items); strings.Join(got, ",") != "audio-flac" {
		t.Fatalf("expected FLAC audio fallback hit, got %v", got)
	}
}

func TestFileQueryDocumentsCategoryUsesMimeAndExtensionRules(t *testing.T) {
	app, db, cleanup := newFileQueryTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "doc-pdf", Name: "fallback-report.PDF", Path: "/Docs", StoragePath: "Docs/fallback-report.PDF", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "doc-md", Name: "fallback-notes.md", Path: "/Docs", StoragePath: "Docs/fallback-notes.md", Size: 10, MimeType: "application/octet-stream", Status: model.FileStatusReady},
		{ID: "doc-json", Name: "fallback-data.json", Path: "/Docs", StoragePath: "Docs/fallback-data.json", Size: 10, MimeType: "application/octet-stream", Status: model.FileStatusReady},
		{ID: "doc-csv", Name: "fallback-table.csv", Path: "/Docs", StoragePath: "Docs/fallback-table.csv", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "doc-xlsx", Name: "fallback-sheet.xlsx", Path: "/Docs", StoragePath: "Docs/fallback-sheet.xlsx", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "doc-pptx", Name: "fallback-slides.pptx", Path: "/Docs", StoragePath: "Docs/fallback-slides.pptx", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "doc-text-mime", Name: "fallback-source.go", Path: "/Docs", StoragePath: "Docs/fallback-source.go", Size: 10, MimeType: "text/x-go", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	docs := queryFilesViaHTTP(t, app, `{"category":"documents","query":"fallback-","limit":20}`)
	got := map[string]bool{}
	for _, id := range fileIDs(docs.Items) {
		got[id] = true
	}
	for _, id := range []string{"doc-pdf", "doc-md", "doc-json", "doc-csv", "doc-xlsx", "doc-pptx", "doc-text-mime"} {
		if !got[id] {
			t.Fatalf("expected document category to include %s, got %v", id, fileIDs(docs.Items))
		}
	}
}

func TestFileQueryDocumentsCanFilterBySubtype(t *testing.T) {
	app, db, cleanup := newFileQueryTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "subtype-pdf", Name: "subtype-contract.pdf", Path: "/Docs", StoragePath: "Docs/subtype-contract.pdf", Size: 10, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "subtype-text", Name: "subtype-brief.docx", Path: "/Docs", StoragePath: "Docs/subtype-brief.docx", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "subtype-spreadsheet", Name: "subtype-budget.xlsx", Path: "/Docs", StoragePath: "Docs/subtype-budget.xlsx", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "subtype-presentation", Name: "subtype-roadshow.pptx", Path: "/Docs", StoragePath: "Docs/subtype-roadshow.pptx", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "subtype-txt", Name: "subtype-notes.md", Path: "/Docs", StoragePath: "Docs/subtype-notes.md", Size: 10, MimeType: "text/markdown", Status: model.FileStatusReady},
		{ID: "subtype-other", Name: "subtype-legacy.rtf", Path: "/Docs", StoragePath: "Docs/subtype-legacy.rtf", Size: 10, MimeType: "application/rtf", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	tests := []struct {
		subtype string
		want    string
	}{
		{subtype: "pdf", want: "subtype-pdf"},
		{subtype: "text", want: "subtype-text"},
		{subtype: "spreadsheet", want: "subtype-spreadsheet"},
		{subtype: "presentation", want: "subtype-presentation"},
		{subtype: "txt", want: "subtype-txt"},
		{subtype: "other", want: "subtype-other"},
	}
	seen := map[string]string{}
	for _, tt := range tests {
		page := queryFilesViaHTTP(t, app, `{"category":"documents","query":"subtype-","document_subtype":"`+tt.subtype+`","limit":20}`)
		ids := fileIDs(page.Items)
		if strings.Join(ids, ",") != tt.want {
			t.Fatalf("expected subtype %s to return %s, got %v", tt.subtype, tt.want, ids)
		}
		if previous, ok := seen[tt.want]; ok {
			t.Fatalf("file %s appeared in both %s and %s", tt.want, previous, tt.subtype)
		}
		seen[tt.want] = tt.subtype
	}

	all := queryFilesViaHTTP(t, app, `{"category":"documents","query":"subtype-","document_subtype":"all","limit":20}`)
	if len(all.Items) != len(tests) {
		t.Fatalf("expected all subtype query to include %d files, got %v", len(tests), fileIDs(all.Items))
	}
}

func TestFileQueryVideosCanFilterByDuration(t *testing.T) {
	app, db, cleanup := newFileQueryTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "video-short", Name: "duration-filter-short.mp4", Path: "/Videos", StoragePath: "Videos/short.mp4", Size: 10, MimeType: "video/mp4", Status: model.FileStatusReady},
		{ID: "video-medium", Name: "duration-filter-medium.mp4", Path: "/Videos", StoragePath: "Videos/medium.mp4", Size: 10, MimeType: "video/mp4", Status: model.FileStatusReady},
		{ID: "video-long", Name: "duration-filter-long.mp4", Path: "/Videos", StoragePath: "Videos/long.mp4", Size: 10, MimeType: "video/mp4", Status: model.FileStatusReady},
		{ID: "video-unknown", Name: "duration-filter-unknown.mp4", Path: "/Videos", StoragePath: "Videos/unknown.mp4", Size: 10, MimeType: "video/mp4", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	for _, meta := range []*model.FileMetadata{
		{FileID: "video-short", MetaJSON: `{"duration":59.5}`},
		{FileID: "video-medium", MetaJSON: `{"duration":600}`},
		{FileID: "video-long", MetaJSON: `{"duration":601}`},
		{FileID: "video-unknown", MetaJSON: `{}`},
	} {
		if err := db.UpsertMetadata(context.Background(), meta); err != nil {
			t.Fatalf("upsert metadata for %s: %v", meta.FileID, err)
		}
	}

	short := queryFilesViaHTTP(t, app, `{"category":"videos","query":"duration-filter-","media_filter":"lt_1m","limit":10}`)
	if got := fileIDs(short.Items); strings.Join(got, ",") != "video-short" {
		t.Fatalf("expected short video only, got %v", got)
	}
	medium := queryFilesViaHTTP(t, app, `{"category":"videos","query":"duration-filter-","media_filter":"1_10m","limit":10}`)
	if got := fileIDs(medium.Items); strings.Join(got, ",") != "video-medium" {
		t.Fatalf("expected medium video only, got %v", got)
	}
	long := queryFilesViaHTTP(t, app, `{"category":"videos","query":"duration-filter-","media_filter":"gt_10m","limit":10}`)
	if got := fileIDs(long.Items); strings.Join(got, ",") != "video-long" {
		t.Fatalf("expected long video only, got %v", got)
	}
	all := queryFilesViaHTTP(t, app, `{"category":"videos","query":"duration-filter-","media_filter":"all","limit":10}`)
	if len(all.Items) != 4 {
		t.Fatalf("expected all videos including unknown duration, got %v", fileIDs(all.Items))
	}
}

func TestPhotoMonthIndexGroupsByTakenAtWithCreatedAtFallback(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "photo-jan-a", Name: "jan-a.jpg", Path: "/Photos", StoragePath: "Photos/jan-a.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "photo-jan-b", Name: "jan-b.heic", Path: "/Photos", StoragePath: "Photos/jan-b.heic", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "photo-dec", Name: "dec.jpg", Path: "/Photos", StoragePath: "Photos/dec.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "photo-fallback", Name: "fallback.jpg", Path: "/Photos", StoragePath: "Photos/fallback.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "video-ignored", Name: "ignored.mp4", Path: "/Videos", StoragePath: "Videos/ignored.mp4", Size: 10, MimeType: "video/mp4", Status: model.FileStatusReady},
		{ID: "photo-trashed", Name: "trashed.jpg", Path: "/Photos", StoragePath: "Photos/trashed.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	for _, meta := range []*model.FileMetadata{
		{FileID: "photo-jan-a", MetaJSON: `{"taken_at":"2025-01-05T10:00:00Z"}`},
		{FileID: "photo-jan-b", MetaJSON: `{"taken_at":"2025-01-20T10:00:00Z"}`},
		{FileID: "photo-dec", MetaJSON: `{"taken_at":"2025-12-01T10:00:00Z"}`},
		{FileID: "video-ignored", MetaJSON: `{"taken_at":"2026-12-01T10:00:00Z"}`},
		{FileID: "photo-trashed", MetaJSON: `{"taken_at":"2027-12-01T10:00:00Z"}`},
	} {
		if err := db.UpsertMetadata(context.Background(), meta); err != nil {
			t.Fatalf("upsert metadata for %s: %v", meta.FileID, err)
		}
	}
	if err := db.SoftDeleteFile(context.Background(), "photo-trashed", "photo-trashed"); err != nil {
		t.Fatalf("soft delete trashed photo: %v", err)
	}
	fallback, err := db.GetFile(context.Background(), "photo-fallback")
	if err != nil {
		t.Fatalf("get fallback photo: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/files/photos/months", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("photo months request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var body service.PhotoMonthIndexResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []service.PhotoMonthIndexItem{
		{Year: fallback.CreatedAt.UTC().Year(), Month: int(fallback.CreatedAt.UTC().Month()), Count: 1},
		{Year: 2025, Month: 12, Count: 1},
		{Year: 2025, Month: 1, Count: 2},
	}
	if len(body.Months) != len(want) {
		t.Fatalf("expected months %#v, got %#v", want, body.Months)
	}
	for i := range want {
		if body.Months[i] != want[i] {
			t.Fatalf("expected month[%d] %#v, got %#v", i, want[i], body.Months[i])
		}
	}
}

func TestPhotoTimelineEndpointPaginatesMonthByTakenAtWithFallback(t *testing.T) {
	app, db, cleanup := newEmptyFileHandlerTestApp(t)
	defer cleanup()
	for _, file := range []*model.File{
		{ID: "timeline-newer", Name: "timeline-newer.jpg", Path: "/Photos", StoragePath: "Photos/timeline-newer.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "timeline-fallback", Name: "timeline-fallback.jpg", Path: "/Photos", StoragePath: "Photos/timeline-fallback.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "timeline-older", Name: "timeline-older.heic", Path: "/Photos", StoragePath: "Photos/timeline-older.heic", Size: 10, MimeType: "", Status: model.FileStatusReady},
		{ID: "timeline-other-month", Name: "timeline-other-month.jpg", Path: "/Photos", StoragePath: "Photos/timeline-other-month.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "timeline-video", Name: "timeline-video.mp4", Path: "/Videos", StoragePath: "Videos/timeline-video.mp4", Size: 10, MimeType: "video/mp4", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	fallback, err := db.GetFile(context.Background(), "timeline-fallback")
	if err != nil {
		t.Fatalf("get fallback photo: %v", err)
	}
	monthStart := time.Date(fallback.CreatedAt.UTC().Year(), fallback.CreatedAt.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)
	thumb := "timeline-newer.jpg"
	for _, meta := range []*model.FileMetadata{
		{FileID: "timeline-newer", MetaJSON: `{"taken_at":` + strconv.Quote(monthEnd.Format(time.RFC3339)) + `}`, ThumbnailPath: &thumb},
		{FileID: "timeline-older", MetaJSON: `{"taken_at":` + strconv.Quote(monthStart.Add(time.Second).Format(time.RFC3339)) + `}`},
		{FileID: "timeline-other-month", MetaJSON: `{"taken_at":` + strconv.Quote(monthStart.AddDate(0, -1, 0).Format(time.RFC3339)) + `}`},
		{FileID: "timeline-video", MetaJSON: `{"taken_at":` + strconv.Quote(monthEnd.Format(time.RFC3339)) + `}`},
	} {
		if err := db.UpsertMetadata(context.Background(), meta); err != nil {
			t.Fatalf("upsert metadata for %s: %v", meta.FileID, err)
		}
	}

	first := queryPhotoTimelineViaHTTP(t, app, `{
		"year":`+strconv.Itoa(fallback.CreatedAt.UTC().Year())+`,
		"month":`+strconv.Itoa(int(fallback.CreatedAt.UTC().Month()))+`,
		"query":"timeline-",
		"sort":"unsupported-sort",
		"limit":2
	}`)
	if got := fileIDs(first.Items); strings.Join(got, ",") != "timeline-newer,timeline-fallback" {
		t.Fatalf("unexpected first timeline page ids %v", got)
	}
	if first.Items[0].Metadata == nil || first.Items[0].Metadata.ThumbnailPath == nil || *first.Items[0].Metadata.ThumbnailPath != thumb {
		t.Fatalf("expected timeline photo to include thumbnail metadata, got %#v", first.Items[0].Metadata)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("expected first timeline page cursor and has_more, got %#v", first)
	}

	second := queryPhotoTimelineViaHTTP(t, app, `{
		"year":`+strconv.Itoa(fallback.CreatedAt.UTC().Year())+`,
		"month":`+strconv.Itoa(int(fallback.CreatedAt.UTC().Month()))+`,
		"query":"timeline-",
		"sort":"unsupported-sort",
		"limit":2,
		"cursor":`+strconv.Quote(first.NextCursor)+`
	}`)
	if got := fileIDs(second.Items); strings.Join(got, ",") != "timeline-older" {
		t.Fatalf("unexpected second timeline page ids %v", got)
	}
	if second.HasMore || second.NextCursor != "" {
		t.Fatalf("expected second timeline page to be terminal, got %#v", second)
	}
}

func TestBatchDeleteEndpointSoftDeletesFilesAndDirectories(t *testing.T) {
	app, _, cleanup := newBatchFileActionTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/files/batch/delete", strings.NewReader(`{"file_ids":["active","dir-b","missing"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("batch delete request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 3 || body.Succeeded != 2 || body.Failed != 1 {
		t.Fatalf("unexpected batch summary: %#v", body)
	}
	for _, id := range []string{"active", "dir-b", "foo", "sub", "bar"} {
		req := httptest.NewRequest(http.MethodGet, "/files/"+id, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("get %s after delete failed: %v", id, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected %s to be hidden from active files, got %d", id, resp.StatusCode)
		}
	}
}

func TestBatchMoveEndpointMovesFilesAndDirectories(t *testing.T) {
	app, _, cleanup := newBatchFileActionTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/files/batch/move", strings.NewReader(`{"file_ids":["active","dir-b","missing"],"path":"/Target"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("batch move request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 3 || body.Succeeded != 2 || body.Failed != 1 {
		t.Fatalf("unexpected batch summary: %#v", body)
	}
	assertFilePathViaHTTP(t, app, "active", "/Target")
	assertFilePathViaHTTP(t, app, "dir-b", "/Target")
	assertFilePathViaHTTP(t, app, "foo", "/Target/B")
	assertFilePathViaHTTP(t, app, "sub", "/Target/B")
	assertFilePathViaHTTP(t, app, "bar", "/Target/B/sub")
}

func TestBatchMoveEndpointSummarizesConflicts(t *testing.T) {
	app, db, cleanup := newBatchFileActionTestApp(t)
	defer cleanup()
	if err := db.CreateFile(context.Background(), &model.File{
		ID:          "target-conflict",
		Name:        "active.txt",
		Path:        "/Target",
		StoragePath: "Target/active.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}); err != nil {
		t.Fatalf("create target conflict: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/files/batch/move", strings.NewReader(`{"file_ids":["active","dir-b","missing"],"path":"/Target"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("batch move request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Total     int `json:"total"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 3 || body.Succeeded != 1 || body.Failed != 2 {
		t.Fatalf("unexpected batch summary: %#v", body)
	}
	assertFilePathViaHTTP(t, app, "active", "/")
	assertFilePathViaHTTP(t, app, "dir-b", "/Target")

	req = httptest.NewRequest(http.MethodPost, "/files/batch/move", strings.NewReader(`{"file_ids":["dir-b"],"path":"/Target/B/sub"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("self-descendant batch move request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode self-descendant response: %v", err)
	}
	if body.Total != 1 || body.Succeeded != 0 || body.Failed != 1 {
		t.Fatalf("unexpected self-descendant summary: %#v", body)
	}
	assertFilePathViaHTTP(t, app, "dir-b", "/Target")
}

func TestRenameMoveRejectsEmptyPayload(t *testing.T) {
	app, cleanup := newFileRenameTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/files/file-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("rename request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRenameMoveMapsConflictTo409(t *testing.T) {
	app, cleanup := newFileRenameTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/files/file-1", strings.NewReader(`{"name":"existing.pdf"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("rename request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestCreateFolderMapsCaseInsensitiveConflictTo409(t *testing.T) {
	app, cleanup := newFileRenameTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/folders", strings.NewReader(`{"path":"/","name":"Notes"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected first folder create to return 201, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodPost, "/folders", strings.NewReader(`{"path":"/","name":"notes"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("create duplicate folder: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected case-insensitive folder conflict to map to 409, got %d", resp.StatusCode)
	}
}

func TestStorageUsageEndpointReturnsUsage(t *testing.T) {
	app, cleanup := newStorageUsageTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/storage/usage", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("storage usage request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		UsedBytes  int64 `json:"used_bytes"`
		TotalBytes int64 `json:"total_bytes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.UsedBytes != 12 {
		t.Fatalf("expected used bytes 12, got %d", body.UsedBytes)
	}
	if body.TotalBytes <= 0 {
		t.Fatalf("expected positive total bytes, got %d", body.TotalBytes)
	}
}

func newFileDownloadTestApp(t *testing.T, content []byte) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, "sample.pdf"), content, 0o644); err != nil {
		t.Fatalf("write sample pdf: %v", err)
	}

	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	file := &model.File{
		ID:          "file-1",
		Name:        "sample.pdf",
		Path:        "/",
		StoragePath: "sample.pdf",
		Size:        int64(len(content)),
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, func() {
		_ = db.Close()
	}
}

func newStorageUsageTestApp(t *testing.T) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, file := range []*model.File{
		{ID: "active", Name: "active.txt", Path: "/", StoragePath: "active.txt", Size: 12, MimeType: "text/plain", Status: model.FileStatusReady},
		{ID: "dir", Name: "dir", Path: "/", StoragePath: "dir", IsDir: true, Status: model.FileStatusReady},
		{ID: "trashed", Name: "trashed.txt", Path: "/", StoragePath: "trashed.txt", Size: 50, MimeType: "text/plain", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	if err := db.SoftDeleteFile(context.Background(), "trashed", "trashed"); err != nil {
		t.Fatalf("soft delete trashed: %v", err)
	}

	app := fiber.New()
	NewStorageHandler(service.NewFileService(cfg, db, nil)).Register(app)
	return app, func() {
		_ = db.Close()
	}
}

func newFileViewTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	file := &model.File{
		ID:          "file-1",
		Name:        "sample.pdf",
		Path:        "/",
		StoragePath: "sample.pdf",
		Size:        3,
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, "sample.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}
	thumbnailName := "file-1.jpg"
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{
		FileID:        file.ID,
		MetaJSON:      `{"type":"pdf"}`,
		ThumbnailPath: &thumbnailName,
	}); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Storage.ThumbnailDir, thumbnailName), []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), service.NewSearchService(cfg, db, nil, nil)).Register(app)
	return app, db, func() {
		_ = db.Close()
	}
}

func markFileViewedViaHTTP(t *testing.T, app *fiber.App, fileID string) time.Time {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/files/"+fileID+"/view", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("view request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body model.File
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.LastViewedAt == nil {
		t.Fatal("expected last_viewed_at to be set")
	}
	return *body.LastViewedAt
}

func queryFilesViaHTTP(t *testing.T, app *fiber.App, payload string) service.FileQueryResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/files/query", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("file query request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var page service.FileQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode file query response: %v", err)
	}
	return page
}

func queryPhotoTimelineViaHTTP(t *testing.T, app *fiber.App, payload string) service.FileQueryResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/files/photos/timeline", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("photo timeline request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var page service.FileQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode photo timeline response: %v", err)
	}
	return page
}

func fileIDs(files []model.File) []string {
	ids := make([]string, 0, len(files))
	for _, file := range files {
		ids = append(ids, file.ID)
	}
	return ids
}

func newEmptyFileHandlerTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, db, func() {
		_ = db.Close()
	}
}

func newFileQueryTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	files := []*model.File{
		{ID: "photo-old", Name: "旅行-1.jpg", Path: "/Photos", StoragePath: "Photos/trip-1.jpg", Size: 10, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "photo-new", Name: "旅行-2.jpg", Path: "/Photos", StoragePath: "Photos/trip-2.jpg", Size: 12, MimeType: "image/jpeg", Status: model.FileStatusReady},
		{ID: "document", Name: "旅行计划.pdf", Path: "/Docs", StoragePath: "Docs/plan.pdf", Size: 20, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "folder", Name: "旅行相册", Path: "/", StoragePath: "folder", IsDir: true, Status: model.FileStatusReady},
		{ID: "trashed", Name: "旅行-删除.jpg", Path: "/Photos", StoragePath: "Photos/deleted.jpg", Size: 8, MimeType: "image/jpeg", Status: model.FileStatusReady},
	}
	for i, file := range files {
		if i == 1 {
			time.Sleep(time.Millisecond)
		}
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	if err := db.SoftDeleteFile(context.Background(), "trashed", "trashed"); err != nil {
		t.Fatalf("soft delete trashed file: %v", err)
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, db, func() {
		_ = db.Close()
	}
}

func newBatchFileActionTestApp(t *testing.T) (*fiber.App, *store.Store, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "active", Name: "active.txt", Path: "/", StoragePath: "active.txt", Size: 6, MimeType: "text/plain", Status: model.FileStatusReady}, "active")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "dir-b", Name: "B", Path: "/A", StoragePath: "A/B", IsDir: true, Status: model.FileStatusReady}, "")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "foo", Name: "foo.txt", Path: "/A/B", StoragePath: "A/B/foo.txt", Size: 3, MimeType: "text/plain", Status: model.FileStatusReady}, "foo")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "sub", Name: "sub", Path: "/A/B", StoragePath: "A/B/sub", IsDir: true, Status: model.FileStatusReady}, "")
	createHandlerTestFile(t, db, storageRoot, &model.File{ID: "bar", Name: "bar.txt", Path: "/A/B/sub", StoragePath: "A/B/sub/bar.txt", Size: 3, MimeType: "text/plain", Status: model.FileStatusReady}, "bar")

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, db, func() {
		_ = db.Close()
	}
}

func createHandlerTestFile(t *testing.T, db *store.Store, storageRoot string, file *model.File, content string) {
	t.Helper()
	absPath := filepath.Join(storageRoot, filepath.FromSlash(file.StoragePath))
	if file.IsDir {
		if err := os.MkdirAll(absPath, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", file.StoragePath, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("mkdir parent %s: %v", file.StoragePath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", file.StoragePath, err)
		}
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file %s: %v", file.ID, err)
	}
}

func assertFilePathViaHTTP(t *testing.T, app *fiber.App, id, wantPath string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/files/"+id, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("get %s failed: %v", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected %s to be available, got %d", id, resp.StatusCode)
	}
	var file model.File
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		t.Fatalf("decode file %s: %v", id, err)
	}
	if file.Path != wantPath {
		t.Fatalf("file %s expected path %q, got %q", id, wantPath, file.Path)
	}
}

func newFileRenameTestApp(t *testing.T) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	for _, name := range []string{"sample.pdf", "existing.pdf"} {
		if err := os.WriteFile(filepath.Join(storageRoot, name), []byte("pdf"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, file := range []*model.File{
		{ID: "file-1", Name: "sample.pdf", Path: "/", StoragePath: "sample.pdf", Size: 3, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "file-2", Name: "existing.pdf", Path: "/", StoragePath: "existing.pdf", Size: 3, MimeType: "application/pdf", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, func() {
		_ = db.Close()
	}
}
