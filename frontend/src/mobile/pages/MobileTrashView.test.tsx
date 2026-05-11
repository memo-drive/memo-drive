import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { DriveFile } from "../../types";
import "../../i18n";
import { MobileTrashView } from "./MobileTrashView";

describe("MobileTrashView", () => {
  it("renders Trash Entries with restore and purge actions", () => {
    const html = renderToString(
      <MobileTrashView
        files={[
          makeFile({
            id: "trash-1",
            name: "trash-1-Memo.pdf",
            original_name: "Memo.pdf",
            original_path: "/Docs/Memo.pdf",
          }),
        ]}
        loading={false}
        actionId=""
        emptying={false}
        onRestore={vi.fn()}
        onPurge={vi.fn()}
        onEmpty={vi.fn()}
      />,
    );

    expect(html).toContain("Memo.pdf");
    expect(html).toContain("/Docs/Memo.pdf");
    expect(html).toContain("恢复");
    expect(html).toContain("永久删除");
    expect(html).toContain("清空回收站");
  });

  it("renders lightweight confirmations for purge and empty trash", () => {
    const file = makeFile({
      id: "trash-1",
      name: "trash-1-Memo.pdf",
      original_name: "Memo.pdf",
      original_path: "/Docs/Memo.pdf",
    });

    const purgeHtml = renderToString(
      <MobileTrashView
        files={[file]}
        loading={false}
        actionId=""
        emptying={false}
        purgeConfirmFile={file}
        onRestore={vi.fn()}
        onPurge={vi.fn()}
        onEmpty={vi.fn()}
        onCancelPurge={vi.fn()}
        onConfirmPurge={vi.fn()}
      />,
    );

    expect(purgeHtml).toContain("data-mobile-confirm=\"light\"");
    expect(purgeHtml).toContain("永久删除");
    expect(purgeHtml).toContain("确定要永久删除 Memo.pdf 吗？此操作不可撤销。");

    const emptyHtml = renderToString(
      <MobileTrashView
        files={[file]}
        loading={false}
        actionId=""
        emptying={false}
        emptyConfirmOpen
        onRestore={vi.fn()}
        onPurge={vi.fn()}
        onEmpty={vi.fn()}
        onCancelEmpty={vi.fn()}
        onConfirmEmpty={vi.fn()}
      />,
    );

    expect(emptyHtml).toContain("data-mobile-confirm=\"light\"");
    expect(emptyHtml).toContain("清空回收站");
    expect(emptyHtml).toContain("确定要永久删除回收站中的全部 1 项吗？此操作不可撤销。");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "trash-1",
    name: "trash-1-Memo.pdf",
    path: "/.trash",
    storage_path: ".trash/trash-1-Memo.pdf",
    size: 1024,
    mime_type: "application/pdf",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    deleted_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
