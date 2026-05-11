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

    expect(folderHtml).toContain("href=\"/m?path=%2FPhotos\"");
    expect(fileHtml).toContain("href=\"/m/preview/file-1?path=%2FDocs\"");
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
