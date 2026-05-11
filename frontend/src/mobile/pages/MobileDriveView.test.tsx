import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { DriveFile, FileSearchHit } from "../../types";
import "../../i18n";
import { MobileDriveView } from "./MobileDriveView";

describe("MobileDriveView", () => {
  it("renders Files as mobile cards for the current Folder", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobileDriveView
          currentPath="/Docs"
          files={[
            makeFile({ id: "folder-1", name: "AI", is_dir: true }),
            makeFile({ id: "file-1", name: "Memo.pdf" }),
          ]}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("AI");
    expect(html).toContain("Memo.pdf");
    expect(html).toContain("href=\"/m?path=%2FDocs%2FAI\"");
    expect(html).toContain("href=\"/m/preview/file-1?path=%2FDocs\"");
  });

  it("renders a single File action sheet without sharing actions", () => {
    const file = makeFile({ id: "file-1", name: "Memo.pdf" });
    const html = renderToString(
      <MemoryRouter>
        <MobileDriveView
          currentPath="/Docs"
          files={[file]}
          actionFile={file}
          actionDownloadHref="/download/file-1"
          onOpenActions={() => undefined}
          onCloseActions={() => undefined}
          onRename={() => undefined}
          onDelete={() => undefined}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("role=\"dialog\"");
    expect(html).toContain("Memo.pdf");
    expect(html).toContain("href=\"/download/file-1\"");
    expect(html).toContain("下载");
    expect(html).toContain("重命名");
    expect(html).toContain("移到回收站");
    expect(html).not.toContain("分享");
  });

  it("renders a lightweight delete confirmation prompt for the selected File", () => {
    const file = makeFile({ id: "file-1", name: "Memo.pdf" });
    const html = renderToString(
      <MemoryRouter>
        <MobileDriveView
          currentPath="/Docs"
          files={[file]}
          deleteConfirmFile={file}
          deleteConfirmBusy={false}
          onOpenActions={() => undefined}
          onCancelDelete={() => undefined}
          onConfirmDelete={() => undefined}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("data-mobile-confirm=\"light\"");
    expect(html).toContain("确认删除");
    expect(html).toContain("确定要把 Memo.pdf 移到回收站吗？");
    expect(html).toContain("移到回收站");
  });

  it("renders current Folder search results as mobile cards", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobileDriveView
          currentPath="/Docs"
          files={[]}
          searchDraft="发票"
          searchActive
          searchHits={[
            makeSearchHit({
              file: makeFile({ id: "invoice-1", name: "invoice.pdf" }),
              snippet: "2026 年 5 月发票",
              score: 0.82,
            }),
          ]}
          searching={false}
          onSearchDraftChange={() => undefined}
          onSearchSubmit={() => undefined}
          onClearSearch={() => undefined}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("value=\"发票\"");
    expect(html).toContain("找到 1 条");
    expect(html).toContain("invoice.pdf");
    expect(html).toContain("2026 年 5 月发票");
    expect(html).toContain("href=\"/m/preview/invoice-1?path=%2FDocs\"");
    expect(html).toContain("清除搜索");
  });

  it("renders semantic search as an opt-in toggle that is off by default", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobileDriveView
          currentPath="/Docs"
          files={[makeFile({ id: "file-1", name: "Memo.pdf" })]}
          includeSemantic={false}
          onSemanticChange={() => undefined}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("role=\"switch\"");
    expect(html).toContain("aria-checked=\"false\"");
    expect(html).toContain("语义搜索");
    expect(html).toContain("更慢");
  });

  it("renders a lightweight rename prompt with the selected File name", () => {
    const file = makeFile({ id: "file-1", name: "Memo.pdf" });
    const html = renderToString(
      <MemoryRouter>
        <MobileDriveView
          currentPath="/Docs"
          files={[file]}
          renameFile={file}
          renameDraft="Memo-renamed.pdf"
          renameError="名称不能包含 /"
          renameBusy={false}
          onRenameDraftChange={() => undefined}
          onCancelRename={() => undefined}
          onConfirmRename={() => undefined}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("data-mobile-text-prompt=\"light\"");
    expect(html).toContain("重命名");
    expect(html).toContain("新的文件名");
    expect(html).toContain("value=\"Memo-renamed.pdf\"");
    expect(html).toContain("名称不能包含 /");
    expect(html).toContain("保存");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "item-1",
    name: "Memo.pdf",
    path: "/Docs",
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

function makeSearchHit(overrides: Partial<FileSearchHit> = {}): FileSearchHit {
  return {
    file: makeFile(),
    match_types: ["name"],
    snippet: "",
    score: 0.5,
    ...overrides,
  };
}
