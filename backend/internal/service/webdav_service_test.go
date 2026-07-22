package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestWebDAVServiceListsDirectActiveChildren(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	webdav := NewWebDAVService(files.cfg, db)

	createServiceTestFile(t, db, root, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")
	createServiceTestFile(t, db, root, &model.File{ID: "visible", Name: "visible.md", Path: "/Notes", StoragePath: "Notes/visible.md", Size: 7, MimeType: "text/markdown", Status: model.FileStatusReady}, "visible")
	createServiceTestFile(t, db, root, &model.File{ID: "child", Name: "Child", Path: "/Notes", StoragePath: "Notes/Child", IsDir: true, Status: model.FileStatusReady}, "")
	createServiceTestFile(t, db, root, &model.File{ID: "nested", Name: "nested.md", Path: "/Notes/Child", StoragePath: "Notes/Child/nested.md", Size: 6, MimeType: "text/markdown", Status: model.FileStatusReady}, "nested")
	createServiceTestFile(t, db, root, &model.File{ID: "trashed", Name: "trashed.md", Path: "/Notes", StoragePath: "Notes/trashed.md", Size: 7, MimeType: "text/markdown", Status: model.FileStatusReady}, "trashed")
	if err := db.SoftDeleteFile(context.Background(), "trashed", "trashed"); err != nil {
		t.Fatalf("soft delete trashed child: %v", err)
	}

	children, err := webdav.ListChildren(context.Background(), "/notes")
	if err != nil {
		t.Fatalf("ListChildren returned error: %v", err)
	}
	got := make([]string, len(children))
	for i := range children {
		got[i] = children[i].ID
	}
	want := []string{"child", "visible"}
	if len(got) != len(want) {
		t.Fatalf("expected direct active children %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected direct active children %v, got %v", want, got)
		}
	}
}

func TestWebDAVServiceCreateFolderCreatesStorageDirectoryAndRecord(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	webdav := NewWebDAVService(files.cfg, db)
	createServiceTestFile(t, db, root, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")

	folder, err := webdav.CreateFolder(context.Background(), "/notes/NewFolder")
	if err != nil {
		t.Fatalf("CreateFolder returned error: %v", err)
	}
	if folder.Name != "NewFolder" || folder.Path != "/Notes" || folder.StoragePath != "Notes/NewFolder" || !folder.IsDir || folder.Status != model.FileStatusReady {
		t.Fatalf("expected created folder with canonical path and ready status, got %#v", folder)
	}

	stat, err := os.Stat(filepath.Join(root, "Notes", "NewFolder"))
	if err != nil {
		t.Fatalf("expected physical folder to exist: %v", err)
	}
	if !stat.IsDir() {
		t.Fatal("expected physical WebDAV folder path to be a directory")
	}
	active, err := db.GetActiveByPath(context.Background(), "/Notes", "NewFolder")
	if err != nil {
		t.Fatalf("expected folder File record to exist: %v", err)
	}
	if active.ID != folder.ID {
		t.Fatalf("expected active folder record %q, got %q", folder.ID, active.ID)
	}
}

func TestWebDAVServiceRejectsUnsafeVirtualPathBeforeNormalizing(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	webdav := NewWebDAVService(files.cfg, db)
	createServiceTestFile(t, db, root, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")

	for _, virtualPath := range []string{
		"Notes/readme.md",
		"/Notes//readme.md",
		"/Notes/./readme.md",
		"/Notes/../escape.md",
		"/Notes/nul\x00byte.md",
		"/Notes\\readme.md",
		"/Notes/ readme.md",
		"/Notes/readme.md.",
		"/Notes/bad:name.md",
		"/.Trash",
	} {
		t.Run(virtualPath, func(t *testing.T) {
			if _, err := webdav.Resolve(context.Background(), virtualPath); !errors.Is(err, ErrInvalidWebDAVPath) {
				t.Fatalf("expected ErrInvalidWebDAVPath, got %v", err)
			}
		})
	}

	_, err := webdav.PutFile(context.Background(), WebDAVCreateFileInput{
		VirtualPath:   "/Notes/../escape.md",
		Body:          strings.NewReader("escape"),
		ContentLength: int64(len("escape")),
		ContentType:   "text/markdown",
	})
	if !errors.Is(err, ErrInvalidWebDAVPath) {
		t.Fatalf("expected ErrInvalidWebDAVPath for unsafe WebDAV path, got %v", err)
	}
	if _, err := webdav.Resolve(context.Background(), "/escape.md"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected unsafe PUT not to create normalized /escape.md, got %v", err)
	}
}

func TestWebDAVServiceCreatesAndResolvesUnicodePaths(t *testing.T) {
	files, db, _ := newFileServiceTestHarness(t)
	files.cfg.Storage.MaxFileSize = 1024 * 1024
	webdav := NewWebDAVService(files.cfg, db)

	folder, err := webdav.CreateFolder(context.Background(), "/文档")
	if err != nil {
		t.Fatalf("CreateFolder returned error: %v", err)
	}
	if folder.Name != "文档" || folder.Path != "/" || folder.StoragePath != "文档" {
		t.Fatalf("expected Unicode folder metadata to remain intact, got %#v", folder)
	}

	result, err := webdav.PutFile(context.Background(), WebDAVCreateFileInput{
		VirtualPath:   "/文档/资料📄.md",
		Body:          strings.NewReader("你好，WebDAV"),
		ContentLength: int64(len("你好，WebDAV")),
		ContentType:   "text/markdown",
	})
	if err != nil {
		t.Fatalf("PutFile returned error: %v", err)
	}
	if !result.Created || result.File.Name != "资料📄.md" || result.File.Path != "/文档" {
		t.Fatalf("expected Unicode file to be created under Unicode folder, got %#v", result)
	}

	resolved, err := webdav.Resolve(context.Background(), "/文档/资料📄.md")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.VirtualPath != "/文档/资料📄.md" || resolved.File.ID != result.File.ID {
		t.Fatalf("expected Unicode path to resolve canonically, got %#v", resolved)
	}

	_, downloadPath, err := webdav.DownloadPath(resolved)
	if err != nil {
		t.Fatalf("DownloadPath returned error: %v", err)
	}
	body, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("read stored Unicode file: %v", err)
	}
	if string(body) != "你好，WebDAV" {
		t.Fatalf("expected stored Unicode file content, got %q", body)
	}
}

func TestMoveWebDAVUploadIntoPlaceFallsBackWhenRenameCrossesDevices(t *testing.T) {
	root := t.TempDir()
	tempPath := filepath.Join(root, "upload.tmp")
	if err := os.WriteFile(tempPath, []byte("new content"), 0o644); err != nil {
		t.Fatalf("write upload temp: %v", err)
	}
	destDir := filepath.Join(root, "files")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("create dest dir: %v", err)
	}
	absPath := filepath.Join(destDir, "target.txt")
	if err := os.WriteFile(absPath, []byte("old content"), 0o644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	rename := func(oldPath, newPath string) error {
		if oldPath == tempPath && newPath == absPath {
			return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
		}
		return os.Rename(oldPath, newPath)
	}
	if err := moveWebDAVUploadIntoPlaceWithRename(tempPath, absPath, rename); err != nil {
		t.Fatalf("move WebDAV upload with EXDEV fallback: %v", err)
	}

	body, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read fallback target: %v", err)
	}
	if string(body) != "new content" {
		t.Fatalf("expected fallback target content to be replaced, got %q", body)
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source temp to be removed after fallback, got %v", err)
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("read dest dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "target.txt" {
		t.Fatalf("expected fallback temp file to be cleaned up, got %#v", entries)
	}
}

func TestWebDAVServiceConcurrentPutSamePathSerializesIntoOverwrite(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	files.cfg.Storage.MaxFileSize = 1024 * 1024
	webdav := NewWebDAVService(files.cfg, db)
	createServiceTestFile(t, db, root, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")

	firstBody := newWebDAVBlockingReader("first")
	firstDone := make(chan webDAVPutResult, 1)
	go func() {
		result, err := webdav.PutFile(context.Background(), WebDAVCreateFileInput{
			VirtualPath:   "/Notes/race.md",
			Body:          firstBody,
			ContentLength: int64(len("first")),
			ContentType:   "text/plain",
		})
		firstDone <- webDAVPutResult{result: result, err: err}
	}()
	firstBody.waitUntilRead(t)

	secondDone := make(chan webDAVPutResult, 1)
	go func() {
		result, err := webdav.PutFile(context.Background(), WebDAVCreateFileInput{
			VirtualPath:   "/Notes/race.md",
			Body:          strings.NewReader("second"),
			ContentLength: int64(len("second")),
			ContentType:   "text/plain",
		})
		secondDone <- webDAVPutResult{result: result, err: err}
	}()
	firstBody.release()
	first := waitForWebDAVPutResult(t, firstDone)
	second := waitForWebDAVPutResult(t, secondDone)

	if first.err != nil {
		t.Fatalf("expected first concurrent PUT to succeed after serialization, got %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("expected second concurrent PUT to overwrite after serialization, got %v", second.err)
	}
	if !first.result.Created || second.result.Created {
		t.Fatalf("expected first PUT to create and second PUT to overwrite, first=%#v second=%#v", first.result, second.result)
	}

	children, err := webdav.ListChildren(context.Background(), "/Notes")
	if err != nil {
		t.Fatalf("list children after concurrent PUT: %v", err)
	}
	count := 0
	var final model.File
	for _, child := range children {
		if child.Name == "race.md" {
			count++
			final = child
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one active race.md after concurrent PUT, got %d in %#v", count, children)
	}
	if final.Size != int64(len("second")) {
		t.Fatalf("expected final PUT content size from second writer, got file %#v", final)
	}
}

func TestWebDAVServiceMoveFolderWaitsForChildPut(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	files.cfg.Storage.MaxFileSize = 1024 * 1024
	webdav := NewWebDAVService(files.cfg, db)
	createServiceTestFile(t, db, root, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")
	if _, err := webdav.CreateFolder(context.Background(), "/Notes/Child"); err != nil {
		t.Fatalf("create child folder: %v", err)
	}

	body := newWebDAVBlockingReader("nested")
	putDone := make(chan webDAVPutResult, 1)
	go func() {
		result, err := webdav.PutFile(context.Background(), WebDAVCreateFileInput{
			VirtualPath:   "/Notes/Child/new.md",
			Body:          body,
			ContentLength: int64(len("nested")),
			ContentType:   "text/markdown",
		})
		putDone <- webDAVPutResult{result: result, err: err}
	}()
	body.waitUntilRead(t)

	source, err := webdav.Resolve(context.Background(), "/Notes/Child")
	if err != nil {
		t.Fatalf("resolve source folder: %v", err)
	}
	moveDone := make(chan webDAVMoveResult, 1)
	go func() {
		result, err := webdav.Move(context.Background(), WebDAVMoveInput{
			Source:          source,
			DestinationPath: "/Moved",
			Overwrite:       true,
		})
		moveDone <- webDAVMoveResult{result: result, err: err}
	}()
	body.release()

	put := waitForWebDAVPutResult(t, putDone)
	move := waitForWebDAVMoveResult(t, moveDone)
	if put.err != nil {
		t.Fatalf("expected child PUT to succeed before folder MOVE completes, got %v", put.err)
	}
	if move.err != nil {
		t.Fatalf("expected folder MOVE to succeed after child PUT, got %v", move.err)
	}

	if _, err := webdav.Resolve(context.Background(), "/Notes/Child/new.md"); err == nil {
		t.Fatal("expected old child path to disappear after folder MOVE")
	}
	moved, err := webdav.Resolve(context.Background(), "/Moved/new.md")
	if err != nil {
		t.Fatalf("expected child PUT to move with folder, got %v", err)
	}
	if moved.File == nil || moved.File.Path != "/Moved" || moved.File.Size != int64(len("nested")) {
		t.Fatalf("expected moved child metadata under /Moved, got %#v", moved.File)
	}
}

func TestWebDAVServiceMoveAndCopyCrossPathsCompleteWithoutDeadlock(t *testing.T) {
	files, db, root := newFileServiceTestHarness(t)
	files.cfg.Storage.MaxFileSize = 1024 * 1024
	webdav := NewWebDAVService(files.cfg, db)
	createServiceTestFile(t, db, root, &model.File{ID: "notes", Name: "Notes", Path: "/", StoragePath: "Notes", IsDir: true, Status: model.FileStatusReady}, "")
	createServiceTestFile(t, db, root, &model.File{ID: "a", Name: "a.md", Path: "/Notes", StoragePath: "Notes/a.md", Size: 1, MimeType: "text/markdown", Status: model.FileStatusReady}, "a")
	createServiceTestFile(t, db, root, &model.File{ID: "b", Name: "b.md", Path: "/Notes", StoragePath: "Notes/b.md", Size: 1, MimeType: "text/markdown", Status: model.FileStatusReady}, "b")

	sourceA, err := webdav.Resolve(context.Background(), "/Notes/a.md")
	if err != nil {
		t.Fatalf("resolve source a: %v", err)
	}
	sourceB, err := webdav.Resolve(context.Background(), "/Notes/b.md")
	if err != nil {
		t.Fatalf("resolve source b: %v", err)
	}

	done := make(chan error, 2)
	go func() {
		_, err := webdav.Move(context.Background(), WebDAVMoveInput{
			Source:          sourceA,
			DestinationPath: "/Notes/b.md",
			Overwrite:       true,
		})
		done <- err
	}()
	go func() {
		_, err := webdav.Copy(context.Background(), WebDAVCopyInput{
			Source:          sourceB,
			DestinationPath: "/Notes/a.md",
			Overwrite:       true,
		})
		done <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("expected concurrent MOVE/COPY to complete without operation error, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for concurrent MOVE/COPY; possible path lock deadlock")
		}
	}
	children, err := webdav.ListChildren(context.Background(), "/Notes")
	if err != nil {
		t.Fatalf("list children after concurrent MOVE/COPY: %v", err)
	}
	names := map[string]int{}
	for _, child := range children {
		names[child.Name]++
	}
	for name, count := range names {
		if count > 1 {
			t.Fatalf("expected no duplicate active child names after concurrent MOVE/COPY, got %s count %d in %#v", name, count, children)
		}
	}
}

type webDAVBlockingReader struct {
	data      string
	started   chan struct{}
	releaseCh chan struct{}
	once      sync.Once
	offset    int
}

func newWebDAVBlockingReader(data string) *webDAVBlockingReader {
	return &webDAVBlockingReader{
		data:      data,
		started:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (r *webDAVBlockingReader) Read(p []byte) (int, error) {
	r.once.Do(func() {
		close(r.started)
		<-r.releaseCh
	})
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *webDAVBlockingReader) waitUntilRead(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked WebDAV reader to be read")
	}
}

func (r *webDAVBlockingReader) release() {
	close(r.releaseCh)
}

type webDAVPutResult struct {
	result *WebDAVPutFileResult
	err    error
}

type webDAVMoveResult struct {
	result *WebDAVMoveResult
	err    error
}

func waitForWebDAVPutResult(t *testing.T, done <-chan webDAVPutResult) webDAVPutResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WebDAV PUT result")
	}
	return webDAVPutResult{}
}

func waitForWebDAVMoveResult(t *testing.T, done <-chan webDAVMoveResult) webDAVMoveResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WebDAV MOVE result")
	}
	return webDAVMoveResult{}
}
