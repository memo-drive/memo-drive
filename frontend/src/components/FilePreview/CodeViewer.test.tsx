import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import "../../i18n";
import type { DriveFile } from "../../types";
import { CodePreviewDocument } from "./CodeViewer";

describe("CodeViewer document preview", () => {
  it("renders markdown content without a non-functional preview toolbar", () => {
    const html = renderToString(
      <CodePreviewDocument file={makeFile({ name: "problem.md", mime_type: "text/markdown" })} content="1" />,
    );

    expect(html).toContain("<p>1</p>");
    expect(html).not.toContain("Markdown 预览");
  });

  it("renders plain text content without an auto-detect toolbar", () => {
    const html = renderToString(
      <CodePreviewDocument file={makeFile({ name: "problem.txt", mime_type: "text/plain" })} content="hello" />,
    );

    expect(html).toContain("hello");
    expect(html).not.toContain("自动识别");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "problem.md",
    path: "/",
    storage_path: "problem.md",
    size: 1,
    mime_type: "text/markdown",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
