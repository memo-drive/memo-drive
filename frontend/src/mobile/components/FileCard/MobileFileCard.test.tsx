import { renderToString } from "react-dom/server";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../../types";
import "../../../i18n";
import { MobileFileCard } from "./MobileFileCard";

describe("MobileFileCard", () => {
  it("links Folders to mobile Files and Files to mobile Preview", () => {
    const folderHtml = renderCard(
      <MobileFileCard
        file={makeFile({ is_dir: true, name: "Photos" })}
        currentPath="/"
      />,
    );
    const fileHtml = renderCard(
      <MobileFileCard
        file={makeFile({ id: "file-1", name: "note.pdf" })}
        currentPath="/Docs"
      />,
    );

    expect(folderHtml).toContain("href=\"/m/files?path=%2FPhotos\"");
    expect(fileHtml).toContain("href=\"/m/preview/file-1?path=%2FDocs\"");
  });

  it("keeps stale folder cards from appending the same path again while entering", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({
          is_dir: true,
          name: "Photos",
          path: "/",
          storage_path: "Photos",
        })}
        currentPath="/Photos"
      />,
    );

    expect(html).toContain("href=\"/m/files?path=%2FPhotos\"");
    expect(html).not.toContain("%2FPhotos%2FPhotos");
  });

  it("marks an entering folder as busy so repeat taps are ignored", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({ is_dir: true, name: "Photos" })}
        currentPath="/"
        entering
        folderNavigationDisabled
      />,
    );

    expect(html).toContain("aria-disabled=\"true\"");
    expect(html).toContain("进入中");
  });

  it("disables file preview and actions while the folder view is loading", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({
          id: "image-1",
          name: "receipt.jpg",
          mime_type: "image/jpeg",
        })}
        currentPath="/Photos"
        folderNavigationDisabled
      />,
    );

    expect(html).toContain("aria-disabled=\"true\"");
    expect(html).toContain("disabled=\"\"");
  });

  it("renders image files with their generated thumbnail", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({
          id: "image-1",
          name: "receipt.jpg",
          mime_type: "image/jpeg",
          metadata: makeMetadata("image-1.jpg"),
        })}
        currentPath="/Docs"
      />,
    );

    expect(html).toContain('src="/api/files/image-1/thumbnail"');
    expect(html).toContain('alt="receipt.jpg"');
    expect(html).toContain("href=\"/m/preview/image-1?path=%2FDocs\"");
  });

  it("renders QuickTime video files with their generated thumbnail", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({
          id: "video-1",
          name: "clip.mov",
          mime_type: "video/quicktime",
          metadata: makeMetadata("video-1.jpg"),
        })}
        currentPath="/Docs"
      />,
    );

    expect(html).toContain('src="/api/files/video-1/thumbnail"');
    expect(html).toContain('alt="clip.mov"');
    expect(html).toContain("href=\"/m/preview/video-1?path=%2FDocs\"");
  });

  it("does not request video thumbnails before processing has finished", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({
          id: "video-1",
          name: "clip.mp4",
          mime_type: "video/mp4",
          status: "processing",
        })}
        currentPath="/Docs"
      />,
    );

    expect(html).not.toContain('src="/api/files/video-1/thumbnail"');
    expect(html).toContain("video_library");
  });

  it("does not request missing thumbnails after processing finished", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({
          id: "video-1",
          name: "clip.mp4",
          mime_type: "video/mp4",
          status: "ready",
        })}
        currentPath="/Docs"
      />,
    );

    expect(html).not.toContain('src="/api/files/video-1/thumbnail"');
    expect(html).toContain("video_library");
  });

  it("renders a selectable state for multi-select mode and hides per-file actions", () => {
    const html = renderCard(
      <MobileFileCard
        file={makeFile({ id: "file-1", name: "note.pdf" })}
        currentPath="/Docs"
        selectionMode
        selected
        onSelectionToggle={() => undefined}
        onLongPress={() => undefined}
      />,
    );

    expect(html).toContain("data-mobile-selectable=\"true\"");
    expect(html).toContain("aria-selected=\"true\"");
    expect(html).toContain("check_circle");
    expect(html).not.toContain("more_vert");
    expect(html).not.toContain("href=\"/m/preview/file-1?path=%2FDocs\"");
  });
});

function renderCard(node: ReactNode) {
  return renderToString(<MemoryRouter>{node}</MemoryRouter>);
}

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "item-1",
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
