import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import "../../i18n";
import { MobilePreviewView } from "./MobilePreviewView";

describe("MobilePreviewView", () => {
  it("renders a full-screen File preview with return and download actions", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobilePreviewView
          file={makeFile({ id: "file-1", name: "Memo.txt" })}
          returnHref="/m/files?path=%2FDocs"
          downloadHref="/api/files/file-1/download"
        />
      </MemoryRouter>,
    );

    expect(html).toContain("Memo.txt");
    expect(html).toContain("href=\"/m/files?path=%2FDocs\"");
    expect(html).toContain("href=\"/api/files/file-1/download\"");
    expect(html).not.toContain("移动预览");
    expect(html).not.toContain(">Loading<");
    expect(html).not.toContain("eyebrow");
  });

  it("returns document category previews to the document category route", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobilePreviewView
          file={makeFile({ id: "doc-1", name: "手册.pdf", mime_type: "application/pdf" })}
          returnHref="/m/category/documents"
          downloadHref="/api/files/doc-1/download"
        />
      </MemoryRouter>,
    );

    expect(html).toContain("href=\"/m/category/documents\"");
    expect(html).not.toContain("href=\"/m/files");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.txt",
    path: "/Docs",
    storage_path: "Memo.txt",
    size: 42,
    mime_type: "text/plain",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
