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
          returnPath="/Docs"
          downloadHref="/api/files/file-1/download"
        />
      </MemoryRouter>,
    );

    expect(html).toContain("Memo.txt");
    expect(html).toContain("href=\"/m?path=%2FDocs\"");
    expect(html).toContain("href=\"/api/files/file-1/download\"");
    expect(html).not.toContain("移动预览");
    expect(html).not.toContain(">Loading<");
    expect(html).not.toContain("eyebrow");
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
