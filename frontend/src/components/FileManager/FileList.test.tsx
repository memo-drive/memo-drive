import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { DriveFile } from "../../types";
import "../../i18n";
import { canEditMarkdownFile, FileList } from "./FileList";

describe("FileList", () => {
  it("renders image files with their generated thumbnail", () => {
    const html = renderToString(
      <FileList
        files={[
          makeFile({
            id: "image-1",
            name: "receipt.jpg",
            mime_type: "image/jpeg",
            metadata: makeMetadata("image-1.jpg"),
          }),
        ]}
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).toContain('src="/api/files/image-1/thumbnail"');
    expect(html).toContain('alt="receipt.jpg"');
  });

  it("marks a folder row as busy while entering it", () => {
    const html = renderToString(
      <FileList
        files={[
          makeFile({
            id: "folder-1",
            name: "Photos",
            is_dir: true,
            mime_type: "",
          }),
        ]}
        enteringFolderId="folder-1"
        folderNavigationDisabled
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).toContain("aria-busy=\"true\"");
    expect(html).toContain("进入中");
  });

  it("virtualizes large desktop file lists without mounting every row", () => {
    const files = Array.from({ length: 120 }, (_, index) =>
      makeFile({ id: `file-${index}`, name: `Memo-${index}.pdf` }),
    );
    const html = renderToString(
      <FileList
        files={files}
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).toContain("data-virtual-list=\"true\"");
    expect(html).toContain("data-virtual-count=\"120\"");
    expect(html).toContain("Memo-0.pdf");
    expect(html).not.toContain("Memo-119.pdf");
    expect(html).not.toContain("第 1 / 3 页");
  });

  it("blocks the whole file list while entering a folder", () => {
    const html = renderToString(
      <FileList
        files={[
          makeFile({
            id: "folder-1",
            name: "Photos",
            is_dir: true,
            mime_type: "",
          }),
          makeFile({
            id: "image-1",
            name: "receipt.jpg",
            mime_type: "image/jpeg",
            metadata: makeMetadata("image-1.jpg"),
          }),
        ]}
        enteringFolderId="folder-1"
        folderNavigationDisabled
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).toContain("data-file-list-busy=\"true\"");
    expect(html.match(/aria-disabled="true"/g)).toHaveLength(3);
    expect(html).toContain("正在加载");
  });

  it("renders QuickTime video files with their generated thumbnail", () => {
    const html = renderToString(
      <FileList
        files={[
          makeFile({
            id: "video-1",
            name: "clip.mov",
            mime_type: "video/quicktime",
            metadata: makeMetadata("video-1.jpg"),
          }),
        ]}
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).toContain('src="/api/files/video-1/thumbnail"');
    expect(html).toContain('alt="clip.mov"');
  });

  it("does not request video thumbnails before processing has finished", () => {
    const html = renderToString(
      <FileList
        files={[
          makeFile({
            id: "video-1",
            name: "clip.mp4",
            mime_type: "video/mp4",
            status: "uploaded",
          }),
        ]}
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).not.toContain('src="/api/files/video-1/thumbnail"');
    expect(html).toContain("video_library");
  });

  it("does not request missing thumbnails after processing finished", () => {
    const html = renderToString(
      <FileList
        files={[
          makeFile({
            id: "video-1",
            name: "clip.mp4",
            mime_type: "video/mp4",
            status: "ready",
          }),
        ]}
        onOpenFolder={vi.fn()}
        onSelect={vi.fn()}
        onDelete={vi.fn()}
        onRename={vi.fn()}
        onMove={vi.fn()}
        onDownload={vi.fn()}
      />,
    );

    expect(html).not.toContain('src="/api/files/video-1/thumbnail"');
    expect(html).toContain("video_library");
  });

  it("allows the edit menu action only for markdown files", () => {
    expect(canEditMarkdownFile(makeFile({ name: "note.md", mime_type: "text/markdown" }))).toBe(true);
    expect(canEditMarkdownFile(makeFile({ name: "note.markdown", mime_type: "text/plain" }))).toBe(true);
    expect(canEditMarkdownFile(makeFile({ name: "memo.pdf", mime_type: "application/pdf" }))).toBe(false);
    expect(canEditMarkdownFile(makeFile({ name: "Notes", is_dir: true, mime_type: "" }))).toBe(false);
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.pdf",
    path: "/",
    storage_path: "Memo.pdf",
    size: 1024,
    mime_type: "application/pdf",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}

function makeMetadata(thumbnailPath: string): DriveFile["metadata"] {
  return {
    file_id: "file-1",
    meta_json: "{}",
    thumbnail_path: thumbnailPath,
    extracted_at: "2026-05-10T00:00:00Z",
  };
}
